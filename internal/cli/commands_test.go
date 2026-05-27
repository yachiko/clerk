package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseTags", func() {
	DescribeTable("parses tag strings",
		func(input string, want map[string]string, wantErrSub string) {
			got, err := parseTags(input)
			if wantErrSub != "" {
				Expect(err).To(MatchError(ContainSubstring(wantErrSub)))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want))
		},
		Entry("empty returns nil", "", map[string]string(nil), ""),
		Entry("single pair", "env=prod", map[string]string{"env": "prod"}, ""),
		Entry("multiple pairs", "env=prod,team=backend", map[string]string{"env": "prod", "team": "backend"}, ""),
		Entry("trims whitespace around key and value", "  env = prod ,  team = backend  ", map[string]string{"env": "prod", "team": "backend"}, ""),
		Entry("skips empty pairs between commas", "env=prod,,team=be", map[string]string{"env": "prod", "team": "be"}, ""),
		Entry("value may itself contain '='", "url=https://x?a=b", map[string]string{"url": "https://x?a=b"}, ""),
		Entry("rejects a token with no '='", "novalue", map[string]string(nil), "invalid tag format"),
		Entry("rejects an empty key", "=value", map[string]string(nil), "empty tag key"),
		Entry("allows empty value", "key=", map[string]string{"key": ""}, ""),
	)
})

var _ = Describe("normalizeSortOption", func() {
	DescribeTable("normalizes sort aliases",
		func(input, want string) {
			Expect(normalizeSortOption(input)).To(Equal(want))
		},
		Entry("name → name", "name", "name"),
		Entry("NAME (case-insensitive) → name", "NAME", "name"),
		Entry("n → name", "n", "name"),
		Entry("created → created", "created", "created"),
		Entry("c → created", "c", "created"),
		Entry("modified → modified", "modified", "modified"),
		Entry("m → modified", "m", "modified"),
		Entry("unknown defaults to name", "unknown", "name"),
		Entry("empty defaults to name", "", "name"),
	)
})

var _ = Describe("parseNameVersionLabel", func() {
	type result struct {
		name    string
		version int64
		label   string
	}

	DescribeTable("parses name@version / name:label syntax",
		func(input string, want result, wantErrSub string) {
			name, version, label, err := parseNameVersionLabel(input)
			if wantErrSub != "" {
				Expect(err).To(MatchError(ContainSubstring(wantErrSub)))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal(want.name))
			Expect(version).To(Equal(want.version))
			Expect(label).To(Equal(want.label))
		},
		Entry("plain name has no version or label", "/dev/db", result{name: "/dev/db"}, ""),
		Entry("@N → version", "/dev/db@3", result{name: "/dev/db", version: 3}, ""),
		Entry("@latest → version=0", "/dev/db@latest", result{name: "/dev/db"}, ""),
		Entry("@LATEST is case-insensitive", "/dev/db@LATEST", result{name: "/dev/db"}, ""),
		Entry(":label → label", "/dev/db:prod", result{name: "/dev/db", label: "prod"}, ""),
		Entry("non-numeric version is rejected", "/dev/db@abc", result{}, "invalid version"),
		Entry("version 0 is rejected (must be positive)", "/dev/db@0", result{}, "positive"),
		Entry("negative version is rejected", "/dev/db@-2", result{}, "positive"),
		Entry("trailing colon (empty label) is rejected", "/dev/db:", result{}, "label cannot be empty"),
	)
})

var _ = Describe("matchPath", func() {
	DescribeTable("matches names against path patterns",
		func(pattern, name string, want bool) {
			Expect(matchPath(pattern, name)).To(Equal(want))
		},
		Entry("empty pattern matches anything", "", "/anything", true),
		Entry("/ matches anything", "/", "/anything", true),
		Entry("/* matches anything", "/*", "/anything", true),
		Entry("/prod/* matches /prod children", "/prod/*", "/prod/x", true),
		Entry("/prod/* matches bare /prod", "/prod/*", "/prod", true),
		Entry("/prod/* rejects /dev children", "/prod/*", "/dev/x", false),
		Entry("*pass* contains-matches", "*pass*", "/db/password", true),
		Entry("*pass* rejects when absent", "*pass*", "/api/key", false),
		Entry("/prod* prefix-matches", "/prod*", "/prod/x", true),
		Entry("/prod* rejects /dev", "/prod*", "/dev/x", false),
		Entry("exact path matches itself", "/exact", "/exact", true),
		Entry("exact path rejects other paths", "/exact", "/different", false),
	)
})

var _ = Describe("extractBasePath", func() {
	DescribeTable("extracts the base prefix from a glob pattern",
		func(pattern, want string) {
			Expect(extractBasePath(pattern)).To(Equal(want))
		},
		Entry("/prod/* → /prod", "/prod/*", "/prod"),
		Entry("/prod/db/* → /prod/db", "/prod/db/*", "/prod/db"),
		Entry("/prod (no glob) → /prod", "/prod", "/prod"),
		Entry("exact path passes through", "/exact/path", "/exact/path"),
		Entry("'*' at index 0 short-circuits → pattern returned as-is", "*", "*"),
		Entry("/* trims to /", "/*", "/"),
	)
})

var _ = Describe("isValidParamType", func() {
	DescribeTable("classifies SSM parameter types",
		func(input string, want bool) {
			Expect(isValidParamType(input)).To(Equal(want))
		},
		Entry("String", "String", true),
		Entry("StringList", "StringList", true),
		Entry("SecureString", "SecureString", true),
		Entry("rejects lowercase 'string'", "string", false),
		Entry("rejects partial 'SECURE'", "SECURE", false),
		Entry("rejects unknown 'Number'", "Number", false),
		Entry("rejects empty", "", false),
		Entry("rejects 'bool'", "bool", false),
	)
})

var _ = Describe("formatTags", func() {
	It("renders a single tag as key=value", func() {
		Expect(formatTags(map[string]string{"env": "prod"})).To(Equal("env=prod"))
	})

	It("renders nil/empty map as empty string", func() {
		Expect(formatTags(nil)).To(BeEmpty())
	})

	It("renders multiple tags joined by ', ' (any order)", func() {
		got := formatTags(map[string]string{"env": "prod", "team": "be"})
		Expect(got).To(ContainSubstring("env=prod"))
		Expect(got).To(ContainSubstring("team=be"))
		Expect(got).To(ContainSubstring(", "))
	})
})
