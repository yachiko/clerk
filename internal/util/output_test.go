package util

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MaskValue", func() {
	DescribeTable("uses a fixed mask without fragments or length",
		func(input, want string) {
			Expect(MaskValue(input)).To(Equal(want))
		},
		Entry("empty", "", "********"),
		Entry("short", "ab", "********"),
		Entry("long", "verylongsecretvalue", "********"),
	)
})

var _ = Describe("MaskValueFull", func() {
	It("also returns a fixed mask", func() {
		Expect(MaskValueFull("")).To(Equal("********"))
		Expect(MaskValueFull("hello")).To(Equal("********"))
	})
})

var _ = Describe("SanitizeTerminal", func() {
	It("escapes terminal controls while retaining useful text", func() {
		input := "safe\n\t\x1b[2J\r\x1b]52;c;data\a\u009bKunicode ✓"
		got := SanitizeTerminal(input)
		Expect(got).To(ContainSubstring("safe\n\t"))
		Expect(got).To(ContainSubstring("\\x1b[2J\\x0d\\x1b]52;c;data\\x07\\u009bK"))
		Expect(got).To(ContainSubstring("unicode ✓"))
		Expect(got).NotTo(ContainSubstring("\x1b"))
	})
})

var _ = Describe("Formatter", func() {
	Describe("NewFormatter", func() {
		It("falls back to plain for unrecognized format strings", func() {
			f := NewFormatter("unrecognized", &bytes.Buffer{})
			Expect(f.format).To(Equal(OutputPlain))
		})
		It("accepts JSON case-insensitively", func() {
			f := NewFormatter("JSON", &bytes.Buffer{})
			Expect(f.format).To(Equal(OutputJSON))
		})
		It("accepts plain explicitly", func() {
			f := NewFormatter("plain", &bytes.Buffer{})
			Expect(f.format).To(Equal(OutputPlain))
		})
	})

	Describe("Print", func() {
		It("emits valid indented JSON in JSON mode", func() {
			var buf bytes.Buffer
			f := NewFormatter("json", &buf)
			Expect(f.Print(map[string]any{"name": "test", "value": 123})).To(Succeed())

			var decoded map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &decoded)).To(Succeed())
			Expect(decoded).To(HaveKeyWithValue("name", "test"))
			Expect(decoded).To(HaveKeyWithValue("value", BeNumerically("==", 123)))
		})

		It("emits a fmt.Println-style line in plain mode", func() {
			var buf bytes.Buffer
			f := NewFormatter("plain", &buf)
			Expect(f.Print("hello")).To(Succeed())
			Expect(buf.String()).To(Equal("hello\n"))
		})
	})

	Describe("styled helpers", func() {
		It("are silent in JSON mode so they don't corrupt the stream", func() {
			var buf bytes.Buffer
			f := NewFormatter("json", &buf)
			f.PrintSuccess("ok %s", "yes")
			f.PrintError("nope %d", 1)
			f.PrintWarning("watch")
			f.PrintInfo("fyi")
			Expect(buf.Bytes()).To(BeEmpty())
		})
	})
})
