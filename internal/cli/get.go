package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/util"
)

var (
	getMask      bool
	getValueOnly bool
	getStage     string
	getVersionID string
	getRaw       bool
)

// InitGetCommand initializes the GET command
func InitGetCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "get <name-or-secret-id>",
		Short: "Retrieve one value from a concrete secret backend",
		Long: `Retrieve one value from Parameter Store or Secrets Manager.

You must explicitly choose --backend ssm or --backend secretsmanager. SSM keeps
the name@version and name:label shorthand. Secrets Manager uses --stage or
--version-id and defaults to AWSCURRENT. Binary secrets are base64 unless
--raw --value is requested.

By default, the secret is decrypted and displayed. Use --mask to show
a masked version of the value.

You can specify a version or label:
  - /dev/secret@3        - get version 3
  - /dev/secret@latest   - get latest version (default)
  - /dev/secret:prod     - get version with label "prod"
  - /dev/secret:staging  - get version with label "staging"

Examples:
  # Get the latest version of a secret
  clerk get "/dev/db_password" --backend ssm

  # Get a specific version
  clerk get "/dev/db_password@2" --backend ssm

  # Get by label
  clerk get "/dev/db_password:prod" --backend ssm

  # Get with masked value
  clerk get "/dev/db_password" --mask --backend ssm

  # Get only the value (useful for scripts)
  clerk get "/dev/db_password" --value --backend ssm

  # Get as JSON
  clerk get "/dev/db_password" --output json --backend ssm

  # Get a Secrets Manager version
  clerk get "dev/db_password" --backend secretsmanager --stage AWSCURRENT`,
		Args:    cobra.ExactArgs(1),
		PreRunE: validateGetFlags,
		RunE:    runGet,
	}

	getCmd.Flags().BoolVar(&getMask, "mask", false, "Show masked value instead of actual value")
	getCmd.Flags().BoolVar(&getValueOnly, "value", false, "Output only the value (no metadata)")
	getCmd.Flags().StringVar(&getStage, "stage", "", "Secrets Manager version stage (default AWSCURRENT)")
	getCmd.Flags().StringVar(&getVersionID, "version-id", "", "Secrets Manager opaque version ID")
	getCmd.Flags().BoolVar(&getRaw, "raw", false, "Write exact SecretBinary bytes (requires --value)")

	return getCmd
}

func validateGetFlags(cmd *cobra.Command, _ []string) error {
	backend, err := requireConcreteBackend(cmd, "get", true)
	if err != nil {
		return err
	}
	if getStage != "" && getVersionID != "" {
		return fmt.Errorf("--stage and --version-id are mutually exclusive")
	}
	if getRaw && !getValueOnly {
		return fmt.Errorf("--raw is valid only with --value")
	}
	if backend == aws.BackendSSM && (getStage != "" || getVersionID != "" || getRaw) {
		return fmt.Errorf("--stage, --version-id, and --raw are supported only with --backend secretsmanager")
	}
	return nil
}

