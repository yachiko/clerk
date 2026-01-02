# Task 07: GET Command

## Objective
Implement the `clerk get` command for retrieving secrets from AWS Parameter Store.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 05 completed (utility modules)

## Command Specification

```
clerk get <name[@version]> [flags]

Arguments:
  name     Parameter name (must start with /)
           Optionally include @version to get specific version (e.g., /dev/secret@3)
           Use @latest for latest version (default behavior)

Flags:
  --mask           Show masked value instead of actual value
  --value          Output only the value (no metadata)
  --output string  Output format: plain, json (default "plain")
  -h, --help       Help for get

Global Flags:
  --profile string  AWS profile to use
  --region string   AWS region to use
```

## Deliverables

### 1. Create GET Command

Create file `internal/cli/get.go`:

```go
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
	getOutput    string
)

func init() {
	rootCmd.AddCommand(getCmd)

	getCmd.Flags().BoolVar(&getMask, "mask", false, "Show masked value instead of actual value")
	getCmd.Flags().BoolVar(&getValueOnly, "value", false, "Output only the value (no metadata)")
	getCmd.Flags().StringVarP(&getOutput, "output", "o", "plain", "Output format: plain, json")
}

var getCmd = &cobra.Command{
	Use:   "get <name[@version]>",
	Short: "Retrieve a secret from AWS Parameter Store",
	Long: `Retrieve the value of a secret from AWS Parameter Store.

By default, the secret is decrypted and displayed. Use --mask to show
a masked version of the value.

You can specify a version using the @version syntax:
  - /dev/secret@3    - get version 3
  - /dev/secret@latest - get latest version (default)

Examples:
  # Get the latest version of a secret
  clerk get "/dev/db_password"

  # Get a specific version
  clerk get "/dev/db_password@2"

  # Get with masked value
  clerk get "/dev/db_password" --mask

  # Get only the value (useful for scripts)
  clerk get "/dev/db_password" --value

  # Get as JSON
  clerk get "/dev/db_password" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nameWithVersion := args[0]

	// Parse name and version
	name, version, err := parseNameAndVersion(nameWithVersion)
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
	awsOpts := aws.ClientOptions{
		Region:  region,
		Profile: profile,
	}
	if awsOpts.Region == "" {
		awsOpts.Region = cfg.Region
	}
	if awsOpts.Profile == "" {
		awsOpts.Profile = cfg.Profile
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Get parameter
	var param *aws.Parameter
	if version > 0 {
		param, err = client.GetParameterByVersion(ctx, name, version, !getMask)
	} else {
		param, err = client.GetParameter(ctx, name, !getMask)
	}
	if err != nil {
		if aws.IsParameterNotFoundError(err) {
			return fmt.Errorf("parameter not found: %s", name)
		}
		return fmt.Errorf("failed to get parameter: %w", err)
	}

	// Handle value masking
	displayValue := param.Value
	if getMask {
		displayValue = util.MaskValue(param.Value)
	}

	// Output
	return outputParameter(param, displayValue)
}

// parseNameAndVersion parses "name@version" format
func parseNameAndVersion(input string) (string, int64, error) {
	// Check for @ symbol
	atIndex := strings.LastIndex(input, "@")
	if atIndex == -1 {
		return input, 0, nil
	}

	name := input[:atIndex]
	versionStr := input[atIndex+1:]

	// Handle @latest
	if strings.EqualFold(versionStr, "latest") {
		return name, 0, nil
	}

	// Parse version number
	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid version number: %s", versionStr)
	}

	if version < 1 {
		return "", 0, fmt.Errorf("version must be a positive number")
	}

	return name, version, nil
}

// outputParameter outputs the parameter in the requested format
func outputParameter(param *aws.Parameter, displayValue string) error {
	// Value-only output
	if getValueOnly {
		fmt.Println(displayValue)
		return nil
	}

	// JSON output
	if getOutput == "json" {
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
```

## Acceptance Criteria

- [ ] `clerk get "/test/secret"` retrieves and displays the secret
- [ ] `clerk get "/test/secret@2"` retrieves version 2
- [ ] `clerk get "/test/secret@latest"` retrieves latest version
- [ ] `--mask` flag displays masked value (e.g., `my****rd`)
- [ ] `--value` flag outputs only the value with no metadata
- [ ] `--output json` outputs JSON format
- [ ] SecureString parameters are decrypted by default
- [ ] Exit code 0 on success
- [ ] Exit code 2 on AWS error
- [ ] Exit code 3 on invalid input (bad name format, invalid version)
- [ ] "Parameter not found" error when parameter doesn't exist
- [ ] Tags are displayed when present

## Example Output

### Plain Output
```
Name: /dev/db_password
Value: mypassword123
Type: SecureString
Version: 3
Last Modified: 2026-01-02T10:30:00Z
ARN: arn:aws:ssm:us-east-1:123456789:parameter/dev/db_password
Tags: env=dev, team=backend
```

### Plain Output (Masked)
```
Name: /dev/db_password
Value: my**********23
Type: SecureString
Version: 3
Last Modified: 2026-01-02T10:30:00Z
```

### Value-Only Output
```
mypassword123
```

### JSON Output
```json
{
  "name": "/dev/db_password",
  "value": "mypassword123",
  "type": "SecureString",
  "version": 3,
  "last_modified_date": "2026-01-02T10:30:00Z",
  "arn": "arn:aws:ssm:us-east-1:123456789:parameter/dev/db_password",
  "tags": {
    "env": "dev",
    "team": "backend"
  }
}
```

## Notes

- The `@` syntax for version is parsed from the last `@` to allow parameter names with `@`
- `@latest` is case-insensitive
- Version 0 means "latest" internally
- `--value` flag is useful for shell scripts: `export DB_PASS=$(clerk get "/dev/db_password" --value)`
- Masked output still fetches the real value (for length calculation) but doesn't display it
- Tags are fetched separately via `ListTagsForResource` API
