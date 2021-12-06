/*
Copyright 2021 The KCP Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
