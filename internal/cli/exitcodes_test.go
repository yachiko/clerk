package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exit codes", func() {
	It("uses 0 for success", func() {
		Expect(ExitSuccess).To(Equal(0))
	})
	It("uses 1 for general errors", func() {
		Expect(ExitGeneralError).To(Equal(1))
	})
	It("uses 2 for AWS-side errors", func() {
		Expect(ExitAWSError).To(Equal(2))
	})
	It("uses 3 for invalid input", func() {
		Expect(ExitInvalidInput).To(Equal(3))
	})
})
