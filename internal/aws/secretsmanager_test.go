package aws

import (
	"context"
	"time"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type secretsManagerStub struct {
	input *secretsmanager.GetSecretValueInput
	out   *secretsmanager.GetSecretValueOutput
}

func (s *secretsManagerStub) DescribeSecret(context.Context, *secretsmanager.DescribeSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	return &secretsmanager.DescribeSecretOutput{}, nil
}

func (s *secretsManagerStub) GetSecretValue(_ context.Context, input *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	s.input = input
	return s.out, nil
}

func (s *secretsManagerStub) ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return &secretsmanager.ListSecretsOutput{}, nil
}

var _ = Describe("Secrets Manager client", func() {
	It("base64 encodes SecretBinary and passes an opaque version ID", func() {
		stub := &secretsManagerStub{out: &secretsmanager.GetSecretValueOutput{
			ARN:          awsSDK.String("arn:aws:secretsmanager:us-east-1:123:secret:binary"),
			VersionId:    awsSDK.String("version-id"),
			CreatedDate:  awsSDK.Time(time.Now()),
			SecretBinary: []byte{0, 1, 2, 3},
		}}
		client := &Client{backend: "secretsmanager", secretsManager: stub}

		secret, err := client.GetSecret(context.Background(), "binary", "", "version-id")

		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Value).To(Equal("AAECAw=="))
		Expect(secret.Binary).To(BeTrue())
		Expect(secret.VersionID).To(Equal("version-id"))
		Expect(awsSDK.ToString(stub.input.VersionId)).To(Equal("version-id"))
		Expect(stub.input.VersionStage).To(BeNil())
	})

	It("passes a version stage without a version ID", func() {
		stub := &secretsManagerStub{out: &secretsmanager.GetSecretValueOutput{SecretString: awsSDK.String("value")}}
		client := &Client{backend: "secretsmanager", secretsManager: stub}

		_, err := client.GetSecret(context.Background(), "named", "AWSPREVIOUS", "")

		Expect(err).NotTo(HaveOccurred())
		Expect(awsSDK.ToString(stub.input.VersionStage)).To(Equal("AWSPREVIOUS"))
		Expect(stub.input.VersionId).To(BeNil())
	})
})
