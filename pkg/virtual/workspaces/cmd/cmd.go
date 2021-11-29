package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kubeoptions "k8s.io/kubernetes/pkg/kubeapiserver/options"
	"k8s.io/apiserver/pkg/registry/rest"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	kcpinformer "github.com/kcp-dev/kcp/pkg/client/informers/externalversions"
	virtualapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/apiserver"
	virtualgenericcmd "github.com/kcp-dev/kcp/pkg/virtual/generic/cmd"
	virtualrootapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/rootapiserver"
	virtualworkspacesregistry "github.com/kcp-dev/kcp/pkg/virtual/workspaces/registry"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
)

type WorkspacesAPIServer struct {
	KubeConfigFile string
	Output     io.Writer

	SecureServing *genericapiserveroptions.SecureServingOptionsWithLoopback
	Authentication *genericapiserveroptions.DelegatingAuthenticationOptions
	Authorization  *genericapiserveroptions.DelegatingAuthorizationOptions
}

var longDescription = templates.LongDesc(`
	Start a virtual workspace apiserver to managing personal, organizational or global workspaces`)

func NewWorkspacesAPIServerCommand(out, errout io.Writer, stopCh <-chan struct{}) *cobra.Command {
	options := &WorkspacesAPIServer{
		Output:         out,
		SecureServing:  kubeoptions.NewSecureServingOptions().WithLoopback(),
		Authentication: genericapiserveroptions.NewDelegatingAuthenticationOptions(),
		Authorization:  genericapiserveroptions.NewDelegatingAuthorizationOptions().WithAlwaysAllowPaths("/healthz", "/healthz/").WithAlwaysAllowGroups("system:masters"),
	}

	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "Launch workspaces virtual workspace apiserver",
		Long:  longDescription,
		Run: func(c *cobra.Command, args []string) {
			kcmdutil.CheckErr(options.Validate())

			if err := options.RunAPIServer(stopCh); err != nil {
				if kerrors.IsInvalid(err) {
					if details := err.(*kerrors.StatusError).ErrStatus.Details; details != nil {
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

func (o *WorkspacesAPIServer) AddFlags(flags *pflag.FlagSet) {
	// This command only supports reading from config
	flags.StringVar(&o.KubeConfigFile, "kubeconfig", "", "Kubeconfig of the Kube API server to proxy to.")
	cobra.MarkFlagRequired(flags, "kubeconfig")

	o.SecureServing.AddFlags(flags)
	o.Authentication.AddFlags(flags)
	o.Authorization.AddFlags(flags)
}

func (o *WorkspacesAPIServer) Validate() error {
	errs := []error{}
	if len(o.KubeConfigFile) == 0 {
		errs = append(errs, errors.New("--kubeconfig is required for this command"))
	}
	errs = append(errs, o.SecureServing.Validate()...)
	errs = append(errs, o.Authentication.Validate()...)
	errs = append(errs, o.Authorization.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// RunAPIServer takes the options, starts the API server and waits until stopCh is closed or initial listening fails.
func (o *WorkspacesAPIServer) RunAPIServer(stopCh <-chan struct{}) error {
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
	kcpInformer := kcpinformer.NewSharedInformerFactory(kcpClient, 10 * time.Minute)
	
	utilruntime.Must(tenancyv1alpha1.AddToScheme(legacyscheme.Scheme))
	legacyscheme.Scheme.SetVersionPriority(tenancyv1alpha1.SchemeGroupVersion)
	
	rootAPIServerBuilder := virtualrootapiserver.RootAPIServerBuilder {
		InformerStarts: []func(stopCh <-chan struct{}) {
			kcpInformer.Start,			
		},
		RootPathresolver: func(urlPath string, requestContext context.Context) (accepted bool, prefixToStrip string, completedContext context.Context) {
			completedContext = requestContext
			if path := urlPath; strings.HasPrefix(path, "/services/applications/") {
				path = strings.TrimPrefix(path, "/services/applications/")
				i := strings.Index(path, "/")
				if i == -1 {
					return 
				}
				workspacesScope := path[:i]
				if workspacesScope != "personal" && workspacesScope != "organization" && workspacesScope != "global" {
					return
				}
				return true, "/services/applications/" + workspacesScope + "/", context.WithValue(requestContext, "VirtualWorkspaceWorkspacesScope", workspacesScope)
			}
			return
		},
		GroupAPIServerBuilders: []virtualrootapiserver.GroupAPIServerBuilder {
			{
				GroupVersion: tenancyv1alpha1.SchemeGroupVersion,
				StorageBuilders: map[string]virtualapiserver.RestStorageBuidler{
					"workspaces": func(config virtualapiserver.CompletedConfig) (rest.Storage, error) {
						return virtualworkspacesregistry.NewREST(), nil
					},
				},
			},
		},
	}
	return virtualgenericcmd.RunRootAPIServer(kubeClientConfig, rootAPIServerBuilder, o.SecureServing, o.Authentication, o.Authorization, stopCh)
}
