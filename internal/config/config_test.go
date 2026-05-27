package config

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/testutil"
)

var _ = Describe("Manager", func() {
	var (
		home string
		mgr  *Manager
	)

	BeforeEach(func() {
		home = testutil.IsolateHome(GinkgoT())
		var err error
		mgr, err = NewManager()
		Expect(err).NotTo(HaveOccurred())
		Expect(mgr).NotTo(BeNil())
	})

	Describe("on a fresh HOME with no config file", func() {
		It("returns the baked-in defaults", func() {
			cfg := mgr.Get()
			Expect(cfg.Region).To(Equal("us-east-1"))
			Expect(cfg.Profile).To(BeEmpty())
		})

		It("computes the cache_path relative to HOME", func() {
			Expect(mgr.Get().CachePath).To(Equal(filepath.Join(home, ".clerk", "cache.json")))
		})
	})

	Describe("Save", func() {
		It("persists mutations and reloads them in a fresh manager", func() {
			cfg := mgr.Get()
			cfg.Region = "eu-west-1"
			cfg.Profile = "production"
			cfg.CacheTTL = 1 * time.Hour
			cfg.ParallelFetches = 25

			Expect(mgr.Save()).To(Succeed())

			configPath := filepath.Join(home, ".clerk", "config.json")
			Expect(configPath).To(BeAnExistingFile())

			info, err := os.Stat(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)),
				"config files contain credentials/profile names; must be user-only")

			mgr2, err := NewManager()
			Expect(err).NotTo(HaveOccurred())
			cfg2 := mgr2.Get()
			Expect(cfg2.Region).To(Equal("eu-west-1"))
			Expect(cfg2.Profile).To(Equal("production"))
			Expect(cfg2.CacheTTL).To(Equal(1 * time.Hour))
			Expect(cfg2.ParallelFetches).To(Equal(25))
		})
	})

	Describe("GetValue", func() {
		DescribeTable("reads defaults",
			func(key, expected string) {
				v, err := mgr.GetValue(key)
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(expected))
			},
			Entry("region", "region", "us-east-1"),
			Entry("profile (empty)", "profile", ""),
			Entry("default_type", "default_type", "SecureString"),
			Entry("dotted key default.type", "default.type", "SecureString"),
			Entry("default_sort", "default_sort", "name"),
			Entry("dotted key default.sort", "default.sort", "name"),
			Entry("parallel_fetches", "parallel_fetches", "10"),
			Entry("cache_ttl (formatted duration)", "cache_ttl", "3h0m0s"),
			Entry("clipboard_timeout", "clipboard_timeout", "1m0s"),
			Entry("search_slash_prefix", "search_slash_prefix", "true"),
			Entry("decrypt_by_default", "decrypt_by_default", "true"),
			Entry("describe_page_size", "describe_page_size", "50"),
		)

		It("returns an error for unknown keys", func() {
			_, err := mgr.GetValue("nope")
			Expect(err).To(MatchError(ContainSubstring("unknown configuration key")))
		})
	})

	Describe("SetValue", func() {
		DescribeTable("accepts valid values and round-trips through GetValue",
			func(key, value, expectedReadback string) {
				Expect(mgr.SetValue(key, value)).To(Succeed())
				got, err := mgr.GetValue(key)
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(expectedReadback))
			},
			Entry("region", "region", "ap-southeast-1", "ap-southeast-1"),
			Entry("profile", "profile", "dev", "dev"),
			Entry("default_type", "default_type", "String", "String"),
			Entry("cache_ttl (duration parsed)", "cache_ttl", "2h", "2h0m0s"),
			Entry("clipboard_timeout", "clipboard_timeout", "30s", "30s"),
			Entry("parallel_fetches", "parallel_fetches", "20", "20"),
			Entry("search_slash_prefix=false", "search_slash_prefix", "false", "false"),
			Entry("decrypt_by_default=false", "decrypt_by_default", "false", "false"),
			Entry("describe_page_size", "describe_page_size", "25", "25"),
			Entry("describe_max_items unlimited", "describe_max_items", "0", "0"),
			Entry("describe_version_batch_size", "describe_version_batch_size", "5", "5"),
			Entry("sort alias modified", "default_sort", "modified", "modified"),
		)

		DescribeTable("rejects invalid values with a clear error",
			func(key, value, errSub string) {
				err := mgr.SetValue(key, value)
				Expect(err).To(MatchError(ContainSubstring(errSub)))
			},
			Entry("unknown parameter type", "default_type", "Number", "invalid type"),
			Entry("unknown sort option", "default_sort", "size", "invalid sort option"),
			Entry("malformed duration", "cache_ttl", "forever", "invalid duration format"),
			Entry("malformed bool", "search_slash_prefix", "maybe", "invalid boolean"),
			Entry("parallel_fetches > 50", "parallel_fetches", "999", "between 1 and 50"),
			Entry("parallel_fetches = 0", "parallel_fetches", "0", "between 1 and 50"),
			Entry("describe_page_size = 0", "describe_page_size", "0", "between 1 and 50"),
			Entry("describe_max_items negative", "describe_max_items", "-1", "must be >= 0"),
			Entry("describe_version_batch_size = 0", "describe_version_batch_size", "0", "must be >= 1"),
			Entry("unknown key", "blarg", "x", "unknown configuration key"),
		)
	})

	Describe("ListKeys", func() {
		It("returns keys that all resolve via GetValue", func() {
			for _, k := range mgr.ListKeys() {
				_, err := mgr.GetValue(k)
				Expect(err).NotTo(HaveOccurred(), "key %q must resolve", k)
			}
		})

		It("includes the headline settable keys", func() {
			keys := mgr.ListKeys()
			Expect(keys).To(ContainElements("region", "default_type", "describe_version_batch_size"))
		})
	})
})
