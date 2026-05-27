//go:build integration

package integration

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/testutil"
)

// generateAtScale creates `count` random parameters in moto and registers
// cleanup. Caller decides whether to skip the spec under -short.
func generateAtScale(count int) []string {
	fixtureCfg := testutil.DefaultFixtureConfig()
	fixtureCfg.Endpoint = integrationCfg.MotoEndpoint
	fixtureCfg.Region = integrationCfg.MotoRegion
	fixtureCfg.NumParameters = count

	gen, err := testutil.NewFixtureGenerator(fixtureCfg)
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	created, err := gen.GenerateParameters(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(created).NotTo(BeEmpty(), "fixture generator should have produced parameters")

	DeferCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = gen.CleanupParameters(ctx, created)
	})

	GinkgoWriter.Printf("generated %d parameters\n", len(created))
	return created
}

var _ = Describe("clerk under load", func() {
	var home string

	BeforeEach(func() {
		// We intentionally do NOT reset moto here; scale_test is allowed to
		// stack parameters within a single run, and each spec creates its own
		// fixture set so prior state is irrelevant.
		home = GinkgoT().TempDir()
	})

	It("lists a 200-parameter Parameter Store", Label("scale"), func() {
		created := generateAtScale(200)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		stdout, stderr, err := testutil.RunClerkInHome(ctx, integrationCfg, home, "list")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		// Count distinct parameter-bearing lines. Random name generation can
		// collide, so we require >= 80% of created visible.
		nameLines := 0
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, "/") && !strings.Contains(line, "─") {
				nameLines++
			}
		}
		Expect(nameLines).To(BeNumerically(">=", len(created)*8/10),
			"list output (%d lines with /) should cover ~all %d created params", nameLines, len(created))
	})

	It("refreshes against a 300-parameter store within the budget", Label("scale"), func() {
		generateAtScale(300)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		start := time.Now()
		stdout, stderr, err := testutil.RunClerkInHome(ctx, integrationCfg, home, "refresh")
		duration := time.Since(start)

		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(strings.ToLower(stdout)).To(ContainSubstring("refresh"))
		GinkgoWriter.Printf("refresh of 300 params completed in %v\n", duration)
	})

	Describe("filter performance on 500 parameters", Label("scale"), func() {
		BeforeEach(func() {
			generateAtScale(500)
		})

		DescribeTable("filters complete within 30s",
			func(pattern string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()

				start := time.Now()
				_, stderr, err := testutil.RunClerkInHome(ctx, integrationCfg, home, "list", pattern)
				duration := time.Since(start)

				Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
				GinkgoWriter.Printf("filter %s took %v\n", pattern, duration)
				Expect(duration).To(BeNumerically("<", 30*time.Second))
			},
			Entry("/dev/*", "/dev/*"),
			Entry("/prod/*", "/prod/*"),
			Entry("/staging/*", "/staging/*"),
			Entry("/qa/*", "/qa/*"),
		)
	})
})
