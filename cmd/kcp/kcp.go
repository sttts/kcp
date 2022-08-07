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

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"

	"k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/config"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/term"
	"k8s.io/component-base/version"
	"k8s.io/klog/v2"

	"github.com/kcp-dev/kcp/pkg/cmd/help"
	"github.com/kcp-dev/kcp/pkg/embeddedetcd"
	kcpfeatures "github.com/kcp-dev/kcp/pkg/features"
	"github.com/kcp-dev/kcp/pkg/server"
	"github.com/kcp-dev/kcp/pkg/server/options"
)

func main() {
	cmd := &cobra.Command{
		Use:   "kcp",
		Short: "Kube for Control Plane (KCP)",
		Long: help.Doc(`
			KCP is the easiest way to manage Kubernetes applications against one or
			more clusters, by giving you a personal control plane that schedules your
			workloads onto one or many clusters, and making it simple to pick up and
			move. Advanced use cases including spreading your apps across clusters for
			resiliency, scheduling batch workloads onto clusters with free capacity,
			and enabling collaboration for individual teams without having access to
			the underlying clusters.

			To get started, launch a new cluster with 'kcp start', which will
			initialize your personal control plane and write an admin kubeconfig file
			to disk.
		`),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())

	// manually extract root directory from flags first as it influence all other flags
	rootDir := ".kcp"
	for i, f := range os.Args {
		if f == "--root-directory" {
			if i < len(os.Args)-1 {
				rootDir = os.Args[i+1]
			} // else let normal flag processing fail
		} else if strings.HasPrefix(f, "--root-directory=") {
			rootDir = strings.TrimPrefix(f, "--root-directory=")
		}
	}

	serverOptions := options.NewOptions(rootDir)
	serverOptions.GenericControlPlane.Logs.Config.Verbosity = config.VerbosityLevel(2)

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the control plane process",
		Long: help.Doc(`
			Start the control plane process

			The server process listens on port 6443 and will act like a Kubernetes
			API server. It will initialize any necessary data to the provided start
			location or as a '.kcp' directory in the current directory. An admin
			kubeconfig file will be generated at initialization time that may be
			used to access the control plane.
		`),
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// silence client-go warnings.
			// apiserver loopback clients should not log self-issued warnings.
			rest.SetDefaultWarningHandler(rest.NoWarnings{})
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// run as early as possible to avoid races later when some components (e.g. grpc) start early using klog
			if err := serverOptions.GenericControlPlane.Logs.ValidateAndApply(kcpfeatures.DefaultFeatureGate); err != nil {
				return err
			}

			// setup zap
			encoderConfig := zap.NewDevelopmentEncoderConfig()
			encoderConfig.EncodeLevel = capitalColorLevelEncoder
			encoderConfig.ConsoleSeparator = " "
			encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
			zapLog := zap.New(zapcore.NewCore(
				colorEncoder{Encoder: zapcore.NewConsoleEncoder(encoderConfig)},
				zapcore.AddSync(os.Stderr),
				zapcore.DebugLevel,
			))
			zapLogr := zapr.NewLogger(zapLog)
			klog.SetLogger(logr.New(trimNewlines{zapLogr.GetSink()}))

			completed, err := serverOptions.Complete()
			if err != nil {
				return err
			}

			if errs := completed.Validate(); len(errs) > 0 {
				return errors.NewAggregate(errs)
			}

			config, err := server.NewConfig(completed)
			if err != nil {
				return err
			}

			completedConfig, err := config.Complete()
			if err != nil {
				return err
			}

			ctx := genericapiserver.SetupSignalContext()

			// the etcd server must be up before NewServer because storage decorators access it right away
			if completedConfig.EmbeddedEtcd.Config != nil {
				if err := embeddedetcd.NewServer(completedConfig.EmbeddedEtcd).Run(ctx); err != nil {
					return err
				}
			}

			s, err := server.NewServer(completedConfig)
			if err != nil {
				return err
			}
			return s.Run(ctx)
		},
	}

	// add start named flag sets to start flags
	namedStartFlagSets := serverOptions.Flags()
	globalflag.AddGlobalFlags(namedStartFlagSets.FlagSet("global"), cmd.Name(), logs.SkipLoggingConfigurationFlags())
	startFlags := startCmd.Flags()
	for _, f := range namedStartFlagSets.FlagSets {
		startFlags.AddFlagSet(f)
	}

	startOptionsCmd := &cobra.Command{
		Use:   "options",
		Short: "Show all start command options",
		Long: help.Doc(`
			Show all start command options

			"kcp start"" has a large number of options. This command shows all of them.
		`),
		PersistentPreRunE: func(*cobra.Command, []string) error {
			// silence client-go warnings.
			// apiserver loopback clients should not log self-issued warnings.
			rest.SetDefaultWarningHandler(rest.NoWarnings{})
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStderr(), usageFmt, startCmd.UseLine())
			cliflag.PrintSections(cmd.OutOrStderr(), namedStartFlagSets, cols)
			return nil
		},
	}
	startCmd.AddCommand(startOptionsCmd)
	cmd.AddCommand(startCmd)

	setPartialUsageAndHelpFunc(startCmd, namedStartFlagSets, cols, []string{
		"etcd-servers",
		"run-controllers",
		"run-virtual-workspaces",
	})

	help.FitTerminal(cmd.OutOrStdout())

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	os.Exit(cli.Run(cmd))
}

// trimNewlines implements a logr.Logger that removes trailing msg newlines.
type trimNewlines struct {
	logr.LogSink // already initialized
}

func (s trimNewlines) Init(info logr.RuntimeInfo) {}
func (s trimNewlines) Info(level int, msg string, keysAndValues ...interface{}) {
	s.LogSink.Info(level, strings.TrimRight(msg, "\n"), keysAndValues...)
}
func (s trimNewlines) Error(err error, msg string, keysAndValues ...interface{}) {
	s.LogSink.Error(err, strings.TrimRight(msg, "\n"), keysAndValues...)
}

type colorEncoder struct {
	zapcore.Encoder
}

func (e colorEncoder) Clone() zapcore.Encoder {
	return colorEncoder{Encoder: e.Encoder.Clone()}
}

var (
	messageColor = color.New(color.FgWhite).SprintFunc()
)

func (e colorEncoder) EncodeEntry(ent zapcore.Entry, flds []zapcore.Field) (*buffer.Buffer, error) {
	ent.Message = messageColor(ent.Message)

	return e.Encoder.EncodeEntry(ent, flds)
}

var (
	levelToColor = map[zapcore.Level]color.Attribute{
		zapcore.DebugLevel:  color.FgMagenta,
		zapcore.InfoLevel:   color.FgBlue,
		zapcore.WarnLevel:   color.FgYellow,
		zapcore.ErrorLevel:  color.FgRed,
		zapcore.DPanicLevel: color.FgRed,
		zapcore.PanicLevel:  color.FgRed,
		zapcore.FatalLevel:  color.FgRed,
	}
	unknownLevelColor         = color.New(color.FgRed).Add(color.Bold)
	levelToCapitalColorString = make(map[zapcore.Level]string, len(levelToColor))
)

func capitalColorLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	s, ok := levelToCapitalColorString[l]
	if !ok {
		s = unknownLevelColor.Sprintf("%-5s", l.CapitalString())
	}
	enc.AppendString(s)
}

func init() {
	for level, c := range levelToColor {
		levelToCapitalColorString[level] = color.New(c).Add(color.Bold).Sprintf("%-5s", level.CapitalString())
	}
}
