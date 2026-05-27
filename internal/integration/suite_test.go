//go:build integration

package integration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/testutil"
)

// integrationCfg is initialized once in BeforeSuite and reused by every spec.
var integrationCfg *testutil.IntegrationTestConfig

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	integrationCfg = testutil.DefaultIntegrationConfig()
	if !testutil.IsMotoAvailable(integrationCfg.MotoEndpoint) {
		Skip("moto server not reachable at " + integrationCfg.MotoEndpoint +
			" — start it with `moto_server -p 5000` or `make moto-start`")
	}
	testutil.BuildClerk(GinkgoT())
})
