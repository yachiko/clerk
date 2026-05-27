package util

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClipboardManager", func() {
	Describe("NewClipboardManager", func() {
		It("stores the clearTimeout it was given", func() {
			cm := NewClipboardManager(30 * time.Second)
			Expect(cm).NotTo(BeNil())
			Expect(cm.clearTimeout).To(Equal(30 * time.Second))
		})
	})

	Describe("IsClipboardSupported", func() {
		It("returns a bool without panicking (env-dependent value)", func() {
			Expect(func() { _ = IsClipboardSupported() }).NotTo(Panic())
		})
	})

	Describe("CopyWithMessage", func() {
		BeforeEach(func() {
			if !IsClipboardSupported() {
				Skip("clipboard not supported in this environment")
			}
		})

		It("returns a message that mentions the timeout when one is set", func() {
			cm := NewClipboardManager(30 * time.Second)
			msg, err := cm.CopyWithMessage("test-value")
			if err != nil {
				Skip("clipboard write failed (likely no display): " + err.Error())
			}
			Expect(strings.HasPrefix(msg, "Copied to clipboard")).To(BeTrue())
			Expect(msg).To(ContainSubstring("30s"))
		})

		It("returns a plain message when no timeout is set", func() {
			cm := NewClipboardManager(0)
			msg, err := cm.CopyWithMessage("test-value")
			if err != nil {
				Skip("clipboard write failed: " + err.Error())
			}
			Expect(msg).To(Equal("Copied to clipboard"))
		})
	})
})
