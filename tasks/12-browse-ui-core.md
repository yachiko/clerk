# Task 12: Browse UI - Core Framework

## Objective
Implement the core browse UI framework including the main view, keyboard handling, and state management.

## Prerequisites
- Task 01 completed (project setup)
- Task 02 completed (configuration module)
- Task 03 completed (AWS SSM client)
- Task 04 completed (cache module)
- Task 05 completed (utility modules)

## Technical Approach

Use `charmbracelet/bubbletea` for the terminal UI instead of `manifoldco/promptui`. Bubbletea provides:
- Elm-architecture for predictable state management
- Built-in keyboard handling
- Flexible layouts
- Better support for complex interactive UIs

Add dependency:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
```

## Deliverables

### 1. Create UI Types

Create file `internal/ui/types.go`:

```go
package ui

import "github.com/yachiko/clerk/internal/cache"

// ViewMode represents the current view mode
type ViewMode int

const (
	ViewModeList ViewMode = iota
	ViewModeTree
	ViewModeDescribe
)

// State represents the UI state
type State struct {
	// Data
	Entries       []cache.CacheEntry
	FilteredItems []cache.CacheEntry

	// View state
	Mode          ViewMode
	SelectedIndex int
	ScrollOffset  int

	// Search
	SearchQuery  string
	SearchActive bool

	// Describe view state
	DescribeEntry   *cache.CacheEntry
	DescribeValue   string
	DescribeMasked  bool
	DescribeHistory []HistoryEntry
	HistoryIndex    int

	// Tree view state
	TreeNodes     []TreeNode
	ExpandedPaths map[string]bool

	// Window dimensions
	Width  int
	Height int

	// Messages
	StatusMessage string
	ErrorMessage  string
}

// HistoryEntry represents a version history entry
type HistoryEntry struct {
	Version  int64
	Value    string
	Modified string
}

// TreeNode represents a node in the tree view
type TreeNode struct {
	Name       string
	Path       string
	IsDir      bool
	Depth      int
	Expanded   bool
	Entry      *cache.CacheEntry // nil for directories
	ChildCount int
}
```

### 2. Create Main Model

Create file `internal/ui/model.go`:

```go
package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/util"
)

// Model is the main UI model
type Model struct {
	state     State
	client    *aws.Client
	cache     *cache.Manager
	config    *config.Config
	clipboard *util.ClipboardManager

	searchInput textinput.Model
	ready       bool
	quitting    bool
}

// NewModel creates a new browse model
func NewModel(client *aws.Client, cacheMgr *cache.Manager, cfg *config.Config) Model {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Search (glob patterns supported)..."
	ti.CharLimit = 100

	return Model{
		state: State{
			Mode:          ViewModeList,
			ExpandedPaths: make(map[string]bool),
		},
		client:      client,
		cache:       cacheMgr,
		config:      cfg,
		clipboard:   util.NewClipboardManager(cfg.ClipboardTimeout),
		searchInput: ti,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.loadEntries,
	)
}

// loadEntries loads entries from cache
func (m Model) loadEntries() tea.Msg {
	entries := m.cache.GetAll()
	return entriesLoadedMsg{entries: entries}
}

type entriesLoadedMsg struct {
	entries []cache.CacheEntry
}

type statusMsg string
type errorMsg string
type clearStatusMsg struct{}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.state.Width = msg.Width
		m.state.Height = msg.Height
		m.ready = true
		return m, nil

	case entriesLoadedMsg:
		m.state.Entries = msg.entries
		m.filterEntries()
		return m, nil

	case statusMsg:
		m.state.StatusMessage = string(msg)
		m.state.ErrorMessage = ""
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case errorMsg:
		m.state.ErrorMessage = string(msg)
		m.state.StatusMessage = ""
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case clearStatusMsg:
		m.state.StatusMessage = ""
		m.state.ErrorMessage = ""
		return m, nil

	case describeLoadedMsg:
		m.state.DescribeValue = msg.value
		m.state.DescribeHistory = msg.history
		m.state.HistoryIndex = 0
		return m, nil
	}

	// Handle search input when active
	if m.state.SearchActive {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)

		// Update filter on input change
		if m.state.SearchQuery != m.searchInput.Value() {
			m.state.SearchQuery = m.searchInput.Value()
			m.filterEntries()
		}
	}

	return m, tea.Batch(cmds...)
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c", "q":
		if m.state.SearchActive {
			m.state.SearchActive = false
			m.searchInput.Blur()
			return m, nil
		}
		if m.state.Mode == ViewModeDescribe {
			m.state.Mode = ViewModeList
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	}

	// Mode-specific handling
	switch m.state.Mode {
	case ViewModeList, ViewModeTree:
		return m.handleBrowseKeys(msg)
	case ViewModeDescribe:
		return m.handleDescribeKeys(msg)
	}

	return m, nil
}

