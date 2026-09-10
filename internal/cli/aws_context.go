package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

func selectedBackend(cmd *cobra.Command) (aws.Backend, error) {
	return aws.ParseBackend(globalOpts.Backend)
}

func backendWasExplicit(cmd *cobra.Command) bool {
	return cmd.Root().PersistentFlags().Changed("backend")
}

func requireConcreteBackend(cmd *cobra.Command, operation string, explicit bool) (aws.Backend, error) {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return "", err
	}
	if explicit && !backendWasExplicit(cmd) {
		return "", fmt.Errorf("%s requires an explicit --backend ssm or --backend secretsmanager", operation)
	}
	if backend == aws.BackendAll {
		return "", fmt.Errorf("%s requires one concrete backend; choose --backend ssm or --backend secretsmanager", operation)
	}
	return backend, nil
}

func requireExplicitSSM(cmd *cobra.Command) error {
	if !backendWasExplicit(cmd) {
		return fmt.Errorf("%s requires explicit --backend ssm", cmd.Name())
	}
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	if backend != aws.BackendSSM {
		return fmt.Errorf("%s is supported only with --backend ssm", cmd.Name())
	}
	return nil
}

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