func runGet(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backend, err := requireConcreteBackend(cmd, "get", true)
	if err != nil {
		return err
	}
	nameWithVersionOrLabel := args[0]
	if backend == aws.BackendSecretsManager {
		return runSecretsManagerGet(ctx, cmd, nameWithVersionOrLabel)
	}

	// Parse name, version, and label
	name, version, label, err := parseNameVersionLabel(nameWithVersionOrLabel)
	if err != nil {
		return err
	}

	if err := validateParameterIdentifier(name, true); err != nil {
		return err
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Create AWS client
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get parameter (prioritize label over version)
	var param *aws.Parameter
	if label != "" {
		param, err = client.GetParameterByLabel(ctx, name, label, !getMask)
		if err != nil {
			if aws.IsParameterNotFoundError(err) {
				return fmt.Errorf("parameter with label %q not found: %s", label, name)
			}
			return fmt.Errorf("failed to get parameter by label: %w", err)
		}
	} else if version > 0 {
		param, err = client.GetParameterByVersion(ctx, name, version, !getMask)
		if err != nil {
			if aws.IsParameterNotFoundError(err) {
				return fmt.Errorf("parameter version %d not found: %s", version, name)
			}
			return fmt.Errorf("failed to get parameter version: %w", err)
		}
	} else {
		param, err = client.GetParameter(ctx, name, !getMask)
		if err != nil {
			if aws.IsParameterNotFoundError(err) {
				return fmt.Errorf("parameter not found: %s", name)
			}
			return fmt.Errorf("failed to get parameter: %w", err)
		}
	}
	// Detail output includes tags when they are readable. Keep raw --value
	// deliberately narrow so scripts do not acquire an unnecessary tag
	// permission or extra request.
	if !getValueOnly {
		if tags, tagErr := client.GetParameterTags(ctx, param.Name); tagErr == nil {
			param.Tags = tags
		}
	}

	// Handle value masking
	displayValue := param.Value
	if getMask {
		displayValue = util.MaskValue(param.Value)
	}

	// Output
	return outputParameter(param, displayValue)
}

func runSecretsManagerGet(ctx context.Context, cmd *cobra.Command, secretID string) error {
	if strings.TrimSpace(secretID) == "" {
		return fmt.Errorf("secret ID is required")
	}
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	awsOpts, err := resolveAWSOptions(cmd, cfgMgr.Get())
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
	detail, err := client.GetSecretValue(ctx, secretID, aws.SecretValueSelector{VersionStage: getStage, VersionID: getVersionID})
	if err != nil {
		return err
	}
	return outputSecret(detail)
}

func outputSecret(detail *aws.SecretDetail) error {
	return outputSecretTo(os.Stdout, detail)
}

func outputSecretTo(w io.Writer, detail *aws.SecretDetail) error {
	if detail.Value.Kind == aws.ValueBinary && getMask {
		return fmt.Errorf("--mask is not valid for binary secrets")
	}
	if detail.Value.Kind != aws.ValueBinary && getRaw {
		return fmt.Errorf("--raw is valid only for binary secrets")
	}
	if getValueOnly {
		if detail.Value.Kind == aws.ValueBinary {
			if getRaw {
				_, err := w.Write(detail.Value.Binary)
				return err
			}
			_, err := fmt.Fprint(w, base64.StdEncoding.EncodeToString(detail.Value.Binary))
			return err
		}
		value := detail.Value.Text
		if getMask {
			value = util.MaskValue(value)
		}
		_, err := fmt.Fprint(w, value)
		return err
	}
	value, encoding := detail.Value.Text, "string"
	if detail.Value.Kind == aws.ValueBinary {
		value, encoding = base64.StdEncoding.EncodeToString(detail.Value.Binary), "base64"
	} else if getMask {
		value = util.MaskValue(value)
	}
	if globalOpts.Output == "json" {
		result := struct {
			Backend       aws.Backend `json:"backend"`
			Name          string      `json:"name"`
			ARN           string      `json:"arn"`
			Value         string      `json:"value"`
			ValueEncoding string      `json:"value_encoding"`
			VersionID     string      `json:"version_id"`
			VersionStages []string    `json:"version_stages,omitempty"`
			CreatedDate   *time.Time  `json:"created_date,omitempty"`
		}{aws.BackendSecretsManager, detail.Name, detail.ARN, value, encoding, detail.VersionID, detail.VersionStages, detail.CreatedDate}
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	fmt.Fprintf(w, "Name: %s\n", util.SanitizeTerminal(detail.Name))
	fmt.Fprintf(w, "Value: %s\n", util.SanitizeTerminal(value))
	fmt.Fprintf(w, "Encoding: %s\n", encoding)
	fmt.Fprintf(w, "Backend: %s\n", aws.BackendSecretsManager)
	fmt.Fprintf(w, "Version ID: %s\n", util.SanitizeTerminal(detail.VersionID))
	fmt.Fprintf(w, "ARN: %s\n", util.SanitizeTerminal(detail.ARN))
	return nil
}

// parseNameVersionLabel parses "name@version" or "name:label" format
// Returns name, version, label, error
func parseNameVersionLabel(input string) (string, int64, string, error) {
	// SSM ARNs contain several colons. They are complete identifiers and do not
	// use Clerk's name:label shorthand.
	if strings.HasPrefix(input, "arn:") {
		return input, 0, "", nil
	}
	// Check for :label syntax first
	colonIndex := strings.LastIndex(input, ":")
	if colonIndex != -1 {
		name := input[:colonIndex]
		label := input[colonIndex+1:]

		if label == "" {
			return "", 0, "", fmt.Errorf("label cannot be empty")
		}

		return name, 0, label, nil
	}

	// Check for @version syntax
	atIndex := strings.LastIndex(input, "@")
	if atIndex == -1 {
		return input, 0, "", nil
	}

	name := input[:atIndex]
	versionStr := input[atIndex+1:]

	// Handle @latest
	if strings.EqualFold(versionStr, "latest") {
		return name, 0, "", nil
	}

	// Parse version number
	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid version number: %s", versionStr)
	}

	if version < 1 {
		return "", 0, "", fmt.Errorf("version must be a positive number")
	}

	return name, version, "", nil
}

// outputParameter outputs the parameter in the requested format
func outputParameter(param *aws.Parameter, displayValue string) error {
	// Value-only output
	if getValueOnly {
		_, err := fmt.Fprint(os.Stdout, displayValue)
		return err
	}

	// JSON output
	if globalOpts.Output == "json" {
		output := struct {
			Name             string            `json:"name"`
			Value            string            `json:"value"`
			Type             string            `json:"type"`
			Version          int64             `json:"version"`
			LastModifiedDate time.Time         `json:"last_modified_date"`
			ARN              string            `json:"arn,omitempty"`
			Tags             map[string]string `json:"tags,omitempty"`
		}{
			Name:             param.Name,
			Value:            displayValue,
			Type:             param.Type,
			Version:          param.Version,
			LastModifiedDate: param.LastModifiedDate,
			ARN:              param.ARN,
			Tags:             param.Tags,
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	// Plain output. Errors from color.Print / fmt.Print on stdout are
	// discarded explicitly — there's nothing useful clerk can do if stdout
	// is closed mid-write, and forwarding the error would just mask the
	// underlying terminal/pipe failure.
	bold := color.New(color.Bold)
	cyan := color.New(color.FgCyan)

	_, _ = bold.Println("Name:", util.SanitizeTerminal(param.Name))

	// Show value with appropriate styling
	_, _ = bold.Print("Value: ")
	if getMask {
		color.Yellow(util.SanitizeTerminal(displayValue))
	} else {
		fmt.Println(util.SanitizeTerminal(displayValue))
	}

	_, _ = cyan.Printf("Type: %s\n", util.SanitizeTerminal(param.Type))
	_, _ = cyan.Printf("Version: %d\n", param.Version)
	_, _ = cyan.Printf("Last Modified: %s\n", param.LastModifiedDate.Format(time.RFC3339))

	if param.ARN != "" {
		_, _ = cyan.Printf("ARN: %s\n", util.SanitizeTerminal(param.ARN))
	}

	if len(param.Tags) > 0 {
		_, _ = cyan.Print("Tags: ")
		var tagPairs []string
		for k, v := range param.Tags {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", util.SanitizeTerminal(k), util.SanitizeTerminal(v)))
		}
		fmt.Println(strings.Join(tagPairs, ", "))
	}

	return nil
}
