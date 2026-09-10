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

Parameter Store is used by default. Each backend has a provider-specific view
and actions.

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
	manager, err := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize %s cache: %w", backend, err)
	}
	scope := aws.ResourceIdentity{Partition: resolved.Partition, AccountID: resolved.AccountID, Region: resolved.Region, Backend: backend}
	var model tea.Model
	switch backend {
	case aws.BackendSSM:
		client, err := aws.NewClientFromContext(resolved, awsOpts)
		if err != nil {
			return fmt.Errorf("failed to create SSM client: %w", err)
		}
		model = ui.NewSSMModel(client, manager, cfg, scope)
	case aws.BackendSecretsManager:
		client, err := aws.NewSecretsManagerClient(resolved)
		if err != nil {
			return fmt.Errorf("failed to create Secrets Manager client: %w", err)
		}
		model = ui.NewSecretsManagerModel(client, manager, cfg, scope)
	default:
		return fmt.Errorf("unsupported browse backend %q", backend)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}
