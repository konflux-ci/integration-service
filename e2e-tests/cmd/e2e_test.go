//go:build e2e

package cmd

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"

	"github.com/onsi/gomega"

	_ "github.com/konflux-ci/integration-service/e2e-tests/tests/integration-service"

	"flag"

	"k8s.io/klog/v2"
)

func init() {

	klog.SetLogger(ginkgo.GinkgoLogr)

	verbosity := 1
	if v, err := strconv.ParseUint(os.Getenv("KLOG_VERBOSITY"), 10, 8); err == nil {
		verbosity = int(v)
	}

	flags := &flag.FlagSet{}
	klog.InitFlags(flags)
	if err := flags.Set("v", fmt.Sprintf("%d", verbosity)); err != nil {
		panic(err)
	}
}

func TestE2E(t *testing.T) {
	klog.Info("Starting Red Hat App Studio e2e tests...")
	gomega.RegisterFailHandler(ginkgo.Fail)

	//GinkgoConfiguration returns a SuiteConfig and ReportConfig.
	//We don't need the ReportConfig object so I'm ignoring it.
	config, _ := ginkgo.GinkgoConfiguration()
	if config.DryRun {
		reports := ginkgo.PreviewSpecs("Red Hat App Studio E2E tests")

		for _, spec := range reports.SpecReports {
			if spec.State.Is(types.SpecStatePassed) {

				klog.Info(spec.FullText())
			}

		}

	} else {
		ginkgo.RunSpecs(t, "Red Hat App Studio E2E tests")
	}
}
