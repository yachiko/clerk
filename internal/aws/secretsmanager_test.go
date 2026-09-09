package aws

import (
	"context"
	"errors"
	"time"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeSecretsManager struct {
	listSecrets          func(*secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error)
	getSecretValue       func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIDs func(*secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (f *fakeSecretsManager) ListSecrets(_ context.Context, input *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	if f.listSecrets == nil {
		panic("unexpected ListSecrets")
	}
	return f.listSecrets(input)
}

func (f *fakeSecretsManager) GetSecretValue(_ context.Context, input *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if f.getSecretValue == nil {
		panic("unexpected GetSecretValue")
	}
	return f.getSecretValue(input)
}

func (f *fakeSecretsManager) ListSecretVersionIds(_ context.Context, input *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if f.listSecretVersionIDs == nil {
		panic("unexpected ListSecretVersionIds")
	}
	return f.listSecretVersionIDs(input)
}

func testSecretsManagerClient(resolved *ResolvedContext, api secretsManagerAPI) *SecretsManagerClient {
	client, err := newSecretsManagerClient(resolved, api)
	Expect(err).NotTo(HaveOccurred())
	return client
}

var _ = Describe("Secrets Manager read-only adapter", func() {
	var resolved *ResolvedContext

	BeforeEach(func() {
		resolved = &ResolvedContext{Partition: "aws", AccountID: "111122223333", Region: "us-east-1"}
	})

	It("fully paginates metadata inventory without reading values", func() {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		deleted := created.Add(24 * time.Hour)
		rotationEnabled := true
		calls := 0
		api := &fakeSecretsManager{listSecrets: func(input *secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) {
			calls++
			Expect(input.IncludePlannedDeletion).To(HaveValue(BeTrue()))
			switch calls {
			case 1:
				Expect(input.NextToken).To(BeNil())
				return &secretsmanager.ListSecretsOutput{
					NextToken: sdkaws.String("next"),
					SecretList: []smtypes.SecretListEntry{{
						Name: sdkaws.String("shared"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:shared-one"),
						Description: sdkaws.String("database"), KmsKeyId: sdkaws.String("alias/db"),
						Tags:        []smtypes.Tag{{Key: sdkaws.String("env"), Value: sdkaws.String("prod")}},
						CreatedDate: &created, DeletedDate: &deleted, RotationEnabled: &rotationEnabled,
						RotationLambdaARN:      sdkaws.String("arn:aws:lambda:us-east-1:111122223333:function:rotate"),
						RotationRules:          &smtypes.RotationRulesType{AutomaticallyAfterDays: sdkaws.Int64(30), ScheduleExpression: sdkaws.String("rate(30 days)")},
						SecretVersionsToStages: map[string][]string{"opaque-a": {"AWSCURRENT"}}, PrimaryRegion: sdkaws.String("us-west-2"),
					}},
				}, nil
			case 2:
				Expect(input.NextToken).To(HaveValue(Equal("next")))
				return &secretsmanager.ListSecretsOutput{SecretList: []smtypes.SecretListEntry{{
					Name: sdkaws.String("second"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:second-two"),
				}}}, nil
			default:
				panic("unexpected page")
			}
		}}

		metadata, err := testSecretsManagerClient(resolved, api).ListSecrets(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata).To(HaveLen(2))
		Expect(metadata[0].Identity).To(Equal(ResourceIdentity{Partition: "aws", AccountID: "111122223333", Region: "us-east-1", Backend: BackendSecretsManager, CanonicalID: "arn:aws:secretsmanager:us-east-1:111122223333:secret:shared-one"}))
		Expect(metadata[0].Description).To(Equal("database"))
		Expect(metadata[0].KMSKeyID).To(Equal("alias/db"))
		Expect(metadata[0].Tags).To(HaveKeyWithValue("env", "prod"))
		Expect(metadata[0].DeletedDate).To(HaveValue(Equal(deleted)))
		Expect(metadata[0].RotationEnabled).To(HaveValue(BeTrue()))
		Expect(metadata[0].RotationRules.AutomaticallyAfterDays).To(HaveValue(Equal(int64(30))))
		Expect(metadata[0].VersionsToStages).To(HaveKeyWithValue("opaque-a", []string{"AWSCURRENT"}))
		Expect(metadata[0].PrimaryRegion).To(Equal("us-west-2"))
		Expect(metadata[0].Replica).To(BeTrue())
		Expect(calls).To(Equal(2))
	})

	It("qualifies identical names by account and region", func() {
		makeAPI := func(secretARN string) *fakeSecretsManager {
			return &fakeSecretsManager{listSecrets: func(*secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) {
				return &secretsmanager.ListSecretsOutput{SecretList: []smtypes.SecretListEntry{{Name: sdkaws.String("shared"), ARN: sdkaws.String(secretARN)}}}, nil
			}}
		}
		first, err := testSecretsManagerClient(resolved, makeAPI("arn:aws:secretsmanager:us-east-1:111122223333:secret:shared-one")).ListSecrets(context.Background())
		Expect(err).NotTo(HaveOccurred())
		otherContext := &ResolvedContext{Partition: "aws-us-gov", AccountID: "999900001111", Region: "us-gov-west-1"}
		second, err := testSecretsManagerClient(otherContext, makeAPI("arn:aws-us-gov:secretsmanager:us-gov-west-1:999900001111:secret:shared-two")).ListSecrets(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(first[0].Name).To(Equal(second[0].Name))
		Expect(first[0].Identity).NotTo(Equal(second[0].Identity))
	})

	DescribeTable("preserves SecretString exactly",
		func(secret string) {
			api := &fakeSecretsManager{getSecretValue: func(input *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				Expect(input.VersionStage).To(HaveValue(Equal("AWSCURRENT")))
				return &secretsmanager.GetSecretValueOutput{Name: sdkaws.String("text"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:text-one"), SecretString: sdkaws.String(secret)}, nil
			}}
			detail, err := testSecretsManagerClient(resolved, api).GetSecretValue(context.Background(), "text", SecretValueSelector{})
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.Value.Kind).To(Equal(ValueText))
			Expect(detail.Value.Text).To(Equal(secret))
		},
		Entry("plain text", "  exact text\n"),
		Entry("Unicode", "密碼はそのまま"),
		Entry("JSON without normalization", "{\n  \"z\": 1, \"a\": [true]\n}"),
	)

	It("returns SecretBinary as owned binary data", func() {
		binary := []byte{0, 255, 1, 2}
		api := &fakeSecretsManager{getSecretValue: func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{Name: sdkaws.String("binary"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:binary-one"), SecretBinary: binary}, nil
		}}
		detail, err := testSecretsManagerClient(resolved, api).GetSecretValue(context.Background(), "binary", SecretValueSelector{})
		Expect(err).NotTo(HaveOccurred())
		binary[0] = 9
		Expect(detail.Value.Kind).To(Equal(ValueBinary))
		Expect(detail.Value.Binary).To(Equal([]byte{0, 255, 1, 2}))
		Expect(detail.Value.Text).To(BeEmpty())
	})

	It("passes explicit stage and opaque ID selectors", func() {
		var inputs []*secretsmanager.GetSecretValueInput
		api := &fakeSecretsManager{getSecretValue: func(input *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			copy := *input
			inputs = append(inputs, &copy)
			return &secretsmanager.GetSecretValueOutput{Name: sdkaws.String("selected"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:selected-one"), SecretString: sdkaws.String("value")}, nil
		}}
		client := testSecretsManagerClient(resolved, api)
		_, err := client.GetSecretValue(context.Background(), "selected", SecretValueSelector{VersionStage: "AWSPREVIOUS"})
		Expect(err).NotTo(HaveOccurred())
		_, err = client.GetSecretValue(context.Background(), "selected", SecretValueSelector{VersionID: "opaque/id:not-a-number"})
		Expect(err).NotTo(HaveOccurred())
		Expect(inputs[0].VersionStage).To(HaveValue(Equal("AWSPREVIOUS")))
		Expect(inputs[0].VersionId).To(BeNil())
		Expect(inputs[1].VersionId).To(HaveValue(Equal("opaque/id:not-a-number")))
		Expect(inputs[1].VersionStage).To(BeNil())

		_, err = client.GetSecretValue(context.Background(), "selected", SecretValueSelector{VersionStage: "current", VersionID: "id"})
		Expect(err).To(MatchError("version stage and version ID are mutually exclusive"))
		Expect(inputs).To(HaveLen(2))
	})

	It("wraps value access errors", func() {
		accessDenied := errors.New("AccessDeniedException: denied")
		api := &fakeSecretsManager{getSecretValue: func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, accessDenied
		}}
		_, err := testSecretsManagerClient(resolved, api).GetSecretValue(context.Background(), "restricted", SecretValueSelector{})
		Expect(err).To(MatchError(ContainSubstring("failed to get secret value")))
		Expect(errors.Is(err, accessDenied)).To(BeTrue())
	})

	It("fully paginates versions and retains opaque IDs, stages, and order", func() {
		calls := 0
		api := &fakeSecretsManager{listSecretVersionIDs: func(input *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			calls++
			Expect(input.IncludeDeprecated).To(HaveValue(BeTrue()))
			if calls == 1 {
				return &secretsmanager.ListSecretVersionIdsOutput{NextToken: sdkaws.String("more"), Versions: []smtypes.SecretVersionsListEntry{{VersionId: sdkaws.String("z-opaque"), VersionStages: []string{"AWSCURRENT"}}}}, nil
			}
			Expect(input.NextToken).To(HaveValue(Equal("more")))
			return &secretsmanager.ListSecretVersionIdsOutput{Versions: []smtypes.SecretVersionsListEntry{{VersionId: sdkaws.String("2-is-not-numeric"), VersionStages: []string{"custom", "AWSPREVIOUS"}, KmsKeyIds: []string{"key-a"}}}}, nil
		}}
		versions, err := testSecretsManagerClient(resolved, api).ListSecretVersionIds(context.Background(), "secret")
		Expect(err).NotTo(HaveOccurred())
		Expect(versions).To(HaveLen(2))
		Expect(versions[0].VersionID).To(Equal("z-opaque"))
		Expect(versions[1].VersionID).To(Equal("2-is-not-numeric"))
		Expect(versions[1].VersionStages).To(Equal([]string{"custom", "AWSPREVIOUS"}))
		Expect(versions[1].KMSKeyIDs).To(Equal([]string{"key-a"}))
	})

	It("rejects malformed empty responses without panicking", func() {
		api := &fakeSecretsManager{listSecrets: func(*secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) { return nil, nil }}
		_, err := testSecretsManagerClient(resolved, api).ListSecrets(context.Background())
		Expect(err).To(MatchError(ContainSubstring("empty response")))
	})
})
