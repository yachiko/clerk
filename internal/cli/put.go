package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/util"
)

var (
	putTags        string
	putType        string
	putKMSKeyID    string
	putFile        string
	putStdin       bool
	putDescription string
)

// InitPutCommand initializes the PUT command
func InitPutCommand() *cobra.Command {
	putCmd := &cobra.Command{
		Use:   "put <name> [value]",
		Short: "Create or update a secret in the selected backend",
		Long: `Create a new secret or update an existing value in the selected backend.

Parameter Store is used by default. For Secrets Manager, a missing secret is
created and an existing secret receives a new AWSCURRENT version.

SSM positional values remain literal. With Secrets Manager, file://PATH reads
an exact SecretString and fileb://PATH reads raw SecretBinary bytes. --file and
--stdin read exact SecretString contents.

Examples:
  # Create a secret with a string value
  clerk put "/dev/db_password" "mypassword123" --backend ssm

  # Create a secret from a file
  clerk put "/dev/api_key" --file ./secrets/api_key.txt --backend ssm

  # Create with tags
  clerk put "/dev/db_password" "mypassword123" --tags "env=dev,team=backend" --backend ssm

  # Create as StringList
  clerk put "/dev/allowed_ips" "10.0.0.1,10.0.0.2" --type StringList --backend ssm

  # Create with custom KMS key
  clerk put "/prod/secret" "value" --kms-key-id "alias/my-key" --backend ssm

  # Create a binary Secrets Manager secret with metadata
  clerk put "prod/certificate" fileb://certificate.p12 --backend secretsmanager --description "TLS certificate" --kms-key-id "alias/secrets" --tags "env=prod"`,
		Args:    validatePutArgs,
		PreRunE: validatePutFlags,
		RunE:    runPut,
	}

	putCmd.Flags().StringVar(&putTags, "tags", "", "Tags in format key1=value1,key2=value2")
	putCmd.Flags().StringVar(&putType, "type", "", "Parameter type: String, StringList, SecureString")
	putCmd.Flags().StringVar(&putKMSKeyID, "kms-key-id", "", "KMS key ID for SecureString encryption")
	putCmd.Flags().StringVar(&putFile, "file", "", "Read an exact value from this file")
	putCmd.Flags().BoolVar(&putStdin, "stdin", false, "Read an exact value from standard input")
	putCmd.Flags().StringVar(&putDescription, "description", "", "Description for a new Secrets Manager secret")

	return putCmd
}