// handleBrowseKeys handles keys in browse view
func (m Model) handleBrowseKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle search input mode
	if m.state.SearchActive {
		switch msg.String() {
		case "enter", "esc":
			m.state.SearchActive = false
			m.searchInput.Blur()
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "/":
		m.state.SearchActive = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "up", "k":
		if m.state.SelectedIndex > 0 {
			m.state.SelectedIndex--
			m.adjustScroll()
		}
		return m, nil

	case "down", "j":
		if m.state.SelectedIndex < len(m.state.FilteredItems)-1 {
			m.state.SelectedIndex++
			m.adjustScroll()
		}
		return m, nil

	case "pgup", "left":
		m.state.SelectedIndex -= m.visibleRows()
		if m.state.SelectedIndex < 0 {
			m.state.SelectedIndex = 0
		}
		m.adjustScroll()
		return m, nil

	case "pgdown", "right":
		m.state.SelectedIndex += m.visibleRows()
		if m.state.SelectedIndex >= len(m.state.FilteredItems) {
			m.state.SelectedIndex = len(m.state.FilteredItems) - 1
		}
		if m.state.SelectedIndex < 0 {
			m.state.SelectedIndex = 0
		}
		m.adjustScroll()
		return m, nil

	case "home":
		m.state.SelectedIndex = 0
		m.adjustScroll()
		return m, nil

	case "end":
		m.state.SelectedIndex = len(m.state.FilteredItems) - 1
		if m.state.SelectedIndex < 0 {
			m.state.SelectedIndex = 0
		}
		m.adjustScroll()
		return m, nil

	case "t":
		// Toggle tree/list view
		if m.state.Mode == ViewModeList {
			m.state.Mode = ViewModeTree
			m.buildTree()
		} else {
			m.state.Mode = ViewModeList
		}
		return m, nil

	case " ":
		// Toggle expand/collapse in tree view
		if m.state.Mode == ViewModeTree && len(m.state.TreeNodes) > 0 {
			node := &m.state.TreeNodes[m.state.SelectedIndex]
			if node.IsDir {
				m.state.ExpandedPaths[node.Path] = !m.state.ExpandedPaths[node.Path]
				m.buildTree()
			}
		}
		return m, nil

	case "d", "enter":
		// Describe selected item
		if len(m.state.FilteredItems) > 0 {
			entry := m.state.FilteredItems[m.state.SelectedIndex]
			m.state.DescribeEntry = &entry
			m.state.Mode = ViewModeDescribe
			m.state.DescribeMasked = true // Start masked
			return m, m.loadDescribe(entry.Name)
		}
		return m, nil

	case "c":
		// Copy secret value
		if len(m.state.FilteredItems) > 0 {
			entry := m.state.FilteredItems[m.state.SelectedIndex]
			return m, m.copySecret(entry.Name)
		}
		return m, nil

	case "e":
		// Edit (handled in separate task)
		return m, func() tea.Msg {
			return statusMsg("Edit mode - press 'q' to return")
		}

	case "delete", "backspace":
		// Delete (requires confirmation - handled in separate task)
		if len(m.state.FilteredItems) > 0 {
			return m, func() tea.Msg {
				return statusMsg("Press 'd' then 'y' to confirm delete")
			}
		}
		return m, nil
	}

	return m, nil
}

// handleDescribeKeys handles keys in describe view
func (m Model) handleDescribeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state.Mode = ViewModeList
		return m, nil

	case "x":
		// Toggle masked/unmasked
		m.state.DescribeMasked = !m.state.DescribeMasked
		return m, nil

	case "c":
		// Copy value
		if m.state.DescribeValue != "" {
			return m, m.copyValue(m.state.DescribeValue)
		}
		return m, nil

	case "up", "k":
		// Navigate to older version
		if m.state.HistoryIndex < len(m.state.DescribeHistory)-1 {
			m.state.HistoryIndex++
			m.state.DescribeValue = m.state.DescribeHistory[m.state.HistoryIndex].Value
		}
		return m, nil

	case "down", "j":
		// Navigate to newer version
		if m.state.HistoryIndex > 0 {
			m.state.HistoryIndex--
			m.state.DescribeValue = m.state.DescribeHistory[m.state.HistoryIndex].Value
		}
		return m, nil
	}

	return m, nil
}

// filterEntries filters entries based on search query
func (m *Model) filterEntries() {
	if m.state.SearchQuery == "" {
		m.state.FilteredItems = m.state.Entries
	} else {
		var filtered []cache.CacheEntry
		for _, e := range m.state.Entries {
			if matchSearch(m.state.SearchQuery, e.Name) {
				filtered = append(filtered, e)
			}
		}
		m.state.FilteredItems = filtered
	}

	// Reset selection if out of bounds
	if m.state.SelectedIndex >= len(m.state.FilteredItems) {
		m.state.SelectedIndex = len(m.state.FilteredItems) - 1
	}
	if m.state.SelectedIndex < 0 {
		m.state.SelectedIndex = 0
	}
}

