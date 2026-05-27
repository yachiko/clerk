package aws

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IsParameterNotFoundError", func() {
	DescribeTable("classifies errors",
		func(err error, want bool) {
			Expect(IsParameterNotFoundError(err)).To(Equal(want))
		},
		Entry("nil → false", nil, false),
		Entry("generic error → false", errors.New("boom"), false),
		Entry("typed ParameterNotFound → true", &types.ParameterNotFound{}, true),
		Entry("wrapped ParameterNotFound → true", fmt.Errorf("get failed: %w", &types.ParameterNotFound{}), true),
	)
})

var _ = Describe("IsParameterAlreadyExistsError", func() {
	DescribeTable("classifies errors",
		func(err error, want bool) {
			Expect(IsParameterAlreadyExistsError(err)).To(Equal(want))
		},
		Entry("nil → false", nil, false),
		Entry("generic error → false", errors.New("boom"), false),
		Entry("typed ParameterAlreadyExists → true", &types.ParameterAlreadyExists{}, true),
		Entry("wrapped ParameterAlreadyExists → true", fmt.Errorf("put failed: %w", &types.ParameterAlreadyExists{}), true),
	)
})

var _ = Describe("IsAccessDeniedError", func() {
	DescribeTable("classifies errors by message substring",
		func(err error, want bool) {
			Expect(IsAccessDeniedError(err)).To(Equal(want))
		},
		Entry("nil → false", nil, false),
		Entry("unrelated error → false", errors.New("boom"), false),
		Entry("AccessDeniedException → true", errors.New("AccessDeniedException: ..."), true),
		Entry("UnauthorizedAccess → true", errors.New("UnauthorizedAccess: ..."), true),
		Entry("wrapped AccessDenied → true", fmt.Errorf("rpc: %w", errors.New("AccessDenied")), true),
	)
})