func runPut(cmd *cobra.Command, args []string) error {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	valueOrFile := ""
	if len(args) == 2 {
		valueOrFile = args[1]
	}

	if backend == aws.BackendSSM {
		if err := validateParameterIdentifier(name, false); err != nil {
			return err
		}
	} else if strings.TrimSpace(name) == "" {
		return fmt.Errorf("secret name cannot be empty")
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	if backend == aws.BackendSecretsManager {
		value, valueErr := resolveSecretValue(valueOrFile, putFile, putStdin, os.Stdin)
		if valueErr != nil {
			return fmt.Errorf("failed to resolve value: %w", valueErr)
		}
		tags, tagErr := parseTags(putTags)
		if tagErr != nil {
			return fmt.Errorf("failed to parse tags: %w", tagErr)
		}
		return runSecretsManagerPut(cmd, cfg, name, value, tags)
	}
	return runSSMPut(cmd, cfg, name, valueOrFile)
}

func runSSMPut(cmd *cobra.Command, cfg *config.Config, name, valueOrFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	value, err := resolveValue(valueOrFile, putFile, putStdin, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to resolve value: %w", err)
	}

	tags, err := parseTags(putTags)
	if err != nil {
		return fmt.Errorf("failed to parse tags: %w", err)
	}

	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Check if parameter exists (to determine if we're creating or updating)
	existing, err := client.GetParameter(ctx, name, false)
	isUpdate := err == nil
	if err != nil && !aws.IsParameterNotFoundError(err) {
		return fmt.Errorf("failed to check existing parameter: %w", err)
	}
	paramType := putType
	if paramType == "" && existing != nil {
		paramType = existing.Type
	}
	if paramType == "" {
		paramType = cfg.DefaultType
	}
	if !isValidParamType(paramType) {
		return fmt.Errorf("invalid parameter type: %s (valid: String, StringList, SecureString)", paramType)
	}
	kmsKeyID := putKMSKeyID
	if kmsKeyID == "" && existing != nil && paramType == "SecureString" {
		metadata, metadataErr := client.GetParameterMetadata(ctx, existing.Name)
		if metadataErr != nil {
			return fmt.Errorf("failed to read existing encryption metadata: %w", metadataErr)
		}
		kmsKeyID = metadata.KMSKeyID
	}

	// Prepare input
	input := &aws.PutParameterInput{
		Name:      name,
		Value:     value,
		Type:      paramType,
		Overwrite: isUpdate,
		KMSKeyID:  kmsKeyID,
		Tags:      tags,
	}

	// Put parameter
	output, err := client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put parameter: %w", err)
	}

	reconcileParameterTags(ctx, cfg, client, name)

	// Output result
	if globalOpts.Output == "json" {
		formatter := util.NewFormatter("json", os.Stdout)
		result := map[string]any{
			"name":    name,
			"version": output.Version,
			"type":    paramType,
			"action":  "updated",
		}
		if !isUpdate {
			result["action"] = "created"
		}
		if len(tags) > 0 {
			result["tags"] = tags
		}
		return formatter.Print(result)
	}

	// Plain output
	action := "Created"
	if isUpdate {
		action = "Updated"
	}
	color.Green("%s parameter: %s (version %d)", action, name, output.Version)

	if len(tags) > 0 {
		color.Cyan("Tags: %s", formatTags(tags))
	}

	return nil
}

func runSecretsManagerPut(cmd *cobra.Command, cfg *config.Config, name string, value aws.SecretValueInput, tags map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}
	resolved, err := aws.ResolveContext(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}
	client, err := aws.NewSecretsManagerClient(resolved)
	if err != nil {
		return fmt.Errorf("failed to create Secrets Manager client: %w", err)
	}

	_, err = client.DescribeSecret(ctx, name)
	isCreate := false
	if err != nil {
		var notFound *smtypes.ResourceNotFoundException
		if !errors.As(err, &notFound) {
			return fmt.Errorf("failed to check existing secret: %w", err)
		}
		isCreate = true
	}

	var versionID string
	if isCreate {
		result, createErr := client.CreateSecret(ctx, aws.CreateSecretRequest{Name: name, Value: value, Description: putDescription, KMSKeyID: putKMSKeyID, Tags: tags})
		if createErr != nil {
			return createErr
		}
		name, versionID = result.Name, result.VersionID
	} else {
		if putDescription != "" || putKMSKeyID != "" || len(tags) > 0 {
			return fmt.Errorf("--description, --kms-key-id, and --tags are supported only when creating a Secrets Manager secret")
		}
		result, putErr := client.PutSecretValue(ctx, aws.PutSecretValueRequest{SecretID: name, Value: value, VersionStages: []string{"AWSCURRENT"}})
		if putErr != nil {
			return putErr
		}
		name, versionID = result.Name, result.VersionID
	}
	reconcileSecretCache(ctx, cfg, resolved, client, name)
	return outputSecretPutResult(os.Stdout, name, versionID, isCreate, tags)
}

func outputSecretPutResult(out io.Writer, name, versionID string, created bool, tags map[string]string) error {
	action := "updated"
	if created {
		action = "created"
	}
	if globalOpts.Output == "json" {
		formatter := util.NewFormatter("json", out)
		result := map[string]any{"name": name, "version_id": versionID, "action": action, "backend": aws.BackendSecretsManager}
		if created && len(tags) > 0 {
			result["tags"] = tags
		}
		return formatter.Print(result)
	}
	_, err := fmt.Fprintf(out, "%s secret: %s (version %s)\n", strings.ToUpper(action[:1])+action[1:], name, versionID)
	return err
}

func validatePutArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("accepts a name and one literal value, or a name with --file/--stdin")
	}
	if putFile != "" && putStdin {
		return fmt.Errorf("--file and --stdin cannot be used together")
	}
	if len(args) == 2 && (putFile != "" || putStdin) {
		return fmt.Errorf("literal value cannot be combined with --file or --stdin")
	}
	if len(args) == 1 && putFile == "" && !putStdin {
		return fmt.Errorf("a literal value, --file, or --stdin is required")
	}
	return nil
}

func validatePutFlags(cmd *cobra.Command, _ []string) error {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	if backend == aws.BackendSecretsManager && putType != "" {
		return fmt.Errorf("--type is supported only with --backend ssm")
	}
	if backend == aws.BackendSSM && putDescription != "" {
		return fmt.Errorf("--description is supported only with --backend secretsmanager")
	}
	return nil
}

// resolveValue resolves the value from a file path or returns it directly
func resolveValue(literal, file string, stdin bool, in *os.File) (string, error) {
	if file != "" && stdin {
		return "", fmt.Errorf("--file and --stdin cannot be used together")
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --file %q: %w", file, err)
		}
		return string(data), nil
	}
	if stdin {
		data, err := io.ReadAll(in)
		if err != nil {
			return "", fmt.Errorf("read --stdin: %w", err)
		}
		return string(data), nil
	}
	return literal, nil
}

func resolveSecretValue(positional, file string, stdin bool, in io.Reader) (aws.SecretValueInput, error) {
	if file != "" && stdin {
		return aws.SecretValueInput{}, fmt.Errorf("--file and --stdin cannot be used together")
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return aws.SecretValueInput{}, fmt.Errorf("read --file %q: %w", file, err)
		}
		return aws.SecretValueInput{Kind: aws.ValueText, Text: string(data)}, nil
	}
	if stdin {
		data, err := io.ReadAll(in)
		if err != nil {
			return aws.SecretValueInput{}, fmt.Errorf("read --stdin: %w", err)
		}
		return aws.SecretValueInput{Kind: aws.ValueText, Text: string(data)}, nil
	}
	kind, prefix := aws.ValueText, ""
	if strings.HasPrefix(positional, "fileb://") {
		kind, prefix = aws.ValueBinary, "fileb://"
	} else if strings.HasPrefix(positional, "file://") {
		prefix = "file://"
	}
	if prefix != "" {
		path := strings.TrimPrefix(positional, prefix)
		if path == "" {
			return aws.SecretValueInput{}, fmt.Errorf("%s value requires a path", strings.TrimSuffix(prefix, "://"))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return aws.SecretValueInput{}, fmt.Errorf("read %s path %q: %w", strings.TrimSuffix(prefix, "://"), path, err)
		}
		if kind == aws.ValueBinary {
			return aws.SecretValueInput{Kind: kind, Binary: data}, nil
		}
		return aws.SecretValueInput{Kind: kind, Text: string(data)}, nil
	}
	return aws.SecretValueInput{Kind: aws.ValueText, Text: positional}, nil
}

// parseTags parses tags from string format "key1=value1,key2=value2"
func parseTags(tagsStr string) (map[string]string, error) {
	if tagsStr == "" {
		return nil, nil
	}

	tags := make(map[string]string)
	pairs := strings.Split(tagsStr, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format: %s (expected key=value)", pair)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("empty tag key in: %s", pair)
		}

		tags[key] = value
	}

	return tags, nil
}

// formatTags formats tags for display
func formatTags(tags map[string]string) string {
	var pairs []string
	for k, v := range tags {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ", ")
}

// isValidParamType checks if the parameter type is valid
func isValidParamType(t string) bool {
	switch t {
	case "String", "StringList", "SecureString":
		return true
	default:
		return false
	}
}
