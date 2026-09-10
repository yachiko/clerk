package aws

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Backend", func() {
	DescribeTable("parses supported selectors",
		func(input string, want Backend) {
			backend, err := ParseBackend(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(backend).To(Equal(want))
		},
		Entry("SSM", " SSM ", BackendSSM),
		Entry("Secrets Manager", "secretsmanager", BackendSecretsManager),
	)

	It("rejects the removed aggregate selector", func() {
		backend, err := ParseBackend("all")
		Expect(err).To(MatchError(`invalid backend "all" (valid: ssm, secretsmanager)`))
		Expect(backend).To(BeEmpty())
	})

	It("rejects an unsupported selector", func() {
		backend, err := ParseBackend("parameterstore")
		Expect(err).To(MatchError(`invalid backend "parameterstore" (valid: ssm, secretsmanager)`))
		Expect(backend).To(BeEmpty())
	})
})

var _ = Describe("backend-neutral resources", func() {
	It("keeps display name outside qualified identity", func() {
		identity := ResourceIdentity{
			Partition:   "aws-us-gov",
			AccountID:   "123456789012",
			Region:      "us-gov-west-1",
			Backend:     BackendSecretsManager,
			CanonicalID: "arn:aws-us-gov:secretsmanager:us-gov-west-1:123456789012:secret:app/db-AbCdEf",
		}
		metadata := ResourceMetadata{Identity: identity, DisplayName: "app/db"}

		Expect(metadata.Identity).To(Equal(identity))
		Expect(metadata.DisplayName).To(Equal("app/db"))
	})

	It("distinguishes empty text from empty binary values", func() {
		identity := ResourceIdentity{Backend: BackendSSM, CanonicalID: "/app/db"}
		text := NewTextValue(identity, "")
		binary := NewBinaryValue(identity, []byte{})

		Expect(text.Identity).To(Equal(identity))
		Expect(binary.Identity).To(Equal(identity))
		Expect(text.Kind).To(Equal(ValueText))
		Expect(binary.Kind).To(Equal(ValueBinary))
	})

	It("owns binary value bytes", func() {
		input := []byte{0, 1, 2}
		value := NewBinaryValue(ResourceIdentity{}, input)
		input[0] = 9

		Expect(value.Binary).To(Equal([]byte{0, 1, 2}))
		Expect(value.Text).To(BeEmpty())
	})
})
