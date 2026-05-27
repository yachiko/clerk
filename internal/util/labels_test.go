package util

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateLabel", func() {
	DescribeTable("validation rules",
		func(label, wantErrSub string) {
			err := ValidateLabel(label)
			if wantErrSub == "" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(ContainSubstring(wantErrSub)))
			}
		},
		Entry("rejects empty", "", "empty"),
		Entry("accepts simple word", "prod", ""),
		Entry("accepts dashes", "rollback-point", ""),
		Entry("accepts dots", "v1.2.3", ""),
		Entry("accepts underscores", "last_known_good", ""),
		Entry("rejects reserved prefix (lowercase)", "aws:foo", "reserved prefix"),
		Entry("rejects reserved prefix (mixed case)", "AWS:foo", "reserved prefix"),
		Entry("rejects spaces", "bad label", "invalid characters"),
		Entry("rejects slashes", "bad/label", "invalid characters"),
		Entry("rejects beyond max length", strings.Repeat("a", MaxLabelLength+1), "maximum length"),
		Entry("accepts exactly max length", strings.Repeat("a", MaxLabelLength), ""),
	)
})

var _ = Describe("ValidateLabels", func() {
	It("accepts a unique, valid set", func() {
		Expect(ValidateLabels([]string{"prod", "stable"})).To(Succeed())
	})

	It("rejects duplicates", func() {
		Expect(ValidateLabels([]string{"prod", "prod"})).To(MatchError(ContainSubstring("duplicate")))
	})

	It("rejects sets larger than the per-version cap", func() {
		tooMany := make([]string, MaxLabelsPerVersion+1)
		for i := range tooMany {
			tooMany[i] = "label" + string(rune('a'+i))
		}
		Expect(ValidateLabels(tooMany)).To(MatchError(ContainSubstring("more than")))
	})

	It("surfaces inner label errors", func() {
		Expect(ValidateLabels([]string{"ok", "aws:bad"})).To(MatchError(ContainSubstring("reserved prefix")))
	})
})

var _ = Describe("SuggestLabels", func() {
	It("returns a non-empty list including common environment names", func() {
		got := SuggestLabels()
		Expect(got).NotTo(BeEmpty())
		Expect(got).To(ContainElements("prod", "staging"))
	})

	It("only suggests labels that themselves validate", func() {
		for _, l := range SuggestLabels() {
			Expect(ValidateLabel(l)).To(Succeed(), "suggested label %q must validate", l)
		}
	})
})
