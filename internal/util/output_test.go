package util

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MaskValue", func() {
	DescribeTable("masks per length",
		func(input, want string) {
			Expect(MaskValue(input)).To(Equal(want))
		},
		Entry("empty stays empty", "", ""),
		Entry("two chars are fully masked", "ab", "**"),
		Entry("8 chars are fully masked (boundary)", "12345678", "********"),
		Entry("9+ chars keep first/last two", "123456789", "12*****89"),
		Entry("long values preserve first/last two", "verylongsecretvalue", "ve***************ue"),
	)
})

var _ = Describe("MaskValueFull", func() {
	It("returns asterisks of the same length", func() {
		Expect(MaskValueFull("")).To(BeEmpty())
		Expect(MaskValueFull("hello")).To(Equal("*****"))
		Expect(MaskValueFull("12345678")).To(Equal("********"))
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
