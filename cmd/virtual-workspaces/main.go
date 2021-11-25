package main

import (
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	genericapiserver "k8s.io/apiserver/pkg/server"

	virtualworkspacescmd "github.com/kcp-dev/kcp/pkg/virtual/workspaces/cmd"
)

// wordSepNormalizeFunc changes all flags that contain "_" separators
func wordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}


func main() {
	stopCh := genericapiserver.SetupSignalHandler()

	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(wordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)


	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	command := NewVirtualWorkspaceApiServerCommand(stopCh)
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func NewVirtualWorkspaceApiServerCommand(stopCh <-chan struct{}) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "virtual-workspaces",
		Short: "Command for virtual workspaces API Servers",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}
	workspaces := virtualworkspacescmd.NewWorkspacesAPIServerCommand(os.Stdout, os.Stderr, stopCh)
	cmd.AddCommand(workspaces)

	return cmd
}
