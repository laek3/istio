// +build integ
// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helmupgrade

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/istio/pkg/test/env"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster"
	kubecluster "istio.io/istio/pkg/test/framework/components/cluster/kube"
	"istio.io/istio/pkg/test/framework/image"
	"istio.io/istio/pkg/test/helm"
	kubetest "istio.io/istio/pkg/test/kube"
	"istio.io/istio/pkg/test/scopes"
	"istio.io/istio/pkg/test/util/retry"
	helmtest "istio.io/istio/tests/integration/helm"
	"istio.io/istio/tests/util/sanitycheck"
)

const (
	gcrHub = "gcr.io/istio-release"

	defaultValues = `
global:
  hub: %s
  tag: %s
`
)

// previousChartPath is path of Helm charts for previous Istio deployments.
var previousChartPath = filepath.Join(env.IstioSrc, "tests/integration/helm/testdata/")

// upgradeCharts upgrades Istio using Helm charts with the provided
// override values file to the latest charts in $ISTIO_SRC/manifests
func upgradeCharts(ctx framework.TestContext, h *helm.Helm, overrideValuesFile string) {
	// Upgrade base chart
	err := h.UpgradeChart(helmtest.BaseReleaseName, filepath.Join(helmtest.ChartPath, helmtest.BaseChart),
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		ctx.Fatalf("failed to upgrade istio %s chart", helmtest.BaseChart)
	}

	// Upgrade discovery chart
	err = h.UpgradeChart(helmtest.IstiodReleaseName, filepath.Join(helmtest.ChartPath, helmtest.ControlChartsDir, helmtest.DiscoveryChart),
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		ctx.Fatalf("failed to upgrade istio %s chart", helmtest.DiscoveryChart)
	}

	// Upgrade ingress gateway chart
	err = h.UpgradeChart(helmtest.IngressReleaseName, filepath.Join(helmtest.ChartPath, helmtest.GatewayChartsDir, helmtest.IngressGatewayChart),
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		ctx.Fatalf("failed to upgrade istio %s chart", helmtest.IngressGatewayChart)
	}

	// Upgrade egress gateway chart
	err = h.UpgradeChart(helmtest.EgressReleaseName, filepath.Join(helmtest.ChartPath, helmtest.GatewayChartsDir, helmtest.EgressGatewayChart),
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		ctx.Fatalf("failed to upgrade istio %s chart", helmtest.EgressGatewayChart)
	}
}

// deleteIstio deletes installed Istio Helm charts and resources
func deleteIstio(cs cluster.Cluster, h *helm.Helm) error {
	scopes.Framework.Infof("cleaning up resources")
	if err := h.DeleteChart(helmtest.EgressReleaseName, helmtest.IstioNamespace); err != nil {
		return fmt.Errorf("failed to delete %s release", helmtest.EgressReleaseName)
	}
	if err := h.DeleteChart(helmtest.IngressReleaseName, helmtest.IstioNamespace); err != nil {
		return fmt.Errorf("failed to delete %s release", helmtest.IngressReleaseName)
	}
	if err := h.DeleteChart(helmtest.IstiodReleaseName, helmtest.IstioNamespace); err != nil {
		return fmt.Errorf("failed to delete %s release", helmtest.IngressReleaseName)
	}
	if err := h.DeleteChart(helmtest.BaseReleaseName, helmtest.IstioNamespace); err != nil {
		return fmt.Errorf("failed to delete %s release", helmtest.BaseReleaseName)
	}
	if err := cs.CoreV1().Namespaces().Delete(context.TODO(), helmtest.IstioNamespace, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete istio namespace: %v", err)
	}
	if err := kubetest.WaitForNamespaceDeletion(cs, helmtest.IstioNamespace, retry.Timeout(helmtest.RetryTimeOut)); err != nil {
		return fmt.Errorf("wating for istio namespace to be deleted: %v", err)
	}

	return nil
}

// getValuesOverrides returns the the values file created to pass into Helm override default values
// for the hub and tag
func getValuesOverrides(ctx framework.TestContext, valuesStr, hub, tag string) string {
	workDir := ctx.CreateTmpDirectoryOrFail("helm")
	overrideValues := fmt.Sprintf(valuesStr, hub, tag)
	overrideValuesFile := filepath.Join(workDir, "values.yaml")
	if err := ioutil.WriteFile(overrideValuesFile, []byte(overrideValues), os.ModePerm); err != nil {
		ctx.Fatalf("failed to write iop cr file: %v", err)
	}

	return overrideValuesFile
}

// installIstio install Istio using Helm charts with the provided
// override values file and fails the tests on any failures.
func installIstio(t framework.TestContext, cs cluster.Cluster,
	h *helm.Helm, overrideValuesFile string) {
	helmtest.CreateNamespace(t, cs, helmtest.IstioNamespace)

	// Install base chart
	err := h.InstallChart(helmtest.BaseReleaseName, helmtest.BaseChart+helmtest.TarGzSuffix,
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		t.Errorf("failed to install istio %s chart", helmtest.BaseChart)
	}

	// Install discovery chart
	err = h.InstallChart(helmtest.IstiodReleaseName, filepath.Join(helmtest.ControlChartsDir, helmtest.DiscoveryChart)+helmtest.TarGzSuffix,
		helmtest.IstioNamespace, overrideValuesFile, helmtest.Timeout)
	if err != nil {
		t.Errorf("failed to install istio %s chart", helmtest.DiscoveryChart)
	}

	helmtest.InstallGatewaysCharts(t, cs, h, helmtest.TarGzSuffix, helmtest.IstioNamespace, overrideValuesFile)
}

// performUpgradeFunc returns the provided function necessary to run inside of a integration test
// for upgrade capability
func performUpgradeFunc(previousVersion string) func(framework.TestContext) {
	return func(t framework.TestContext) {
		cs := t.Clusters().Default().(*kubecluster.Cluster)
		h := helm.New(cs.Filename(), filepath.Join(previousChartPath, previousVersion))

		t.ConditionalCleanup(func() {
			// only need to do call this once as helm doesn't need to remove
			// all versions
			err := deleteIstio(cs, h)
			if err != nil {
				t.Fatalf("could not delete istio: %v", err)
			}
		})

		overrideValuesFile := getValuesOverrides(t, defaultValues, gcrHub, previousVersion)
		installIstio(t, cs, h, overrideValuesFile)
		helmtest.VerifyInstallation(t, cs)

		oldClient, oldServer := sanitycheck.SetupTrafficTest(t, t)
		sanitycheck.RunTrafficTestClientServer(t, oldClient, oldServer)

		// now upgrade istio to the latest version found in this branch
		// use the command line or environmental vars from the user to set
		// the hub/tag
		s, err := image.SettingsFromCommandLine()
		if err != nil {
			t.Fatal(err)
		}

		overrideValuesFile = getValuesOverrides(t, defaultValues, s.Hub, s.Tag)
		upgradeCharts(t, h, overrideValuesFile)
		helmtest.VerifyInstallation(t, cs)

		newClient, newServer := sanitycheck.SetupTrafficTest(t, t)
		sanitycheck.RunTrafficTestClientServer(t, newClient, newServer)

		// now check that we are compatible with N-1 proxy with N proxy
		sanitycheck.RunTrafficTestClientServer(t, oldClient, newServer)
	}
}
