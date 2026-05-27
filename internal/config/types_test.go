package config

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DefaultConfig", func() {
	var cfg *Config

	BeforeEach(func() {
		cfg = DefaultConfig()
	})

	It("sets sensible AWS defaults", func() {
		Expect(cfg.Region).To(Equal("us-east-1"))
		Expect(cfg.Profile).To(BeEmpty(), "empty = SDK default, avoids forcing shared-config lookup")
	})

	It("does not pin a cache path (it's computed at runtime)", func() {
		Expect(cfg.CachePath).To(BeEmpty())
	})

	It("uses three-hour cache TTL and one-minute clipboard timeout", func() {
		Expect(cfg.CacheTTL).To(Equal(3 * time.Hour))
		Expect(cfg.ClipboardTimeout).To(Equal(60 * time.Second))
	})

	It("defaults parameter type to SecureString and sort to name", func() {
		Expect(cfg.DefaultType).To(Equal("SecureString"))
		Expect(cfg.DefaultSort).To(Equal("name"))
	})

	It("configures refresh parallelism and describe paging", func() {
		Expect(cfg.ParallelFetches).To(Equal(10))
		Expect(cfg.DescribePageSize).To(Equal(int32(50)))
		Expect(cfg.DescribeMaxItems).To(Equal(int32(0)), "0 = unlimited")
		Expect(cfg.DescribeVersionBatchSize).To(Equal(10))
	})

	It("enables search-slash prefix and decrypt-by-default", func() {
		Expect(cfg.SearchSlashPrefix).To(BeTrue())
		Expect(cfg.DecryptByDefault).To(BeTrue())
	})

	It("enables browse auto-refresh with a five-minute cooldown", func() {
		Expect(cfg.BrowseAutoRefresh).To(BeTrue())
		Expect(cfg.BrowseRefreshCooldown).To(Equal(5 * time.Minute))
	})
})

var _ = Describe("ValidTypes", func() {
	It("returns exactly the three SSM parameter types", func() {
		Expect(ValidTypes()).To(ConsistOf("String", "StringList", "SecureString"))
	})
})

var _ = Describe("ValidSortOptions", func() {
	It("returns full names and short aliases", func() {
		Expect(ValidSortOptions()).To(ConsistOf("name", "created", "modified", "n", "c", "m"))
	})
})

var _ = Describe("isValidType", func() {
	DescribeTable("classifies parameter type strings",
		func(input string, want bool) {
			Expect(isValidType(input)).To(Equal(want))
		},
		Entry("String is valid", "String", true),
		Entry("SecureString is valid", "SecureString", true),
		Entry("StringList is valid", "StringList", true),
		Entry("case-insensitive (lowercase)", "securestring", true),
		Entry("rejects unknown type", "Number", false),
		Entry("rejects empty string", "", false),
	)
})

var _ = Describe("isValidSort", func() {
	DescribeTable("classifies sort option strings",
		func(input string, want bool) {
			Expect(isValidSort(input)).To(Equal(want))
		},
		Entry("name", "name", true),
		Entry("short alias n", "n", true),
		Entry("created", "created", true),
		Entry("short alias c", "c", true),
		Entry("modified", "modified", true),
		Entry("short alias m", "m", true),
		Entry("uppercase NAME", "NAME", true),
		Entry("rejects version", "version", false),
		Entry("rejects empty", "", false),
	)
})
