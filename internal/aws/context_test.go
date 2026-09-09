package aws

import (
	"context"
	"errors"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeSTS struct {
	output *sts.GetCallerIdentityOutput
	err    error
	calls  int
}

func (f *fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.calls++
	return f.output, f.err
}

var _ = Describe("resolved AWS context", func() {
	It("loads configuration and resolves caller scope once", func() {
		loadCalls := 0
		identity := &fakeSTS{output: &sts.GetCallerIdentityOutput{
			Account: sdkaws.String("123456789012"),
			Arn:     sdkaws.String("arn:aws-us-gov:sts::123456789012:assumed-role/test/session"),
		}}
		load := func(_ context.Context, options ...func(*config.LoadOptions) error) (sdkaws.Config, error) {
			loadCalls++
			loadOptions := config.LoadOptions{}
			for _, option := range options {
				Expect(option(&loadOptions)).To(Succeed())
			}
			Expect(loadOptions.Region).To(Equal("us-gov-west-1"))
			Expect(loadOptions.SharedConfigProfile).To(Equal("default"))
			return sdkaws.Config{Region: loadOptions.Region}, nil
		}

		resolved, err := resolveContext(context.Background(), ClientOptions{
			Region: "us-gov-west-1", Profile: "default", ProfileSet: true,
		}, load, func(sdkaws.Config) callerIdentityAPI { return identity })

		Expect(err).NotTo(HaveOccurred())
		Expect(loadCalls).To(Equal(1))
		Expect(identity.calls).To(Equal(1))
		Expect(resolved.Partition).To(Equal("aws-us-gov"))
		Expect(resolved.AccountID).To(Equal("123456789012"))
		Expect(resolved.Region).To(Equal("us-gov-west-1"))
		Expect(resolved.CallerARN).To(Equal("arn:aws-us-gov:sts::123456789012:assumed-role/test/session"))
	})

	It("does not call STS when the resolved SDK region is empty", func() {
		identity := &fakeSTS{}
		load := func(context.Context, ...func(*config.LoadOptions) error) (sdkaws.Config, error) {
			return sdkaws.Config{}, nil
		}

		_, err := resolveContext(context.Background(), ClientOptions{}, load, func(sdkaws.Config) callerIdentityAPI { return identity })

		Expect(err).To(MatchError(ContainSubstring("AWS region is not configured")))
		Expect(identity.calls).To(BeZero())
	})

	It("returns STS failures with the existing error context", func() {
		identity := &fakeSTS{err: errors.New("denied")}
		load := func(context.Context, ...func(*config.LoadOptions) error) (sdkaws.Config, error) {
			return sdkaws.Config{Region: "us-east-1"}, nil
		}

		_, err := resolveContext(context.Background(), ClientOptions{}, load, func(sdkaws.Config) callerIdentityAPI { return identity })

		Expect(err).To(MatchError("failed to get AWS account ID: denied"))
		Expect(identity.calls).To(Equal(1))
	})

	DescribeTable("derives standard AWS partitions",
		func(callerARN, want string) {
			partition, err := partitionFromCallerARN(callerARN)
			Expect(err).NotTo(HaveOccurred())
			Expect(partition).To(Equal(want))
		},
		Entry("commercial", "arn:aws:iam::123456789012:user/test", "aws"),
		Entry("GovCloud", "arn:aws-us-gov:sts::123456789012:assumed-role/test/session", "aws-us-gov"),
		Entry("China", "arn:aws-cn:iam::123456789012:user/test", "aws-cn"),
	)

	It("rejects a malformed caller ARN instead of guessing a partition", func() {
		partition, err := partitionFromCallerARN("not-an-arn")
		Expect(err).To(MatchError(`failed to derive AWS partition from caller ARN "not-an-arn"`))
		Expect(partition).To(BeEmpty())
	})
})
