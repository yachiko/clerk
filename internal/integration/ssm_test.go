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

// run30s wraps the common 30-second context pattern used by most specs.
func run30s(home string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return testutil.RunClerkInHome(ctx, integrationCfg, home, args...)
}

var _ = Describe("clerk against moto", func() {
	var home string

	BeforeEach(func() {
		Expect(testutil.ResetMoto(integrationCfg.MotoEndpoint)).To(Succeed())
		home = GinkgoT().TempDir()
	})

	Describe("put + get round-trip", func() {
		It("creates a SecureString secret and reads it back", func() {
			stdout, stderr, err := run30s(home, "put", "/test/integration/secret", "my-secret-value")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(stdout).To(ContainSubstring("Created"))
			Expect(stdout).To(ContainSubstring("/test/integration/secret"))

			stdout, _, err = run30s(home, "get", "/test/integration/secret", "--value")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal("my-secret-value\n"))
		})

		It("bumps the version on a second put", func() {
			_, stderr, err := run30s(home, "put", "/test/v/secret", "v1")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

			stdout, _, err := run30s(home, "put", "/test/v/secret", "v2")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(ContainSubstring("Updated"))
			Expect(stdout).To(ContainSubstring("version 2"))

			stdout, _, err = run30s(home, "get", "/test/v/secret", "--value")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal("v2\n"))
		})

		It("attaches tags and surfaces them in JSON output", func() {
			_, _, err := run30s(home, "put", "/test/tagged/secret", "v", "--tags", "env=test,team=backend")
			Expect(err).NotTo(HaveOccurred())

			stdout, _, err := run30s(home, "get", "/test/tagged/secret", "--output", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(SatisfyAll(
				ContainSubstring(`"env"`),
				ContainSubstring(`"test"`),
				ContainSubstring(`"team"`),
				ContainSubstring(`"backend"`),
			))
		})
	})

	Describe("get with version selector", func() {
		BeforeEach(func() {
			for _, v := range []string{"version-1", "version-2"} {
				_, _, err := run30s(home, "put", "/test/versions/secret", v)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("returns the requested version when @N is given", func() {
			stdout, _, err := run30s(home, "get", "/test/versions/secret@1", "--value")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal("version-1\n"))
		})

		It("returns the latest version when no @ is given", func() {
			stdout, _, err := run30s(home, "get", "/test/versions/secret", "--value")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal("version-2\n"))
		})
	})

	Describe("get with --mask", func() {
		It("redacts the value", func() {
			_, _, err := run30s(home, "put", "/test/mask/secret", "sensitive-data-123")
			Expect(err).NotTo(HaveOccurred())

			stdout, _, err := run30s(home, "get", "/test/mask/secret", "--mask")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).NotTo(ContainSubstring("sensitive-data-123"))
			Expect(stdout).To(ContainSubstring("*"))
		})
	})

	Describe("delete", func() {
		It("removes the parameter and a subsequent get fails", func() {
			_, _, err := run30s(home, "put", "/test/del/secret", "bye")
			Expect(err).NotTo(HaveOccurred())

			stdout, stderr, err := run30s(home, "delete", "/test/del/secret", "--force")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(strings.ToLower(stdout)).To(ContainSubstring("deleted"))

			_, _, err = run30s(home, "get", "/test/del/secret")
			Expect(err).To(HaveOccurred(), "get must fail after delete")
		})
	})

	Describe("list", func() {
		var (
			gen     *testutil.FixtureGenerator
			created []string
		)

		BeforeEach(func() {
			var err error
			gen, err = testutil.NewFixtureGenerator(&testutil.FixtureConfig{
				Endpoint: integrationCfg.MotoEndpoint,
				Region:   integrationCfg.MotoRegion,
			})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			created, err = gen.GenerateSpecificParameters(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = gen.CleanupParameters(ctx, created)
		})

		It("filters by /dev/* glob", func() {
			stdout, stderr, err := run30s(home, "list", "/dev/*")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(stdout).To(SatisfyAll(
				ContainSubstring("/dev/"),
				Not(ContainSubstring("/prod/")),
			))
		})

		It("filters by /prod/* glob", func() {
			stdout, _, err := run30s(home, "list", "/prod/*")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(SatisfyAll(
				ContainSubstring("/prod/"),
				Not(ContainSubstring("/dev/")),
			))
		})

		It("returns everything when no pattern is supplied", func() {
			stdout, _, err := run30s(home, "list")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(SatisfyAll(
				ContainSubstring("/dev/"),
				ContainSubstring("/prod/"),
				ContainSubstring("/staging/"),
			))
		})
	})

	Describe("cp", func() {
		// SecureString round-trips through moto break here because moto's mock
		// encryption prepends "kms:alias/aws/ssm:" when withDecryption=false,
		// and clerk's cp reads with withDecryption=false on purpose. Use String
		// type to dodge the moto quirk; against real AWS this isn't an issue.
		It("duplicates a parameter to a new path", func() {
			_, _, err := run30s(home, "put", "/test/cp/src", "the-value", "--type", "String")
			Expect(err).NotTo(HaveOccurred())

			stdout, stderr, err := run30s(home, "cp", "/test/cp/src", "/test/cp/dst")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(strings.ToLower(stdout)).To(ContainSubstring("copied"))

			for _, name := range []string{"/test/cp/src", "/test/cp/dst"} {
				out, _, err := run30s(home, "get", name, "--value")
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(Equal("the-value\n"))
			}
		})
	})

	Describe("mv", func() {
		It("moves the parameter and source is no longer readable", func() {
			_, _, err := run30s(home, "put", "/test/mv/src", "movee", "--type", "String")
			Expect(err).NotTo(HaveOccurred())

			stdout, stderr, err := run30s(home, "mv", "/test/mv/src", "/test/mv/dst", "--force")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(strings.ToLower(stdout)).To(ContainSubstring("moved"))

			out, _, err := run30s(home, "get", "/test/mv/dst", "--value")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal("movee\n"))

			_, _, err = run30s(home, "get", "/test/mv/src")
			Expect(err).To(HaveOccurred(), "source must be gone after mv")
		})
	})

	Describe("refresh", func() {
		It("rebuilds the cache against the current AWS state", func() {
			gen, err := testutil.NewFixtureGenerator(&testutil.FixtureConfig{
				Endpoint: integrationCfg.MotoEndpoint,
				Region:   integrationCfg.MotoRegion,
			})
			Expect(err).NotTo(HaveOccurred())
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			created, err := gen.GenerateSpecificParameters(ctx)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = gen.CleanupParameters(context.Background(), created) })

			stdout, stderr, err := run30s(home, "refresh")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
			Expect(strings.ToLower(stdout)).To(ContainSubstring("refresh"))
		})
	})

	Describe("--output json", func() {
		It("emits a JSON object with the parameter's name and value", func() {
			_, _, err := run30s(home, "put", "/test/json/secret", "json-value")
			Expect(err).NotTo(HaveOccurred())

			stdout, _, err := run30s(home, "get", "/test/json/secret", "--output", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(SatisfyAll(
				ContainSubstring(`"name"`),
				ContainSubstring(`"/test/json/secret"`),
				ContainSubstring(`"value"`),
			))
		})
	})

	Describe("error cases", func() {
		It("fails when getting a non-existent parameter", func() {
			_, stderr, err := run30s(home, "get", "/never/created")
			Expect(err).To(HaveOccurred())
			Expect(stderr).NotTo(BeEmpty())
		})

		It("fails when deleting a non-existent parameter", func() {
			_, _, err := run30s(home, "delete", "/never/created", "--force")
			Expect(err).To(HaveOccurred())
		})

		It("fails when requesting a version that doesn't exist", func() {
			_, _, err := run30s(home, "put", "/test/badver/secret", "value")
			Expect(err).NotTo(HaveOccurred())
			_, _, err = run30s(home, "get", "/test/badver/secret@99")
			Expect(err).To(HaveOccurred(), "version 99 doesn't exist")
		})
	})
})
