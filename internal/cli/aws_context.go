package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

// resolveAWSOptions applies the single precedence order used by every data
// command: command-line flag, explicitly saved Clerk setting, then the SDK's
// environment/shared-config resolution. A saved profile yields to static
// credentials in the environment (e.g. from an assume-role wrapper), because
// the SDK would otherwise let the profile shadow them; --profile still wins.
func resolveAWSOptions(cmd *cobra.Command, cfg *config.Config) (aws.ClientOptions, error) {
	if cfg == nil {
		return aws.ClientOptions{}, fmt.Errorf("configuration is required")
	}
	region := cfg.Region
	profile := cfg.Profile
	profileSet := profile != "" && !envHasStaticCredentials()
	if cmd.Root().PersistentFlags().Changed("region") {
		region = globalOpts.Region
	}
	if cmd.Root().PersistentFlags().Changed("profile") {
		profile, profileSet = globalOpts.Profile, true
	}
	return aws.ClientOptions{
		Region: region, Profile: profile, ProfileSet: profileSet,
		DescribePageSize: cfg.DescribePageSize, DescribeMaxItems: cfg.DescribeMaxItems,
		Backend: globalOpts.Backend,
	}, nil
}

// envHasStaticCredentials mirrors the SDK's check for an access key pair in
// the environment, including its legacy variable names.
func envHasStaticCredentials() bool {
	return firstEnv("AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY") != "" &&
		firstEnv("AWS_SECRET_ACCESS_KEY", "AWS_SECRET_KEY") != ""
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func requireWritableBackend() error {
	if globalOpts.Backend == "secretsmanager" {
		return fmt.Errorf("the secretsmanager backend is read-only; put, delete, cp, and mv are not supported")
	}
	return nil
}
