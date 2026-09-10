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
	describeSecret       func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error)
	getSecretValue       func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIDs func(*secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error)
	createSecret         func(*secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error)
	putSecretValue       func(*secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error)
	tagResource          func(*secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error)
	untagResource        func(*secretsmanager.UntagResourceInput) (*secretsmanager.UntagResourceOutput, error)
	deleteSecret         func(*secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error)
	restoreSecret        func(*secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error)
}

func (f *fakeSecretsManager) ListSecrets(_ context.Context, input *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	if f.listSecrets == nil {
		panic("unexpected ListSecrets")
	}
	return f.listSecrets(input)
}

func (f *fakeSecretsManager) DescribeSecret(_ context.Context, input *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	if f.describeSecret == nil {
		panic("unexpected DescribeSecret")
	}
	return f.describeSecret(input)
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

func (f *fakeSecretsManager) CreateSecret(_ context.Context, input *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	if f.createSecret == nil {
		panic("unexpected CreateSecret")
	}
	return f.createSecret(input)
}

func (f *fakeSecretsManager) PutSecretValue(_ context.Context, input *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if f.putSecretValue == nil {
		panic("unexpected PutSecretValue")
	}
	return f.putSecretValue(input)
}

func (f *fakeSecretsManager) TagResource(_ context.Context, input *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
	if f.tagResource == nil {
		panic("unexpected TagResource")
	}
	return f.tagResource(input)
}

func (f *fakeSecretsManager) UntagResource(_ context.Context, input *secretsmanager.UntagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
	if f.untagResource == nil {
		panic("unexpected UntagResource")
	}
	return f.untagResource(input)
}

func (f *fakeSecretsManager) DeleteSecret(_ context.Context, input *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if f.deleteSecret == nil {
		panic("unexpected DeleteSecret")
	}
	return f.deleteSecret(input)
}

func (f *fakeSecretsManager) RestoreSecret(_ context.Context, input *secretsmanager.RestoreSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error) {
	if f.restoreSecret == nil {
		panic("unexpected RestoreSecret")
	}
	return f.restoreSecret(input)
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

	It("describes complete metadata without reading a value", func() {
		created := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
		api := &fakeSecretsManager{describeSecret: func(input *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			Expect(input.SecretId).To(HaveValue(Equal("database")))
			return &secretsmanager.DescribeSecretOutput{
				Name: sdkaws.String("database"), ARN: sdkaws.String("arn:aws:secretsmanager:us-east-1:111122223333:secret:database-one"),
				Description: sdkaws.String("primary"), KmsKeyId: sdkaws.String("alias/database"), CreatedDate: &created,
				Tags: []smtypes.Tag{{Key: sdkaws.String("env"), Value: sdkaws.String("prod")}}, VersionIdsToStages: map[string][]string{"opaque/id": {"AWSCURRENT"}},
			}, nil
		}}
		metadata, err := testSecretsManagerClient(resolved, api).DescribeSecret(context.Background(), "database")
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata.Name).To(Equal("database"))
		Expect(metadata.Description).To(Equal("primary"))
		Expect(metadata.KMSKeyID).To(Equal("alias/database"))
		Expect(metadata.Tags).To(Equal(map[string]string{"env": "prod"}))
		Expect(metadata.VersionsToStages).To(Equal(map[string][]string{"opaque/id": {"AWSCURRENT"}}))
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

	Describe("mutations", func() {
		const secretARN = "arn:aws:secretsmanager:us-east-1:111122223333:secret:created-one"

		It("creates text secrets with metadata and a secure request token", func() {
			api := &fakeSecretsManager{createSecret: func(input *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
				Expect(input.Name).To(HaveValue(Equal("created")))
				Expect(input.SecretString).To(HaveValue(Equal("")))
				Expect(input.SecretBinary).To(BeNil())
				Expect(input.Description).To(HaveValue(Equal("description")))
				Expect(input.KmsKeyId).To(HaveValue(Equal("alias/key")))
				Expect(input.Tags).To(Equal([]smtypes.Tag{{Key: sdkaws.String("a"), Value: sdkaws.String("first")}, {Key: sdkaws.String("z"), Value: sdkaws.String("last")}}))
				Expect(input.ClientRequestToken).To(HaveValue(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)))
				return &secretsmanager.CreateSecretOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN), VersionId: sdkaws.String("opaque-version")}, nil
			}}
			result, err := testSecretsManagerClient(resolved, api).CreateSecret(context.Background(), CreateSecretRequest{
				Name: "created", Value: SecretValueInput{Kind: ValueText}, Description: "description", KMSKeyID: "alias/key", Tags: map[string]string{"z": "last", "a": "first"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.VersionID).To(Equal("opaque-version"))
			Expect(result.Identity.CanonicalID).To(Equal(secretARN))
		})

		It("copies binary bytes before creating a secret", func() {
			value := []byte{0, 1, 255}
			api := &fakeSecretsManager{createSecret: func(input *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
				Expect(input.SecretString).To(BeNil())
				Expect(input.SecretBinary).To(Equal([]byte{0, 1, 255}))
				input.SecretBinary[0] = 9
				return &secretsmanager.CreateSecretOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN), VersionId: sdkaws.String("v1")}, nil
			}}
			_, err := testSecretsManagerClient(resolved, api).CreateSecret(context.Background(), CreateSecretRequest{Name: "created", Value: SecretValueInput{Kind: ValueBinary, Binary: value}})
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal([]byte{0, 1, 255}))
		})

		It("puts binary versions and preserves opaque result IDs", func() {
			value := []byte{3, 2, 1}
			api := &fakeSecretsManager{putSecretValue: func(input *secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error) {
				Expect(input.SecretId).To(HaveValue(Equal("created")))
				Expect(input.SecretBinary).To(Equal(value))
				Expect(input.VersionStages).To(Equal([]string{"CUSTOM"}))
				Expect(input.ClientRequestToken).NotTo(BeNil())
				input.SecretBinary[0] = 8
				return &secretsmanager.PutSecretValueOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN), VersionId: sdkaws.String("not/a:number"), VersionStages: []string{"CUSTOM"}}, nil
			}}
			result, err := testSecretsManagerClient(resolved, api).PutSecretValue(context.Background(), PutSecretValueRequest{SecretID: "created", Value: SecretValueInput{Kind: ValueBinary, Binary: value}, VersionStages: []string{"CUSTOM"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal([]byte{3, 2, 1}))
			Expect(result.VersionID).To(Equal("not/a:number"))
			Expect(result.VersionStages).To(Equal([]string{"CUSTOM"}))
		})

		It("tags and untags using owned deterministic inputs", func() {
			calls := 0
			api := &fakeSecretsManager{
				tagResource: func(input *secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error) {
					calls++
					Expect(input.Tags).To(Equal([]smtypes.Tag{{Key: sdkaws.String("a"), Value: sdkaws.String("1")}, {Key: sdkaws.String("b"), Value: sdkaws.String("2")}}))
					return &secretsmanager.TagResourceOutput{}, nil
				},
				untagResource: func(input *secretsmanager.UntagResourceInput) (*secretsmanager.UntagResourceOutput, error) {
					calls++
					Expect(input.TagKeys).To(Equal([]string{"a", "b"}))
					input.TagKeys[0] = "changed"
					return &secretsmanager.UntagResourceOutput{}, nil
				},
			}
			keys := []string{"a", "b"}
			client := testSecretsManagerClient(resolved, api)
			Expect(client.TagResource(context.Background(), TagSecretRequest{SecretID: "created", Tags: map[string]string{"b": "2", "a": "1"}})).To(Succeed())
			Expect(client.UntagResource(context.Background(), UntagSecretRequest{SecretID: "created", TagKeys: keys})).To(Succeed())
			Expect(keys).To(Equal([]string{"a", "b"}))
			Expect(calls).To(Equal(2))
		})

		It("supports scheduled and explicitly permanent deletion", func() {
			deletionDate := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
			calls := 0
			api := &fakeSecretsManager{deleteSecret: func(input *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
				calls++
				if calls == 1 {
					Expect(input.RecoveryWindowInDays).To(HaveValue(Equal(int64(14))))
					Expect(input.ForceDeleteWithoutRecovery).To(BeNil())
					return &secretsmanager.DeleteSecretOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN), DeletionDate: &deletionDate}, nil
				}
				Expect(input.RecoveryWindowInDays).To(BeNil())
				Expect(input.ForceDeleteWithoutRecovery).To(HaveValue(BeTrue()))
				return &secretsmanager.DeleteSecretOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN)}, nil
			}}
			client := testSecretsManagerClient(resolved, api)
			result, err := client.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "created", RecoveryWindowDays: 14})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DeletionDate).To(HaveValue(Equal(deletionDate)))
			result, err = client.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "created", Permanent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.DeletionDate).To(BeNil())
		})

		It("restores a scheduled secret", func() {
			api := &fakeSecretsManager{restoreSecret: func(input *secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error) {
				Expect(input.SecretId).To(HaveValue(Equal("created")))
				return &secretsmanager.RestoreSecretOutput{Name: sdkaws.String("created"), ARN: sdkaws.String(secretARN)}, nil
			}}
			result, err := testSecretsManagerClient(resolved, api).RestoreSecret(context.Background(), RestoreSecretRequest{SecretID: "created"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Identity.CanonicalID).To(Equal(secretARN))
		})

		DescribeTable("validates mutation requests before API calls",
			func(run func(*SecretsManagerClient) error, expected string) {
				err := run(testSecretsManagerClient(resolved, &fakeSecretsManager{}))
				Expect(err).To(MatchError(expected))
			},
			Entry("create name", func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "secret name is required"),
			Entry("create value kind", func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Name: "name"})
				return err
			}, "secret value kind must be text or binary"),
			Entry("create tag key", func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Name: "name", Value: SecretValueInput{Kind: ValueText}, Tags: map[string]string{" ": "value"}})
				return err
			}, "tag keys must not be empty"),
			Entry("text with binary", func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Name: "name", Value: SecretValueInput{Kind: ValueText, Binary: []byte{1}}})
				return err
			}, "text secret value must not include binary data"),
			Entry("put ID", func(c *SecretsManagerClient) error {
				_, err := c.PutSecretValue(context.Background(), PutSecretValueRequest{Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "secret ID is required"),
			Entry("put stage", func(c *SecretsManagerClient) error {
				_, err := c.PutSecretValue(context.Background(), PutSecretValueRequest{SecretID: "id", Value: SecretValueInput{Kind: ValueText}, VersionStages: []string{" "}})
				return err
			}, "version stages must not contain empty values"),
			Entry("tag list", func(c *SecretsManagerClient) error {
				return c.TagResource(context.Background(), TagSecretRequest{SecretID: "id"})
			}, "at least one tag is required"),
			Entry("tag key", func(c *SecretsManagerClient) error {
				return c.TagResource(context.Background(), TagSecretRequest{SecretID: "id", Tags: map[string]string{"": "value"}})
			}, "tag keys must not be empty"),
			Entry("untag list", func(c *SecretsManagerClient) error {
				return c.UntagResource(context.Background(), UntagSecretRequest{SecretID: "id"})
			}, "at least one tag key is required"),
			Entry("untag key", func(c *SecretsManagerClient) error {
				return c.UntagResource(context.Background(), UntagSecretRequest{SecretID: "id", TagKeys: []string{""}})
			}, "tag keys must not contain empty values"),
			Entry("short recovery", func(c *SecretsManagerClient) error {
				_, err := c.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "id", RecoveryWindowDays: 6})
				return err
			}, "recovery window must be between 7 and 30 days"),
			Entry("long recovery", func(c *SecretsManagerClient) error {
				_, err := c.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "id", RecoveryWindowDays: 31})
				return err
			}, "recovery window must be between 7 and 30 days"),
			Entry("permanent recovery", func(c *SecretsManagerClient) error {
				_, err := c.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "id", RecoveryWindowDays: 7, Permanent: true})
				return err
			}, "recovery window cannot be set for permanent deletion"),
			Entry("restore ID", func(c *SecretsManagerClient) error {
				_, err := c.RestoreSecret(context.Background(), RestoreSecretRequest{})
				return err
			}, "secret ID is required"),
		)

		DescribeTable("rejects nil and malformed mutation responses",
			func(api *fakeSecretsManager, run func(*SecretsManagerClient) error, expected string) {
				err := run(testSecretsManagerClient(resolved, api))
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("describe nil", &fakeSecretsManager{describeSecret: func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
				return nil, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.DescribeSecret(context.Background(), "id")
				return err
			}, "empty response"),
			Entry("describe malformed", &fakeSecretsManager{describeSecret: func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
				return &secretsmanager.DescribeSecretOutput{}, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.DescribeSecret(context.Background(), "id")
				return err
			}, "without a name or ARN"),
			Entry("create nil", &fakeSecretsManager{createSecret: func(*secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) { return nil, nil }}, func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Name: "id", Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "empty response"),
			Entry("create malformed", &fakeSecretsManager{createSecret: func(*secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
				return &secretsmanager.CreateSecretOutput{}, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.CreateSecret(context.Background(), CreateSecretRequest{Name: "id", Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "without a name, ARN, or version ID"),
			Entry("put nil", &fakeSecretsManager{putSecretValue: func(*secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error) {
				return nil, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.PutSecretValue(context.Background(), PutSecretValueRequest{SecretID: "id", Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "empty response"),
			Entry("put malformed", &fakeSecretsManager{putSecretValue: func(*secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error) {
				return &secretsmanager.PutSecretValueOutput{}, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.PutSecretValue(context.Background(), PutSecretValueRequest{SecretID: "id", Value: SecretValueInput{Kind: ValueText}})
				return err
			}, "without a name, ARN, or version ID"),
			Entry("tag nil", &fakeSecretsManager{tagResource: func(*secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error) {
				return nil, nil
			}}, func(c *SecretsManagerClient) error {
				return c.TagResource(context.Background(), TagSecretRequest{SecretID: "id", Tags: map[string]string{"key": "value"}})
			}, "empty response"),
			Entry("untag nil", &fakeSecretsManager{untagResource: func(*secretsmanager.UntagResourceInput) (*secretsmanager.UntagResourceOutput, error) {
				return nil, nil
			}}, func(c *SecretsManagerClient) error {
				return c.UntagResource(context.Background(), UntagSecretRequest{SecretID: "id", TagKeys: []string{"key"}})
			}, "empty response"),
			Entry("delete nil", &fakeSecretsManager{deleteSecret: func(*secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) { return nil, nil }}, func(c *SecretsManagerClient) error {
				_, err := c.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "id", Permanent: true})
				return err
			}, "empty response"),
			Entry("scheduled deletion date", &fakeSecretsManager{deleteSecret: func(*secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
				return &secretsmanager.DeleteSecretOutput{Name: sdkaws.String("id"), ARN: sdkaws.String(secretARN)}, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.DeleteSecret(context.Background(), DeleteSecretRequest{SecretID: "id", RecoveryWindowDays: 7})
				return err
			}, "without a deletion date"),
			Entry("restore malformed", &fakeSecretsManager{restoreSecret: func(*secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error) {
				return &secretsmanager.RestoreSecretOutput{}, nil
			}}, func(c *SecretsManagerClient) error {
				_, err := c.RestoreSecret(context.Background(), RestoreSecretRequest{SecretID: "id"})
				return err
			}, "without a name or ARN"),
		)

		It("wraps mutation API errors", func() {
			denied := errors.New("denied")
			api := &fakeSecretsManager{tagResource: func(*secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error) { return nil, denied }}
			err := testSecretsManagerClient(resolved, api).TagResource(context.Background(), TagSecretRequest{SecretID: "id", Tags: map[string]string{"a": "b"}})
			Expect(err).To(MatchError(ContainSubstring("failed to tag secret")))
			Expect(errors.Is(err, denied)).To(BeTrue())
		})
	})
})
