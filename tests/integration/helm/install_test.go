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

package helm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"istio.io/istio/pkg/test/framework"
	kubecluster "istio.io/istio/pkg/test/framework/components/cluster/kube"
	"istio.io/istio/pkg/test/framework/image"
	"istio.io/istio/pkg/test/helm"
)

// TestDefaultInstall tests Istio installation using Helm with default options
func TestDefaultInstall(t *testing.T) {
	overrideValuesStr := `
global:
  hub: %s
  tag: %s
`
	framework.
		NewTest(t).
		Features("installation.helm.default.install").
		Run(setupInstallation(overrideValuesStr))
}

// TestInstallWithFirstPartyJwt tests Istio installation using Helm
// with first-party-jwt enabled
func TestInstallWithFirstPartyJwt(t *testing.T) {
	overrideValuesStr := `
global:
  hub: %s
  tag: %s
  jwtPolicy: first-party-jwt
`
	framework.
		NewTest(t).
		Features("installation.helm.firstpartyjwt.install").
		Run(setupInstallation(overrideValuesStr))
}

func setupInstallation(overrideValuesStr string) func(t framework.TestContext) {
	return func(t framework.TestContext) {
		workDir, err := t.CreateTmpDirectory("helm-install-test")
		if err != nil {
			t.Fatal("failed to create test directory")
		}
		cs := t.Clusters().Default().(*kubecluster.Cluster)
		h := helm.New(cs.Filename(), ChartPath)
		s, err := image.SettingsFromCommandLine()
		if err != nil {
			t.Fatal(err)
		}
		overrideValues := fmt.Sprintf(overrideValuesStr, s.Hub, s.Tag)
		overrideValuesFile := filepath.Join(workDir, "values.yaml")
		if err := ioutil.WriteFile(overrideValuesFile, []byte(overrideValues), os.ModePerm); err != nil {
			t.Fatalf("failed to write iop cr file: %v", err)
		}
		InstallGatewaysCharts(t, cs, h, "", IstioNamespace, overrideValuesFile)

		VerifyInstallation(t, cs)

		t.Cleanup(func() {
			deleteGatewayCharts(t, h)
		})
	}
}
