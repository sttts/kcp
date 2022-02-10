/*
Copyright 2022 The KCP Authors.

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

package plugin

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
)

// Options provides options that will drive the update of the current context
// on a user's KUBECONFIG based on actions done on KCP workspaces
type Options struct {
	WorkspaceDirectoryOverrides *clientcmd.ConfigOverrides
	KubectlOverrides            *clientcmd.ConfigOverrides

	genericclioptions.IOStreams
}

// NewOptions provides an instance of Options with default values
func NewOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		WorkspaceDirectoryOverrides: &clientcmd.ConfigOverrides{},
		KubectlOverrides:            &clientcmd.ConfigOverrides{},

		IOStreams: streams,
	}
}

// BindFlags binds the arguments common to all sub-commands,
// to the corresponding main command flags
func (o *Options) BindFlags(cmd *cobra.Command) {
	// We add only a subset of kubeconfig-related flags to the plugin.
	// All those with with LongName == "" will be ignored.
	kubectlConfigOverrideFlags := clientcmd.RecommendedConfigOverrideFlags("")
	kubectlConfigOverrideFlags.AuthOverrideFlags.ClientCertificate.LongName = ""
	kubectlConfigOverrideFlags.AuthOverrideFlags.ClientKey.LongName = ""
	kubectlConfigOverrideFlags.AuthOverrideFlags.Impersonate.LongName = ""
	kubectlConfigOverrideFlags.AuthOverrideFlags.ImpersonateGroups.LongName = ""
	kubectlConfigOverrideFlags.ContextOverrideFlags.AuthInfoName.LongName = ""
	kubectlConfigOverrideFlags.ContextOverrideFlags.ClusterName.LongName = ""
	kubectlConfigOverrideFlags.ContextOverrideFlags.Namespace.LongName = ""
	kubectlConfigOverrideFlags.Timeout.LongName = ""

	clientcmd.BindOverrideFlags(o.KubectlOverrides, cmd.PersistentFlags(), kubectlConfigOverrideFlags)

	// We also add a subset of kubeconfig-related flags related specifically to
	// workspace directory (user workspace list). They would override the way the
	// `workspaces` virtual workspace API server is accessed.
	descriptionSuffix := " for workspace directory context"
	workspaceDirectoryConfigOverrideFlags := clientcmd.RecommendedConfigOverrideFlags("workspace-directory-")

	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.ClientCertificate.LongName = ""
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.ClientKey.LongName = ""
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.Impersonate.LongName = ""
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.ImpersonateGroups.LongName = ""
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.Password.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.Token.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.AuthOverrideFlags.Username.Description += descriptionSuffix

	workspaceDirectoryConfigOverrideFlags.ContextOverrideFlags.AuthInfoName.LongName = ""
	workspaceDirectoryConfigOverrideFlags.ContextOverrideFlags.ClusterName.LongName = ""
	workspaceDirectoryConfigOverrideFlags.ContextOverrideFlags.Namespace.LongName = ""

	workspaceDirectoryConfigOverrideFlags.ClusterOverrideFlags.APIVersion.LongName = ""
	workspaceDirectoryConfigOverrideFlags.ClusterOverrideFlags.APIServer.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.ClusterOverrideFlags.CertificateAuthority.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.ClusterOverrideFlags.InsecureSkipTLSVerify.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.ClusterOverrideFlags.TLSServerName.Description += descriptionSuffix

	workspaceDirectoryConfigOverrideFlags.CurrentContext.Description += descriptionSuffix
	workspaceDirectoryConfigOverrideFlags.CurrentContext.Default = "workspace-directory"
	workspaceDirectoryConfigOverrideFlags.Timeout.LongName = ""

	clientcmd.BindOverrideFlags(o.WorkspaceDirectoryOverrides, cmd.PersistentFlags(), workspaceDirectoryConfigOverrideFlags)
}
