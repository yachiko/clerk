package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
)

// InitGetCommand initializes the GET command
func InitGetCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "get <name[@version|:label]>",
		Short: "Retrieve a secret from AWS Parameter Store",
		Long: `Retrieve the value of a secret from AWS Parameter Store.

By default, the secret is decrypted and displayed. Use --mask to show
a masked version of the value.

You can specify a version or label:
  - /dev/secret@3        - get version 3
  - /dev/secret@latest   - get latest version (default)
  - /dev/secret:prod     - get version with label "prod"
  - /dev/secret:staging  - get version with label "staging"

Examples:
  # Get the latest version of a secret
  clerk get "/dev/db_password"

  # Get a specific version
  clerk get "/dev/db_password@2"

  # Get by label
  clerk get "/dev/db_password:prod"

  # Get with masked value
  clerk get "/dev/db_password" --mask

  # Get only the value (useful for scripts)
  clerk get "/dev/db_password" --value

  # Get as JSON
  clerk get "/dev/db_password" --output json`,
		Args: cobra.ExactArgs(1),
		RunE: runGet,
	}

	getCmd.Flags().BoolVar(&getMask, "mask", false, "Show masked value instead of actual value")
	getCmd.Flags().BoolVar(&getValueOnly, "value", false, "Output only the value (no metadata)")

	return getCmd
}

func runGet(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nameWithVersionOrLabel := args[0]

	// Parse name, version, and label
	name, version, label, err := parseNameVersionLabel(nameWithVersionOrLabel)
	if err != nil {
		return err
	}

	// Validate name starts with /
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("parameter name must start with /")
	}

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()

	// Create AWS client
	region := globalOpts.Region
	if region == "" && cfg.Region != "" {
		region = cfg.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	profile := globalOpts.Profile
	if profile == "" && cfg.Profile != "" {
		profile = cfg.Profile
	}

	awsOpts := aws.ClientOptions{
		Region:           region,
		Profile:          profile,
		DescribePageSize: cfg.DescribePageSize,
		DescribeMaxItems: cfg.DescribeMaxItems,
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

	// Handle value masking
	displayValue := param.Value
	if getMask {
		displayValue = util.MaskValue(param.Value)
	}

	// Output
	return outputParameter(param, displayValue)
}

// parseNameVersionLabel parses "name@version" or "name:label" format
// Returns name, version, label, error
func parseNameVersionLabel(input string) (string, int64, string, error) {
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
		fmt.Println(displayValue)
		return nil
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

	// Plain output
	bold := color.New(color.Bold)
	cyan := color.New(color.FgCyan)

	bold.Println("Name:", param.Name)

	// Show value with appropriate styling
	bold.Print("Value: ")
	if getMask {
		color.Yellow(displayValue)
	} else {
		fmt.Println(displayValue)
	}

	cyan.Printf("Type: %s\n", param.Type)
	cyan.Printf("Version: %d\n", param.Version)
	cyan.Printf("Last Modified: %s\n", param.LastModifiedDate.Format(time.RFC3339))

	if param.ARN != "" {
		cyan.Printf("ARN: %s\n", param.ARN)
	}

	if len(param.Tags) > 0 {
		cyan.Print("Tags: ")
		var tagPairs []string
		for k, v := range param.Tags {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Println(strings.Join(tagPairs, ", "))
	}

	return nil
}
