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
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/go-logr/zerologr"
	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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
	"k8s.io/klog/v2"

	"github.com/kcp-dev/kcp/pkg/cmd/help"
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
			if err := serverOptions.GenericControlPlane.Logs.ValidateAndApply(); err != nil {
				return err
			}

			logger := "zerolog"
			switch logger {
			case "logrus":
				logrusLogr := logrusr.New(logrus.StandardLogger())
				logrus.SetReportCaller(true)
				logrus.SetFormatter(&logrus.TextFormatter{
					CallerPrettyfier: func(f *runtime.Frame) (string, string) {
						filename := path.Base(f.File)
						return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", filepath.Base(filename), f.Line)
					},
				})
				klog.SetLogger(logrusLogr)

			case "zerolog":
				fileColumn := 40
				pad := func(n int) string {
					if n <= 0 {
						return ""
					}
					return strings.Repeat(" ", n)
				}
				zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000"
				zerologr.NameFieldName = "logger"
				zerologr.NameSeparator = "/"
				zerolog.CallerMarshalFunc = func(file string, line int) string {
					at := strings.Index(file, "@")
					if at == -1 {
						lineStr := strconv.Itoa(line)
						slash := strings.LastIndex(file, "/")
						if slash == -1 {
							return file + ":" + lineStr + pad(fileColumn-len(file)-1-len(lineStr))
						}
						return file[slash+1:] + ":" + lineStr + pad(fileColumn-len(file)+slash+1-1-len(lineStr))
					}
					pth := file[at+1:]
					slash := strings.LastIndex(pth, "/")
					if slash != -1 {
						pth = pth[slash+1:]
					}
					pkg := file[:at]
					slash = strings.LastIndex(pkg, "/")
					if slash != -1 {
						pkg = pkg[slash+1:]
					}
					ret := fmt.Sprintf("%s@%s:%d", pkg, pth, line)
					return ret + pad(fileColumn-len(ret))
				}

				zl := zerolog.New(zerolog.ConsoleWriter{
					Out:        os.Stdout,
					TimeFormat: "15:04:05.000",
					FormatMessage: func(i interface{}) string {
						if i == nil {
							return ""
						}
						return strings.TrimSuffix(fmt.Sprintf("%s", i), "\n")
					},
				})
				zl = zl.With().Caller().Timestamp().Logger()

				klog.SetLogger(logr.New(klogZlAdjust{zerologr.NewLogSink(&zl)}))

				/*
					klog.V(9).Info("v9")
					klog.V(5).Info("v5")
					klog.V(4).Info("v4")
					klog.V(3).Info("v3")
					klog.V(2).Info("v2")
					klog.V(1).Info("v1")
					klog.Info("v0")
					klog.Warningf("warning")
					klog.Errorf("error")
					klog.InfoS("structured", "key", "value")
					os.Exit(1)
				*/
			case "zap":
				zapEncCfg := zap.NewDevelopmentEncoderConfig()
				zapEncCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
				zapLog := zap.New(zapcore.NewCore(
					zapcore.NewConsoleEncoder(zapEncCfg),
					zapcore.AddSync(colorable.NewColorableStdout()),
					zapcore.DebugLevel,
				))

				zapLogr := zapr.NewLogger(zapLog)
				klog.SetLogger(zapLogr)
			}

			completed, err := serverOptions.Complete()
			if err != nil {
				return err
			}

			if errs := completed.Validate(); len(errs) > 0 {
				return errors.NewAggregate(errs)
			}

			s, err := server.NewServer(completed)
			if err != nil {
				return err
			}

			return s.Run(genericapiserver.SetupSignalContext())
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

	os.Exit(cli.Run(cmd))
}

type klogZlAdjust struct {
	logr.LogSink
}

func (ls klogZlAdjust) Enabled(level int) bool {
	return ls.LogSink.Enabled(toZeroLogrLevel(level))
}

func (ls klogZlAdjust) Info(level int, msg string, keysAndValues ...interface{}) {
	ls.LogSink.Info(toZeroLogrLevel(level), msg, keysAndValues...)
}

func (ls klogZlAdjust) Error(err error, msg string, keysAndValues ...interface{}) {
	ls.LogSink.Error(err, msg, keysAndValues...)
}

func (ls klogZlAdjust) WithCallDepth(depth int) logr.LogSink {
	if logSink, ok := ls.LogSink.(logr.CallDepthLogSink); ok {
		return klogZlAdjust{logSink.WithCallDepth(depth + 1)}
	}
	return ls
}

func toZeroLogrLevel(level int) int {
	switch level {
	case 4, 5, 6, 7, 8, 9:
		return 2
	case 3:
		return 1
	case 0, 1, 2:
		return 0
	default:
		return level - 1
	}
}
