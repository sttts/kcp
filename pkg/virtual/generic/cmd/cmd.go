package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kubeoptions "k8s.io/kubernetes/pkg/kubeapiserver/options"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	kcpinformer "github.com/kcp-dev/kcp/pkg/client/informers/externalversions"
	virtualbuilders "github.com/kcp-dev/kcp/pkg/virtual/generic/builders"
	virtualrootapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/rootapiserver"
)

type APIServerOptions struct {
	KubeConfigFile string
	Output         io.Writer

	secureServing     *genericapiserveroptions.SecureServingOptionsWithLoopback
	authentication    *genericapiserveroptions.DelegatingAuthenticationOptions
	authorization     *genericapiserveroptions.DelegatingAuthorizationOptions
	subCommandOptions SubCommandOptions
}

type SubCommandDescription struct {
	Name  string
	Use   string
	Short string
	Long  string
}

type SubCommandOptions interface {
	Description() SubCommandDescription
	AddFlags(flags *pflag.FlagSet)
	Validate() []error
	InitializeBuilders(clientcmd.ClientConfig, *rest.Config) ([]virtualrootapiserver.InformerStart, []virtualbuilders.VirtualWorkspaceBuilder, error)
}

func APIServerCommand(out, errout io.Writer, stopCh <-chan struct{}, subCommandOptions SubCommandOptions) *cobra.Command {
	options := &APIServerOptions{
		Output:            out,
		secureServing:     kubeoptions.NewSecureServingOptions().WithLoopback(),
		authentication:    genericapiserveroptions.NewDelegatingAuthenticationOptions(),
		authorization:     genericapiserveroptions.NewDelegatingAuthorizationOptions().WithAlwaysAllowPaths("/healthz", "/healthz/").WithAlwaysAllowGroups("system:masters"),
		subCommandOptions: subCommandOptions,
	}

	cmd := &cobra.Command{
		Use:   options.subCommandOptions.Description().Use,
		Short: options.subCommandOptions.Description().Short,
		Long:  templates.LongDesc(options.subCommandOptions.Description().Long),
		Run: func(c *cobra.Command, args []string) {
			kcmdutil.CheckErr(options.Validate())

			if err := options.RunAPIServer(stopCh); err != nil {
				if kerrors.IsInvalid(err) {
					var statusError *kerrors.StatusError
					if isStatusError := errors.As(err, &statusError); isStatusError && statusError.ErrStatus.Details != nil {
						details := statusError.ErrStatus.Details
						fmt.Fprintf(errout, "Invalid %s %s\n", details.Kind, details.Name)
						for _, cause := range details.Causes {
							fmt.Fprintf(errout, "  %s: %s\n", cause.Field, cause.Message)
						}
						os.Exit(255)
					}
				}
				klog.Fatal(err)
			}
		},
	}

	options.AddFlags(cmd.Flags())

	return cmd
}

func (o *APIServerOptions) AddFlags(flags *pflag.FlagSet) {
	// This command only supports reading from config
	flags.StringVar(&o.KubeConfigFile, "kubeconfig", "", "Kubeconfig of the Kube API server to proxy to.")
	_ = cobra.MarkFlagRequired(flags, "kubeconfig")

	o.secureServing.AddFlags(flags)
	o.authentication.AddFlags(flags)
	o.authorization.AddFlags(flags)
	o.subCommandOptions.AddFlags(flags)
}

func (o *APIServerOptions) Validate() error {
	errs := []error{}
	if len(o.KubeConfigFile) == 0 {
		errs = append(errs, errors.New("--kubeconfig is required for this command"))
	}
	errs = append(errs, o.secureServing.Validate()...)
	errs = append(errs, o.authentication.Validate()...)
	errs = append(errs, o.authorization.Validate()...)
	errs = append(errs, o.subCommandOptions.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// RunAPIServer takes the options, starts the API server and waits until stopCh is closed or initial listening fails.
func (o *APIServerOptions) RunAPIServer(stopCh <-chan struct{}) error {
	// Resolve relative to CWD
	absoluteKubeConfigFile, err := api.MakeAbs(o.KubeConfigFile, "")
	if err != nil {
		return err
	}

	kubeConfigBytes, err := ioutil.ReadFile(absoluteKubeConfigFile)
	if err != nil {
		return err
	}
	kubeConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	if err != nil {
		return err
	}
	kubeClientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	kcpClient, err := kcpclient.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}
	kcpInformer := kcpinformer.NewSharedInformerFactory(kcpClient, 10*time.Minute)

	utilruntime.Must(tenancyv1alpha1.AddToScheme(legacyscheme.Scheme))
	if err := legacyscheme.Scheme.SetVersionPriority(tenancyv1alpha1.SchemeGroupVersion); err != nil {
		return err
	}

	informerStarts := []virtualrootapiserver.InformerStart{
		kcpInformer.Start,
	}
	newInformerStarts, virtualWorkspaceBuilders, err := o.subCommandOptions.InitializeBuilders(kubeConfig, kubeClientConfig)
	if err != nil {
		return err
	}
	informerStarts = append(informerStarts, newInformerStarts...)

	rootAPIServerConfig, err := virtualrootapiserver.NewRootAPIConfig(kubeClientConfig, kcpClient, kcpInformer, o.secureServing, o.authentication, o.authorization, informerStarts, virtualWorkspaceBuilders...)
	if err != nil {
		return err
	}

	completedRootAPIServerConfig := rootAPIServerConfig.Complete()
	rootAPIServer, err := completedRootAPIServerConfig.New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		return err
	}
	preparedRootAPIServer := rootAPIServer.GenericAPIServer.PrepareRun()

	// this **must** be done after PrepareRun() as it sets up the openapi endpoints
	if err := completedRootAPIServerConfig.WithOpenAPIAggregationController(preparedRootAPIServer.GenericAPIServer); err != nil {
		return err
	}

	klog.Infof("Starting master on %s (%s)", rootAPIServerConfig.GenericConfig.ExternalAddress, version.Get().String())

	return preparedRootAPIServer.Run(stopCh)
}
