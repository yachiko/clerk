package cli

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/ui"
)

// InitBrowseCommand initializes the BROWSE command
func InitBrowseCommand() *cobra.Command {
	browseCmd := &cobra.Command{
		Use:   "browse",
		Short: "Interactively browse secret metadata",
		Long: `Start an interactive terminal UI for the selected secret backend.

The default view combines Parameter Store (SSM) and Secrets Manager (SM) in one
hierarchy. Secrets Manager actions are read-only.

Keyboard shortcuts:
  Navigation:
    ↑/↓, j/k     Move selection up/down
    PgUp/PgDn    Move page up/down
    Home/End     Jump to first/last item
    
  Actions:
    d, Enter     Describe selected secret
    c            Copy secret value to clipboard
    e            Edit secret in $EDITOR
    Delete       Delete secret (with confirmation)
    
  View:
    /            Search/filter
    t            Toggle tree/flat view
    Space        Expand/collapse (tree view)
    
  General:
    q            Quit / Back
    Esc          Cancel search / Close describe

Examples:
  clerk browse
  clerk browse --profile production`,
		RunE: runBrowse,
	}

	return browseCmd
}

func runBrowse(cmd *cobra.Command, args []string) error {
	// Short timeout for initial setup only
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load config
	cfgMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgMgr.Get()
	backend, err := selectedBackend(cmd)
	if err != nil {
		return err
	}

	// Create AWS client
	awsOpts, err := resolveAWSOptions(cmd, cfg)
	if err != nil {
		return err
	}

	resolved, err := aws.ResolveContext(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve AWS context: %w", err)
	}
	var ssmClient *aws.Client
	if backend == aws.BackendAll || backend == aws.BackendSSM {
		ssmClient, err = aws.NewClientFromContext(resolved, awsOpts)
		if err != nil {
			return fmt.Errorf("failed to create SSM client: %w", err)
		}
	}
	var secretsClient *aws.SecretsManagerClient
	if backend == aws.BackendAll || backend == aws.BackendSecretsManager {
		secretsClient, err = aws.NewSecretsManagerClient(resolved)
		if err != nil {
			return fmt.Errorf("failed to create Secrets Manager client: %w", err)
		}
	}

	caches := make(map[aws.Backend]*cache.Manager)
	for _, candidate := range []aws.Backend{aws.BackendSSM, aws.BackendSecretsManager} {
		if backend != aws.BackendAll && backend != candidate {
			continue
		}
		manager, cacheErr := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, candidate)
		if cacheErr != nil {
			return fmt.Errorf("failed to initialize %s cache: %w", candidate, cacheErr)
		}
		caches[candidate] = manager
	}

	// Create and run UI immediately - background refresh will load data
	scope := aws.ResourceIdentity{Partition: resolved.Partition, AccountID: resolved.AccountID, Region: resolved.Region}
	model := ui.NewCombinedModel(ssmClient, secretsClient, caches, cfg, backend, scope)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}