// matchSearch checks if name matches search query
func matchSearch(query, name string) bool {
	query = strings.ToLower(query)
	name = strings.ToLower(name)

	// Handle glob patterns
	if strings.HasSuffix(query, "/*") {
		prefix := strings.TrimSuffix(query, "/*")
		return strings.HasPrefix(name, prefix+"/") || name == prefix
	}
	if strings.HasSuffix(query, "*") {
		prefix := strings.TrimSuffix(query, "*")
		return strings.HasPrefix(name, prefix)
	}
	if strings.HasPrefix(query, "*") {
		suffix := strings.TrimPrefix(query, "*")
		return strings.HasSuffix(name, suffix)
	}

	// Simple contains
	return strings.Contains(name, query)
}

// adjustScroll adjusts scroll offset to keep selection visible
func (m *Model) adjustScroll() {
	visible := m.visibleRows()
	if m.state.SelectedIndex < m.state.ScrollOffset {
		m.state.ScrollOffset = m.state.SelectedIndex
	}
	if m.state.SelectedIndex >= m.state.ScrollOffset+visible {
		m.state.ScrollOffset = m.state.SelectedIndex - visible + 1
	}
}

// visibleRows returns number of visible rows
func (m *Model) visibleRows() int {
	// Subtract header, footer, and status lines
	return m.state.Height - 6
}

// buildTree builds tree structure from entries
func (m *Model) buildTree() {
	// Implementation in tree.go
	m.state.TreeNodes = buildTreeNodes(m.state.FilteredItems, m.state.ExpandedPaths)
}

type describeLoadedMsg struct {
	value   string
	history []HistoryEntry
}

// loadDescribe loads describe data for a parameter
func (m Model) loadDescribe(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get current value
		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			return errorMsg("Failed to load parameter: " + err.Error())
		}

		// Get history
		history, err := m.client.GetParameterHistory(ctx, name, 3, true)
		if err != nil {
			// Continue without history
			return describeLoadedMsg{
				value:   param.Value,
				history: []HistoryEntry{{Version: param.Version, Value: param.Value, Modified: param.LastModifiedDate.Format(time.RFC3339)}},
			}
		}

		var historyEntries []HistoryEntry
		for _, h := range history {
			historyEntries = append(historyEntries, HistoryEntry{
				Version:  h.Version,
				Value:    h.Value,
				Modified: h.LastModifiedDate.Format(time.RFC3339),
			})
		}

		return describeLoadedMsg{
			value:   param.Value,
			history: historyEntries,
		}
	}
}

// copySecret copies secret value to clipboard
func (m Model) copySecret(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			return errorMsg("Failed to get secret: " + err.Error())
		}

		msg, err := m.clipboard.CopyWithMessage(param.Value)
		if err != nil {
			return errorMsg("Failed to copy: " + err.Error())
		}

		return statusMsg(msg)
	}
}

// copyValue copies a value to clipboard
func (m Model) copyValue(value string) tea.Cmd {
	return func() tea.Msg {
		msg, err := m.clipboard.CopyWithMessage(value)
		if err != nil {
			return errorMsg("Failed to copy: " + err.Error())
		}
		return statusMsg(msg)
	}
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	switch m.state.Mode {
	case ViewModeDescribe:
		return m.renderDescribeView()
	default:
		return m.renderBrowseView()
	}
}
```

### 3. Create Browse Command

Create file `internal/cli/browse.go`:

```go
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

func init() {
	rootCmd.AddCommand(browseCmd)
}

var browseCmd = &cobra.Command{
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

func runBrowse(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// Initialize cache
	cacheMgr, err := cache.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Check if cache needs refresh
	if cacheMgr.IsExpired() {
		fmt.Println("Cache expired, refreshing...")
		err = cacheMgr.Refresh(ctx, client, cfg.Region, cfg.ParallelFetches, nil)
		if err != nil {
			return fmt.Errorf("failed to refresh cache: %w", err)
		}
	}

	// Create and run UI
	model := ui.NewModel(client, cacheMgr, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}
```

## Acceptance Criteria

- [ ] `clerk browse` launches interactive TUI
- [ ] Arrow keys navigate up/down
- [ ] Page up/down move by page
- [ ] Home/End jump to start/end
- [ ] `/` activates search mode
- [ ] Search filters list in real-time
- [ ] Glob patterns work in search (/dev/*)
- [ ] Esc cancels search (preserves results)
- [ ] `d` or Enter opens describe view
- [ ] `c` copies secret to clipboard
- [ ] `t` toggles tree/flat view
- [ ] Space expands/collapses in tree view
- [ ] `q` quits or goes back
- [ ] Status messages appear and auto-clear
- [ ] Window resize is handled

## Notes

- Using `charmbracelet/bubbletea` instead of `promptui` for richer UI
- State is immutable-ish (Elm architecture)
- Commands are returned for async operations
- View is rendered on every update
- Alt screen mode keeps terminal clean on exit
- Cache is auto-refreshed if expired on browse start
