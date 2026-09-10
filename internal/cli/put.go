package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/util"
)

var (
	putTags     string
	putType     string
	putKMSKeyID string
	putFile     string
	putStdin    bool
)

// InitPutCommand initializes the PUT command
func InitPutCommand() *cobra.Command {
	putCmd := &cobra.Command{
		Use:   "put <name> [value]",
		Short: "Create or update a secret in AWS Parameter Store",
		Long: `Create a new secret or update an existing one in AWS Parameter Store.

Positional values are literal. Use --file or --stdin for exact bytes.

Examples:
  # Create a secret with a string value
  clerk put "/dev/db_password" "mypassword123"

  # Create a secret from a file
  clerk put "/dev/api_key" ./secrets/api_key.txt

  # Create with tags
  clerk put "/dev/db_password" "mypassword123" --tags "env=dev,team=backend"

  # Create as StringList
  clerk put "/dev/allowed_ips" "10.0.0.1,10.0.0.2" --type StringList

  # Create with custom KMS key
  clerk put "/prod/secret" "value" --kms-key-id "alias/my-key"`,
		Args: validatePutArgs,
		RunE: runPut,
	}

	putCmd.Flags().StringVar(&putTags, "tags", "", "Tags in format key1=value1,key2=value2")
	putCmd.Flags().StringVar(&putType, "type", "", "Parameter type: String, StringList, SecureString")
	putCmd.Flags().StringVar(&putKMSKeyID, "kms-key-id", "", "KMS key ID for SecureString encryption")
	putCmd.Flags().StringVar(&putFile, "file", "", "Read an exact value from this file")
	putCmd.Flags().BoolVar(&putStdin, "stdin", false, "Read an exact value from standard input")

	return putCmd
}

func runPut(cmd *cobra.Command, args []string) error {
	if err := requireWritableBackend(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := args[0]
	valueOrFile := ""
	if len(args) == 2 {
		valueOrFile = args[1]
	}

	if err := validateParameterIdentifier(name, false); err != nil {
		return err
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	value, err := resolveValue(valueOrFile, putFile, putStdin, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to resolve value: %w", err)
	}

	// Parse tags
	tags, err := parseTags(putTags)
	if err != nil {
		return fmt.Errorf("failed to parse tags: %w", err)
	}

	// Create AWS client
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

	// Update cache with region and account ID
	cacheMgr, err := cache.NewManager(cfg, client.GetRegion(), client.GetAccountID(), client.GetBackend())
	if err == nil {
		cacheEntry := cache.CacheEntry{
			Name:             name,
			Type:             paramType,
			Version:          output.Version,
			LastModifiedDate: time.Now(),
			Tags:             tags,
		}
		_ = cacheMgr.Update(cacheEntry)
	}

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
