package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
)

var tagValues string
var untagKeys string

func InitTagCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag <name-or-arn> [key=value ...]",
		Short: "Add or replace tags in the selected backend",
		Long: `Add or replace resource tags in Parameter Store or Secrets Manager.

Parameter Store is used by default. Supply tags as positional key=value values
or as a comma-separated --tags value.`,
		Args: cobra.MinimumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if _, err := selectedBackend(cmd); err != nil {
				return err
			}
			_, err := tagArguments(args[1:], tagValues)
			return err
		},
		RunE: runTag,
	}
	cmd.Flags().StringVar(&tagValues, "tags", "", "Comma-separated tags in key=value format")
	return cmd
}

func InitUntagCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "untag <name-or-arn> [key ...]",
		Short: "Remove tags in the selected backend",
		Long: `Remove resource tags from Parameter Store or Secrets Manager.

Parameter Store is used by default. Supply keys positionally or as a
comma-separated --keys value.`,
		Args: cobra.MinimumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if _, err := selectedBackend(cmd); err != nil {
				return err
			}
			_, err := untagArguments(args[1:], untagKeys)
			return err
		},
		RunE: runUntag,
	}
	cmd.Flags().StringVar(&untagKeys, "keys", "", "Comma-separated tag keys")
	return cmd
}

func tagArguments(positional []string, flag string) (map[string]string, error) {
	if len(positional) > 0 && flag != "" {
		return nil, fmt.Errorf("positional tags cannot be combined with --tags")
	}
	input := flag
	if len(positional) > 0 {
		input = strings.Join(positional, ",")
	}
	if input == "" {
		return nil, fmt.Errorf("at least one tag is required")
	}
	tags, err := parseTags(input)
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("at least one tag is required")
	}
	return tags, nil
}

func untagArguments(positional []string, flag string) ([]string, error) {
	if len(positional) > 0 && flag != "" {
		return nil, fmt.Errorf("positional tag keys cannot be combined with --keys")
	}
	keys := positional
	if flag != "" {
		keys = strings.Split(flag, ",")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one tag key is required")
	}
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("tag keys cannot be empty")
		}
		result = append(result, key)
	}
	return result, nil
}

func runTag(cmd *cobra.Command, args []string) error {
	tags, err := tagArguments(args[1:], tagValues)
	if err != nil {
		return err
	}
	return mutateTags(cmd, args[0], tags, nil)
}

func runUntag(cmd *cobra.Command, args []string) error {
	keys, err := untagArguments(args[1:], untagKeys)
	if err != nil {
		return err
	}
	return mutateTags(cmd, args[0], nil, keys)
}

func mutateTags(cmd *cobra.Command, name string, tags map[string]string, keys []string) error {
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}
	if backend == aws.BackendSSM {
		if err := validateParameterIdentifier(name, false); err != nil {
			return err
		}
	} else if strings.TrimSpace(name) == "" {
		return fmt.Errorf("secret ID cannot be empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	options, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}
	if backend == aws.BackendSSM {
		client, err := aws.NewClient(ctx, options)
		if err != nil {
			return fmt.Errorf("failed to create AWS client: %w", err)
		}
		if len(tags) > 0 {
			err = client.AddTagsToResource(ctx, name, tags)
		} else {
			err = client.RemoveTagsFromResource(ctx, name, keys)
		}
		if err != nil {
			return err
		}
		reconcileParameterTags(ctx, cfg, client, name)
	} else {
		resolved, err := aws.ResolveContext(ctx, options)
		if err != nil {
			return fmt.Errorf("failed to resolve AWS context: %w", err)
		}
		client, err := aws.NewSecretsManagerClient(resolved)
		if err != nil {
			return fmt.Errorf("failed to create Secrets Manager client: %w", err)
		}
		if len(tags) > 0 {
			err = client.TagResource(ctx, aws.TagSecretRequest{SecretID: name, Tags: tags})
		} else {
			err = client.UntagResource(ctx, aws.UntagSecretRequest{SecretID: name, TagKeys: keys})
		}
		if err != nil {
			return err
		}
		reconcileSecretCache(ctx, cfg, resolved, client, name)
	}
	return outputTagResult(os.Stdout, backend, name, tags, keys)
}

func outputTagResult(out io.Writer, backend aws.Backend, name string, tags map[string]string, keys []string) error {
	action := "tagged"
	if len(keys) > 0 {
		action = "untagged"
	}
	if globalOpts.Output == "json" {
		return json.NewEncoder(out).Encode(struct {
			Name    string            `json:"name"`
			Backend aws.Backend       `json:"backend"`
			Action  string            `json:"action"`
			Tags    map[string]string `json:"tags,omitempty"`
			Keys    []string          `json:"keys,omitempty"`
		}{name, backend, action, tags, keys})
	}
	_, err := fmt.Fprintf(out, "%s %s: %s\n", strings.ToUpper(action[:1])+action[1:], map[aws.Backend]string{aws.BackendSSM: "parameter", aws.BackendSecretsManager: "secret"}[backend], name)
	return err
}
