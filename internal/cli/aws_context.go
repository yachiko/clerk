package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

// resolveAWSOptions applies the single precedence order used by every data
// command: command-line flag, explicitly saved Clerk setting, then the SDK's
// environment/shared-config resolution.
func resolveAWSOptions(cmd *cobra.Command, cfg *config.Config) (aws.ClientOptions, error) {
	if cfg == nil {
		return aws.ClientOptions{}, fmt.Errorf("configuration is required")
	}
	region := cfg.Region
	profile := cfg.Profile
	profileSet := profile != ""
	if cmd.Root().PersistentFlags().Changed("region") {
		region = globalOpts.Region
	}
	if cmd.Root().PersistentFlags().Changed("profile") {
		profile, profileSet = globalOpts.Profile, true
	}
	return aws.ClientOptions{
		Region: region, Profile: profile, ProfileSet: profileSet,
		DescribePageSize: cfg.DescribePageSize, DescribeMaxItems: cfg.DescribeMaxItems,
	}, nil
}
