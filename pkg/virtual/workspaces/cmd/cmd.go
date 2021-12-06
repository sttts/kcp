package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kcp-dev/kcp/pkg/virtual/generic/builders"
	virtualgenericcmd "github.com/kcp-dev/kcp/pkg/virtual/generic/cmd"
	virtualrootapiserver "github.com/kcp-dev/kcp/pkg/virtual/generic/rootapiserver"
	virtualworkspacesbuilders "github.com/kcp-dev/kcp/pkg/virtual/workspaces/builders"
)

var _ virtualgenericcmd.SubCommandOptions = (*WorkspacesSubCommandOptions)(nil)

type WorkspacesSubCommandOptions struct {
	rootPathPrefix string
}

func (o *WorkspacesSubCommandOptions) Description() virtualgenericcmd.SubCommandDescription {
	return virtualgenericcmd.SubCommandDescription{
		Name:  "workspaces",
		Use:   "workspaces",
		Short: "Launch workspaces virtual workspace apiserver",
		Long:  "Start a virtual workspace apiserver to managing personal, organizational or global workspaces",
	}
}

func (o *WorkspacesSubCommandOptions) AddFlags(flags *pflag.FlagSet) {
	if o == nil {
		return
	}

	flags.StringVar(&o.rootPathPrefix, "workspaces:root-path-prefix", virtualworkspacesbuilders.DefaultRootPathPrefix, ""+
		"The prefix of the workspaces API server root path.\n"+
		"The final workspaces API root path will be of the form:\n    <root-path-prefix>/personal|organization|global")
}

func (o *WorkspacesSubCommandOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errors := []error{}

	if !strings.HasPrefix(o.rootPathPrefix, "/") {
		errors = append(errors, fmt.Errorf("--workspaces:root-path-prefix %v should start with /", o.rootPathPrefix))
	}

	return errors
}

func (o *WorkspacesSubCommandOptions) InitializeBuilders(cc clientcmd.ClientConfig, c *rest.Config) ([]virtualrootapiserver.InformerStart, []builders.VirtualWorkspaceBuilder, error) {
	builders := []builders.VirtualWorkspaceBuilder{
		virtualworkspacesbuilders.WorkspacesVirtualWorkspaceBuilder(o.rootPathPrefix),
	}
	return nil, builders, nil
}
