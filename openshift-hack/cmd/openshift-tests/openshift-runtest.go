package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/library-go/pkg/serviceability"
	testginkgo "github.com/openshift/origin/pkg/test/ginkgo"
	exutil "github.com/openshift/origin/test/extended/util"
)

func main() {
	// KUBE_TEST_REPO_LIST is calculated during package initialization and prevents
	// proper mirroring of images referenced by tests. Clear the value and re-exec the
	// current process to ensure we can verify from a known state.
	if len(os.Getenv("KUBE_TEST_REPO_LIST")) > 0 {
		fmt.Fprintln(os.Stderr, "warning: KUBE_TEST_REPO_LIST may not be set when using openshift-tests and will be ignored")
		os.Setenv("KUBE_TEST_REPO_LIST", "")
		// resolve the call to execute since Exec() does not do PATH resolution
		if err := syscall.Exec(exec.Command(os.Args[0]).Path, os.Args, os.Environ()); err != nil {
			panic(fmt.Sprintf("%s: %v", os.Args[0], err))
		}
		return
	}

	logs.InitLogs()
	defer logs.FlushLogs()

	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	//pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	root := &cobra.Command{
		Long: templates.LongDesc(`
		OpenShift Tests

		This command verifies behavior of an OpenShift cluster by running remote tests against
		the cluster API that exercise functionality. In general these tests may be disruptive
		or require elevated privileges - see the descriptions of each test suite.
		`),
	}

	root.AddCommand(
		newRunTestCommand(),
		newListTestsCommand(),
	)

	f := flag.CommandLine.Lookup("v")
	root.PersistentFlags().AddGoFlag(f)
	pflag.CommandLine = pflag.NewFlagSet("empty", pflag.ExitOnError)
	flag.CommandLine = flag.NewFlagSet("empty", flag.ExitOnError)
	exutil.InitStandardFlags()

	if err := func() error {
		defer serviceability.Profile(os.Getenv("OPENSHIFT_PROFILE")).Stop()
		return root.Execute()
	}(); err != nil {
		if ex, ok := err.(testginkgo.ExitError); ok {
			fmt.Fprintf(os.Stderr, "Ginkgo exit error %d: %v\n", ex.Code, err)
			os.Exit(ex.Code)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRunTestCommand() *cobra.Command {
	testOpt := testginkgo.NewTestOptions(os.Stdout, os.Stderr)

	cmd := &cobra.Command{
		Use:   "run-test NAME",
		Short: "Run a single test by name",
		Long: templates.LongDesc(`
		Execute a single test

		This executes a single test by name. It is used by the run command during suite execution but may also
		be used to test in isolation while developing new tests.
		`),

		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if v := os.Getenv("TEST_LOG_LEVEL"); len(v) > 0 {
				cmd.Flags().Lookup("v").Value.Set(v)
			}

			if err := verifyImagesWithoutEnv(); err != nil {
				return err
			}

			config, err := decodeProvider(os.Getenv("TEST_PROVIDER"), testOpt.DryRun, false, nil)
			if err != nil {
				return err
			}
			if err := initializeTestFramework(exutil.TestContext, config, testOpt.DryRun); err != nil {
				return err
			}
			klog.V(4).Infof("Loaded test configuration: %#v", exutil.TestContext)

			exutil.TestContext.ReportDir = os.Getenv("TEST_JUNIT_DIR")

			// allow upgrade test to pass some parameters here, although this may be
			// better handled as an env var within the test itself in the future
			/*
				if err := upgradeTestPreTest(); err != nil {
					return err
				}
			*/

			exutil.WithCleanup(func() { err = testOpt.Run(args) })
			return err
		},
	}
	cmd.Flags().BoolVar(&testOpt.DryRun, "dry-run", testOpt.DryRun, "Print the test to run without executing them.")
	return cmd
}

func newListTestsCommand() *cobra.Command {
	opt := testginkgo.ListOptions{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available tests",
		Long: templates.LongDesc(`
		List the available tests in this binary
		`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opt.ListTests()
		},
	}
	cmd.Flags().StringVar(&opt.TestListPath, "test-list-path", "", "Output path for temporary list of tests provided by this binary")

	return cmd
}

func bindTestOptions(opt *testginkgo.Options, flags *pflag.FlagSet) {
	flags.BoolVar(&opt.DryRun, "dry-run", opt.DryRun, "Print the tests to run without executing them.")
	flags.BoolVar(&opt.PrintCommands, "print-commands", opt.PrintCommands, "Print the sub-commands that would be executed instead.")
	flags.StringVar(&opt.JUnitDir, "junit-dir", opt.JUnitDir, "The directory to write test reports to.")
	flags.StringVarP(&opt.TestFile, "file", "f", opt.TestFile, "Create a suite from the newline-delimited test names in this file.")
	flags.StringVar(&opt.Regex, "run", opt.Regex, "Regular expression of tests to run.")
	flags.StringVarP(&opt.OutFile, "output-file", "o", opt.OutFile, "Write all test output to this file.")
	flags.IntVar(&opt.Count, "count", opt.Count, "Run each test a specified number of times. Defaults to 1 or the suite's preferred value. -1 will run forever.")
	flags.BoolVar(&opt.FailFast, "fail-fast", opt.FailFast, "If a test fails, exit immediately.")
	flags.DurationVar(&opt.Timeout, "timeout", opt.Timeout, "Set the maximum time a test can run before being aborted. This is read from the suite by default, but will be 10 minutes otherwise.")
	flags.BoolVar(&opt.IncludeSuccessOutput, "include-success", opt.IncludeSuccessOutput, "Print output from successful tests.")
	flags.IntVar(&opt.Parallelism, "max-parallel-tests", opt.Parallelism, "Maximum number of tests running in parallel. 0 defaults to test suite recommended value, which is different in each suite.")
}
