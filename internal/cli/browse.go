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
		Short: "Interactively browse secrets in AWS Parameter Store",
		Long: `Start an interactive terminal UI to browse and manage secrets.

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

	// Create AWS client
	region := globalOpts.Region
	profile := globalOpts.Profile

	// Use config defaults if global options are empty
	if region == "" && cfg.Region != "" {
		region = cfg.Region
	}
	if region == "" {
		region = "us-east-1" // Final fallback
	}
	if profile == "" && cfg.Profile != "" {
		profile = cfg.Profile
	}

	awsOpts := aws.ClientOptions{
		Region:  region,
		Profile: profile,
	}

	client, err := aws.NewClient(ctx, awsOpts)
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	// Initialize cache
	cacheMgr, err := cache.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Always refresh cache if empty or expired
	if cacheMgr.IsExpired() || len(cacheMgr.GetAll()) == 0 {
		fmt.Println("Loading parameters...")
		err = cacheMgr.Refresh(ctx, client, region, cfg.ParallelFetches, nil)
		if err != nil {
			return fmt.Errorf("failed to refresh cache: %w", err)
		}
	}

	// Create and run UI (with new context for UI operations)
	model := ui.NewModel(client, cacheMgr, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}
