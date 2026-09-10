package ui

import (
	"context"
	"fmt"
	"sort"
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
	client    SSMBrowseClient
	cache     *cache.Manager
	config    *config.Config
	clipboard *util.ClipboardManager
	scope     aws.ResourceIdentity

	searchInput   textinput.Model
	ready         bool
	quitting      bool
	refreshing    bool
	refreshCancel context.CancelFunc
}

// SSMModel names the Parameter Store-specific model while retaining the
// established Model API for existing SSM callers.
type SSMModel = Model

// SSMBrowseClient is the Parameter Store surface used by the browser.
type SSMBrowseClient interface {
	cache.RefreshClient
	GetRegion() string
	GetParameter(context.Context, string, bool) (*aws.Parameter, error)
	GetParameterMetadata(context.Context, string) (*aws.Parameter, error)
	GetParameterByVersion(context.Context, string, int64, bool) (*aws.Parameter, error)
	GetParameterHistory(context.Context, string, int32, bool) ([]aws.ParameterHistory, error)
	DeleteParameter(context.Context, string) error
	Transfer(context.Context, aws.TransferInput) (aws.TransferResult, error)
	PutParameter(context.Context, *aws.PutParameterInput) (*aws.PutParameterOutput, error)
	LabelParameterVersion(context.Context, *aws.LabelParameterInput) (*aws.LabelParameterOutput, error)
	UnlabelParameterVersion(context.Context, *aws.UnlabelParameterInput) error
	AddTagsToResource(context.Context, string, map[string]string) error
	RemoveTagsFromResource(context.Context, string, []string) error
	GetParameterTags(context.Context, string) (map[string]string, error)
}

// NewSSMModel creates a Parameter Store browser.
func NewSSMModel(client SSMBrowseClient, cacheMgr *cache.Manager, cfg *config.Config, scope aws.ResourceIdentity) SSMModel {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Search (glob patterns supported)..."
	ti.CharLimit = 100

	m := Model{
		state: State{
			Mode:          ViewModeList,
			PreviousMode:  ViewModeList,
			ExpandedPaths: make(map[string]bool),
			SortType:      SortByName,
			SortAscending: true,
		},
		client:      client,
		cache:       cacheMgr,
		config:      cfg,
		clipboard:   util.NewClipboardManager(cfg.ClipboardTimeout),
		scope:       scope,
		searchInput: ti,
	}
	return m
}

// NewModel preserves the original SSM constructor for package consumers.
func NewModel(client *aws.Client, cacheMgr *cache.Manager, cfg *config.Config, _ ...aws.Backend) Model {
	scope := aws.ResourceIdentity{Backend: aws.BackendSSM}
	if client != nil {
		scope.Partition, scope.AccountID, scope.Region = client.GetPartition(), client.GetAccountID(), client.GetRegion()
	}
	return NewSSMModel(client, cacheMgr, cfg, scope)
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tea.EnableMouseAllMotion,
		m.loadEntries,
		m.checkBackgroundRefresh,
	)
}

// loadEntries loads entries from cache
func (m Model) loadEntries() tea.Msg {
	entries := m.cachedEntries()
	return entriesLoadedMsg{entries: entries}
}

func (m Model) cachedEntries() []cache.CacheEntry {
	if m.cache == nil {
		return nil
	}
	return m.cache.GetAll()
}

// checkBackgroundRefresh checks if cache should be refreshed in background
func (m Model) checkBackgroundRefresh() tea.Msg {
	// Skip if auto-refresh is disabled
	if !m.config.BrowseAutoRefresh {
		return nil
	}

	// Check if cache is empty (first run)
	if len(m.cachedEntries()) == 0 {
		return backgroundRefreshStartMsg{}
	}

	// The cache tracks never-refreshed and incomplete snapshots explicitly.
	// Do not treat mutation-only entries with a zero refresh time as fresh.
	if m.cache != nil && !m.cache.IsExpired() && m.cache.GetAge() < m.config.BrowseRefreshCooldown {
		// Cache is fresh, no refresh needed
		return nil
	}

	// Cache is stale, trigger background refresh
	return backgroundRefreshStartMsg{}
}

// doBackgroundRefresh performs cache refresh in background with live progress
func (m Model) doBackgroundRefresh(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// This will be replaced by the streaming version
		return startRefreshWithProgress(ctx, m.cache, m.client, m.config)
	}
}

// startRefreshWithProgress starts the refresh and returns a sub for progress updates
func startRefreshWithProgress(ctx context.Context, cacheMgr *cache.Manager, client SSMBrowseClient, cfg *config.Config) tea.Msg {
	// Create a channel for progress
	progressCh := make(chan refreshEvent, 100)

	// Start refresh in background
	go func() {
		progressCallback := func(current, total int) {
			select {
			case progressCh <- refreshEvent{current: current}:
			default:
				// Channel full, skip this update
			}
		}

		// Use the actual region from the client, not config
		err := cacheMgr.Refresh(ctx, client, client.GetRegion(), cfg.ParallelFetches, progressCallback)

		// Signal completion
		progressCh <- refreshEvent{done: true, err: err}
		close(progressCh)
	}()

	// Return a message that starts listening for progress
	return refreshProgressChannelMsg{ch: progressCh}
}

// refreshProgressChannelMsg carries the progress channel
type refreshProgressChannelMsg struct {
	ch chan refreshEvent
}
type refreshEvent struct {
	current int
	done    bool
	err     error
}

// waitForProgress returns a command that waits for the next progress update
func waitForProgress(ch chan refreshEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		if event.done {
			return backgroundRefreshCompleteMsg{loadFromCache: true, err: event.err}
		}
		return backgroundRefreshProgressMsg{current: event.current, ch: ch}
	}
}

type entriesLoadedMsg struct {
	entries []cache.CacheEntry
}

type backgroundRefreshStartMsg struct{}
type backgroundRefreshProgressMsg struct {
	current int
	ch      chan refreshEvent
}
type backgroundRefreshCompleteMsg struct {
	// loadFromCache signals that the handler should re-read entries from
	// the cache manager rather than carrying them on the message.
	loadFromCache bool
	err           error
}

type statusMsg string
type errorMsg string
type clearStatusMsg struct{}
type describeLoadedMsg struct {
	name       string
	identity   aws.ResourceIdentity
	generation uint64
	value      string
	history    []HistoryEntry
}

type versionValuesLoadedMsg struct {
	name       string
	identity   aws.ResourceIdentity
	generation uint64
	versions   map[int64]string // version -> value mapping
}
type resourceErrorMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	message    string
}
type resourceStatusMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	message    string
}
type parameterValueLoadedMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	value      string
	err        error
}

type editCompleteMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	name       string
	newValue   string
	version    int64
	err        error
}

type editPreparedMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	name       string
	original   aws.Parameter
	session    *util.EditorSession
	err        error
}

type deleteCompleteMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	name       string
	err        error
}

type moveCompleteMsg struct {
	identity    aws.ResourceIdentity
	generation  uint64
	source      string
	target      string
	destination *aws.Parameter
	warning     string
	err         error
}

type copyCompleteMsg struct {
	identity    aws.ResourceIdentity
	generation  uint64
	source      string
	target      string
	destination *aws.Parameter
	warning     string
	err         error
}

// Label operation messages
type labelCompleteMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	action     string
	label      string
	version    int64
	err        error
}

type tagCompleteMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	action     string
	key        string
	err        error
}

type tagsRefreshMsg struct {
	name       string
	identity   aws.ResourceIdentity
	generation uint64
	tags       map[string]string
}

type historyRefreshMsg struct {
	name       string
	identity   aws.ResourceIdentity
	generation uint64
	history    []aws.ParameterHistory
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle search input when active (before key processing)
	if m.state.SearchActive {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// Special handling for search mode
			switch keyMsg.String() {
			case "esc":
				m.state.SearchActive = false
				m.state.CurrentSuggestion = ""
				m.state.SuggestionAlternatives = nil
				m.state.SuggestionIndex = -1
				m.searchInput.Blur()
				return m, nil
			case "enter":
				m.state.SearchActive = false
				m.state.CurrentSuggestion = ""
				m.state.SuggestionAlternatives = nil
				m.state.SuggestionIndex = -1
				m.searchInput.Blur()
				return m, nil
			case "tab", "right":
				// Accept current suggestion
				if m.state.CurrentSuggestion != "" {
					m.searchInput.SetValue(m.state.CurrentSuggestion)
					m.searchInput.CursorEnd()
					m.state.SearchQuery = m.state.CurrentSuggestion
					m.updatePathSuggestions()
					m.filterEntries()
				}
				return m, nil
			case "down":
				// Cycle to next alternative
				if len(m.state.SuggestionAlternatives) > 0 {
					m.state.SuggestionIndex++
					if m.state.SuggestionIndex >= len(m.state.SuggestionAlternatives) {
						m.state.SuggestionIndex = 0
					}
					m.state.CurrentSuggestion = m.state.SuggestionAlternatives[m.state.SuggestionIndex]
					return m, nil
				}
				// No suggestions, exit search and navigate down
				m.state.SearchActive = false
				m.searchInput.Blur()
				if m.state.SelectedIndex < len(m.state.FilteredItems)-1 {
					m.state.SelectedIndex++
					m.adjustScroll()
				}
				return m, nil
			case "up":
				// Cycle to previous alternative
				if len(m.state.SuggestionAlternatives) > 0 {
					m.state.SuggestionIndex--
					if m.state.SuggestionIndex < 0 {
						m.state.SuggestionIndex = len(m.state.SuggestionAlternatives) - 1
					}
					m.state.CurrentSuggestion = m.state.SuggestionAlternatives[m.state.SuggestionIndex]
					return m, nil
				}
				// No suggestions, exit search and navigate up
				m.state.SearchActive = false
				m.searchInput.Blur()
				if m.state.SelectedIndex > 0 {
					m.state.SelectedIndex--
					m.adjustScroll()
				}
				return m, nil
			}
			// Pass other keys to search input
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			cmds = append(cmds, cmd)

			// Update filter and suggestions on input change
			if m.state.SearchQuery != m.searchInput.Value() {
				m.state.SearchQuery = m.searchInput.Value()
				m.updatePathSuggestions()
				m.filterEntries()
			}
			return m, tea.Batch(cmds...)
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		// bubbletea v1.x deprecated msg.Type / tea.MouseWheelUp in favor of
		// msg.Button / tea.MouseButtonWheelUp. Wheel events still arrive as
		// MouseActionPress so we don't need to filter on action.
		switch m.state.Mode {
		case ViewModeDescribe:
			// Describe view: scroll value vertically or horizontally
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				// Shift + wheel: horizontal scroll
				if msg.Shift && !m.state.ValueLineWrap {
					if m.state.ValueHorizontalScroll > 0 {
						m.state.ValueHorizontalScroll--
					}
				} else {
					// Normal wheel: vertical scroll
					if m.state.ValueScrollOffset > 0 {
						m.state.ValueScrollOffset--
					}
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				// Shift + wheel: horizontal scroll
				if msg.Shift && !m.state.ValueLineWrap {
					m.state.ValueHorizontalScroll++
				} else {
					// Normal wheel: vertical scroll
					m.state.ValueScrollOffset++
				}
				return m, nil
			}
		case ViewModeList, ViewModeTree:
			// Browse view: scroll through entries
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.state.SelectedIndex > 0 {
					m.state.SelectedIndex--
					m.adjustScroll()
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				maxIndex := len(m.state.FilteredItems) - 1
				if m.state.Mode == ViewModeTree {
					maxIndex = len(m.state.TreeNodes) - 1
				}
				if m.state.SelectedIndex < maxIndex {
					m.state.SelectedIndex++
					m.adjustScroll()
				}
				return m, nil
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.state.Width = msg.Width
		m.state.Height = msg.Height
		m.ready = true
		return m, nil

	case entriesLoadedMsg:
		m.state.Entries = msg.entries
		m.state.CacheAge = m.cacheAge()
		m.filterEntries()
		return m, nil

	case backgroundRefreshStartMsg:
		if m.refreshing {
			return m, nil
		}
		m.refreshing = true
		refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		m.refreshCancel = cancel
		// Show appropriate message based on whether cache is empty
		if len(m.state.Entries) == 0 {
			m.state.StatusMessage = "Loading parameters from AWS..."
		} else {
			m.state.StatusMessage = "Refreshing cache in background..."
		}
		return m, m.doBackgroundRefresh(refreshCtx)

	case refreshProgressChannelMsg:
		// Start listening for progress updates
		return m, waitForProgress(msg.ch)

	case backgroundRefreshProgressMsg:
		// Update status with current count
		if len(m.state.Entries) == 0 {
			m.state.StatusMessage = fmt.Sprintf("Loading parameters from AWS... (%d loaded)", msg.current)
		} else {
			m.state.StatusMessage = fmt.Sprintf("Refreshing cache... (%d loaded)", msg.current)
		}
		// Continue waiting for more progress
		return m, waitForProgress(msg.ch)

	case backgroundRefreshCompleteMsg:
		if m.refreshCancel != nil {
			m.refreshCancel()
			m.refreshCancel = nil
		}
		m.refreshing = false
		if msg.err != nil {
			if len(m.state.Entries) > 0 {
				m.state.StatusMessage = "Refresh failed; showing cached parameters"
			} else {
				m.state.ErrorMessage = "Refresh failed: " + msg.err.Error()
			}
			return m, nil
		}
		// Load entries from cache
		entries := m.cachedEntries()

		if len(entries) == 0 {
			// Refresh failed - user is offline
			m.state.OfflineMode = true
			m.state.StatusMessage = ""
		} else {
			m.state.OfflineMode = false
			m.state.Entries = entries
			m.state.CacheAge = m.cache.GetAge()
			m.filterEntries()
			// Show count of loaded parameters
			count := len(entries)
			m.state.StatusMessage = fmt.Sprintf("Cache refreshed - %d parameters loaded", count)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
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

	case resourceErrorMsg:
		if !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		m.state.ErrorMessage = backendLabel(msg.identity.Backend) + " " + m.state.DescribeParamName + ": " + msg.message
		m.state.StatusMessage = ""
		return m, nil

	case resourceStatusMsg:
		if !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		m.state.StatusMessage = backendLabel(msg.identity.Backend) + " " + m.resourceName(msg.identity) + ": " + msg.message
		m.state.ErrorMessage = ""
		return m, nil

	case clearStatusMsg:
		m.state.StatusMessage = ""
		m.state.ErrorMessage = ""
		return m, nil

	case describeLoadedMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.currentResource(msg.identity, msg.generation) || msg.identity == (aws.ResourceIdentity{}) && (msg.generation != m.state.DescribeGeneration || msg.name != m.state.DescribeParamName || m.state.Mode != ViewModeDescribe) {
			return m, nil
		}
		m.state.DescribeValue = msg.value
		m.state.DescribeHistory = msg.history
		m.state.DescribeValueKind = ""
		if len(msg.history) > 0 && msg.history[0].ValueLoaded {
			m.state.DescribeValueKind = aws.ValueText
		}
		m.state.HistoryIndex = 0
		m.state.DescribeLoading = false
		return m, nil

	case versionValuesLoadedMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.currentResource(msg.identity, msg.generation) || msg.identity == (aws.ResourceIdentity{}) && (msg.generation != m.state.DescribeGeneration || msg.name != m.state.DescribeParamName || m.state.Mode != ViewModeDescribe) {
			return m, nil
		}
		// Update history entries with loaded values
		for i := range m.state.DescribeHistory {
			if value, ok := msg.versions[m.state.DescribeHistory[i].Version]; ok {
				m.state.DescribeHistory[i].Value = value
				m.state.DescribeHistory[i].ValueLoaded = true
				m.state.DescribeHistory[i].ValueKind = aws.ValueText
			}
		}
		// Update current displayed value if it was loaded
		if m.state.HistoryIndex < len(m.state.DescribeHistory) {
			m.state.DescribeValue = m.state.DescribeHistory[m.state.HistoryIndex].Value
			m.state.DescribeValueKind = aws.ValueText
		}
		return m, nil

	case parameterValueLoadedMsg:
		if !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.state.ErrorMessage = "SSM " + m.resourceName(msg.identity) + ": value unavailable: " + msg.err.Error()
			return m, nil
		}
		return m, m.copyResourceValue(msg.identity, msg.generation, msg.value, aws.ValueText)

	case editCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: %v", backendLabel(msg.identity.Backend), m.resourceName(msg.identity), msg.err)
			return m, nil
		}

		// Update cache
		for i := range m.state.Entries {
			if m.state.Entries[i].Identity == msg.identity {
				m.state.Entries[i].Version = msg.version
				m.state.Entries[i].LastModifiedDate = time.Now()
				break
			}
		}
		m.filterEntries()

		// Update cache manager
		if m.cache != nil {
			if entry, ok := m.cache.GetByIdentity(msg.identity); ok {
				entry.Version = msg.version
				entry.LastModifiedDate = time.Now()
				_ = m.cache.Update(*entry)
			}
		}
		if m.state.Mode == ViewModeDescribe && m.state.DescribeIdentity == msg.identity {
			m.state.DescribeValue = msg.newValue
			m.state.DescribeLoading = false
			found := false
			for i := range m.state.DescribeHistory {
				if m.state.DescribeHistory[i].Version == msg.version {
					m.state.DescribeHistory[i].Value, m.state.DescribeHistory[i].ValueLoaded, m.state.HistoryIndex, found = msg.newValue, true, i, true
					break
				}
			}
			if !found {
				m.state.DescribeHistory = append([]HistoryEntry{{Version: msg.version, Value: msg.newValue, ValueLoaded: true, Modified: time.Now().Format(time.RFC3339)}}, m.state.DescribeHistory...)
				m.state.HistoryIndex = 0
			}
		}

		m.state.StatusMessage = fmt.Sprintf("%s %s: updated to version %d", backendLabel(msg.identity.Backend), msg.name, msg.version)
		m.state.ErrorMessage = ""
		return m, nil

	case editPreparedMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.currentResource(msg.identity, msg.generation) {
			if msg.session != nil {
				_ = msg.session.Close()
			}
			return m, nil
		}
		if msg.err != nil {
			return m, func() tea.Msg {
				return resourceErrorMsg{identity: msg.identity, generation: msg.generation, message: msg.err.Error()}
			}
		}
		// tea.ExecProcess releases and restores the terminal around an
		// interactive editor. Saving deliberately uses a fresh deadline after
		// the user returns, rather than consuming the editor's wall time.
		return m, tea.ExecProcess(msg.session.Command(), func(runErr error) tea.Msg {
			defer func() { _ = msg.session.Close() }()
			if runErr != nil {
				return editCompleteMsg{identity: msg.identity, generation: msg.generation, err: fmt.Errorf("editor error: %w", runErr)}
			}
			newValue, err := msg.session.Read()
			if err != nil {
				return editCompleteMsg{identity: msg.identity, generation: msg.generation, err: fmt.Errorf("failed to read edited value: %w", err)}
			}
			if newValue == msg.original.Value {
				return resourceStatusMsg{identity: msg.identity, generation: msg.generation, message: "no changes made"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			current, err := m.client.GetParameter(ctx, msg.name, false)
			if err != nil {
				return editCompleteMsg{identity: msg.identity, generation: msg.generation, err: fmt.Errorf("failed to check current version: %w", err)}
			}
			if current.Version != msg.original.Version {
				return editCompleteMsg{identity: msg.identity, generation: msg.generation, err: fmt.Errorf("parameter changed from version %d to %d while editing; review and retry", msg.original.Version, current.Version)}
			}
			output, err := m.client.PutParameter(ctx, &aws.PutParameterInput{Name: msg.name, Value: newValue, Type: msg.original.Type, Overwrite: true, KMSKeyID: msg.original.KMSKeyID, Description: msg.original.Description, Tier: msg.original.Tier, AllowedPattern: msg.original.AllowedPattern, Policies: msg.original.Policies, DataType: msg.original.DataType})
			if err != nil {
				return editCompleteMsg{identity: msg.identity, generation: msg.generation, err: fmt.Errorf("failed to update: %w", err)}
			}
			return editCompleteMsg{identity: msg.identity, generation: msg.generation, name: msg.name, newValue: newValue, version: output.Version}
		})

	case deleteCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: delete failed: %v", backendLabel(msg.identity.Backend), msg.name, msg.err)
			return m, nil
		}

		// Remove from cache
		if m.cache != nil {
			_ = m.cache.DeleteByIdentity(msg.identity)
		}
		m.state.StatusMessage = fmt.Sprintf("%s %s: deleted", backendLabel(msg.identity.Backend), msg.name)
		m.state.ErrorMessage = ""

		// Reload entries from cache
		return m, m.loadEntries

	case moveCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: move failed: %v", backendLabel(msg.identity.Backend), msg.source, msg.err)
			return m, nil
		}

		// Remove from cache
		if err := m.cache.Delete(msg.source); err != nil {
			msg.warning = "remote move succeeded but cache delete failed: " + err.Error()
		}
		if msg.destination != nil {
			if err := m.cache.Update(cache.CacheEntry{Name: msg.destination.Name, Type: msg.destination.Type, Version: msg.destination.Version, LastModifiedDate: msg.destination.LastModifiedDate, Tags: msg.destination.Tags, TagsComplete: true, TagsFetchedAt: time.Now()}); err != nil {
				msg.warning = "remote move succeeded but cache update failed: " + err.Error()
			}
		}

		// Reload entries from cache
		status := fmt.Sprintf("Moved %s to %s", msg.source, msg.target)
		if msg.warning != "" {
			status += " (remote write succeeded; cache refresh needed)"
		}
		m.state.StatusMessage = fmt.Sprintf("%s %s: %s", backendLabel(msg.identity.Backend), msg.source, status)
		m.state.ErrorMessage = ""
		return m, m.loadEntries

	case copyCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: copy failed: %v", backendLabel(msg.identity.Backend), msg.source, msg.err)
			return m, nil
		}

		if msg.destination != nil {
			if err := m.cache.Update(cache.CacheEntry{Name: msg.destination.Name, Type: msg.destination.Type, Version: msg.destination.Version, LastModifiedDate: msg.destination.LastModifiedDate, Tags: msg.destination.Tags, TagsComplete: true, TagsFetchedAt: time.Now()}); err != nil {
				msg.warning = "remote copy succeeded but cache update failed: " + err.Error()
			}
		}
		// Reload entries from cache
		status := fmt.Sprintf("Copied %s to %s", msg.source, msg.target)
		if msg.warning != "" {
			status += " (remote write succeeded; cache refresh needed)"
		}
		m.state.StatusMessage = fmt.Sprintf("%s %s: %s", backendLabel(msg.identity.Backend), msg.source, status)
		m.state.ErrorMessage = ""
		return m, m.loadEntries

	case labelCompleteMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: label %s failed: %v", backendLabel(msg.identity.Backend), m.resourceName(msg.identity), msg.action, msg.err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Refresh history to show updated labels
		m.state.StatusMessage = fmt.Sprintf("%s %s: label '%s' %sed on v%d", backendLabel(msg.identity.Backend), m.resourceName(msg.identity), msg.label, msg.action, msg.version)

		// Trigger history refresh
		return m, m.refreshHistory()

	case tagCompleteMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("%s %s: tag %s failed: %v", backendLabel(msg.identity.Backend), m.resourceName(msg.identity), msg.action, msg.err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Refresh tags on the entry
		m.state.StatusMessage = fmt.Sprintf("%s %s: tag '%s' %sed", backendLabel(msg.identity.Backend), m.resourceName(msg.identity), msg.key, msg.action)
		return m, tea.Batch(
			m.refreshTags(),
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			}),
		)

	case tagsRefreshMsg:
		if m.currentResource(msg.identity, msg.generation) && m.state.DescribeEntry != nil {
			m.state.DescribeEntry.Tags = msg.tags
			for i := range m.state.Entries {
				if m.state.Entries[i].Identity == msg.identity {
					m.state.Entries[i].Tags = msg.tags
				}
			}
			if m.cache != nil {
				if entry, ok := m.cache.GetByIdentity(msg.identity); ok {
					entry.Tags, entry.TagsComplete, entry.TagsError, entry.TagsFetchedAt = msg.tags, true, "", time.Now()
					if err := m.cache.Update(*entry); err != nil {
						m.state.ErrorMessage = "Tags updated remotely but cache update failed: " + err.Error()
					}
				}
			}
			m.filterEntries()
		}
		return m, nil

	case historyRefreshMsg:
		if !m.currentResource(msg.identity, msg.generation) {
			return m, nil
		}
		// Save the version the user was viewing before refresh
		var previousVersion int64
		if m.state.HistoryIndex >= 0 && m.state.HistoryIndex < len(m.state.DescribeHistory) {
			previousVersion = m.state.DescribeHistory[m.state.HistoryIndex].Version
		}

		// Preserve previously loaded (decrypted) values
		oldValues := make(map[int64]HistoryEntry, len(m.state.DescribeHistory))
		for _, h := range m.state.DescribeHistory {
			if h.ValueLoaded {
				oldValues[h.Version] = h
			}
		}

		newHistory := convertHistory(msg.history)

		// Reverse to show newest first (matching loadDescribe behavior)
		for i, j := 0, len(newHistory)-1; i < j; i, j = i+1, j-1 {
			newHistory[i], newHistory[j] = newHistory[j], newHistory[i]
		}

		// Merge old decrypted values into new history
		for i := range newHistory {
			if old, ok := oldValues[newHistory[i].Version]; ok {
				newHistory[i].Value = old.Value
				newHistory[i].ValueLoaded = old.ValueLoaded
			}
		}

		m.state.DescribeHistory = newHistory

		// Preserve the user's selected version position
		if len(newHistory) > 0 {
			found := false
			for i, h := range newHistory {
				if h.Version == previousVersion {
					m.state.HistoryIndex = i
					found = true
					break
				}
			}
			if !found && m.state.HistoryIndex >= len(newHistory) {
				m.state.HistoryIndex = len(newHistory) - 1
			}
		} else {
			m.state.HistoryIndex = 0
		}

		// Adjust scroll offset to keep selection visible
		if m.state.HistoryIndex < m.state.HistoryScrollOffset {
			m.state.HistoryScrollOffset = m.state.HistoryIndex
		}

		// Update displayed value to match current selection
		if m.state.HistoryIndex >= 0 && m.state.HistoryIndex < len(m.state.DescribeHistory) {
			if m.state.DescribeHistory[m.state.HistoryIndex].ValueLoaded {
				m.state.DescribeValue = m.state.DescribeHistory[m.state.HistoryIndex].Value
			}
		}

		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}

	return m, tea.Batch(cmds...)
}

func (m Model) cacheAge() time.Duration {
	if m.cache == nil {
		return 0
	}
	return m.cache.GetAge()
}

func (m Model) currentResource(identity aws.ResourceIdentity, generation uint64) bool {
	if generation != m.state.DescribeGeneration {
		return false
	}
	if m.state.Mode == ViewModeDescribe {
		return identity == m.state.DescribeIdentity
	}
	entry := m.getSelectedEntry()
	return entry != nil && entry.Identity == identity
}

func (m Model) resourceName(identity aws.ResourceIdentity) string {
	if identity == m.state.DescribeIdentity && m.state.DescribeParamName != "" {
		return m.state.DescribeParamName
	}
	for _, entry := range m.state.Entries {
		if entry.Identity == identity {
			return entry.Name
		}
	}
	return identity.CanonicalID
}

func backendLabel(backend aws.Backend) string {
	return "SSM"
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle confirmation dialog first
	if m.state.Confirm.Active {
		return m.handleConfirmKeys(msg)
	}

	// Global keys
	switch msg.String() {
	case "ctrl+c", "q":
		if m.state.SearchActive {
			m.state.SearchActive = false
			m.searchInput.Blur()
			return m, nil
		}
		if m.state.Mode == ViewModeDescribe {
			m.state.DescribeGeneration++
			m.state.DescribeValue = ""
			m.state.DescribeHistory = nil
			m.state.DescribeEntry = nil
			m.state.DescribeIdentity = aws.ResourceIdentity{}
			m.state.Mode = m.state.PreviousMode
			return m, nil
		}
		if m.refreshCancel != nil {
			m.refreshCancel()
			m.refreshCancel = nil
		}
		if m.clipboard != nil {
			_ = m.clipboard.Close()
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
	switch msg.String() {
	case "/":
		m.state.SearchActive = true
		m.state.CurrentSuggestion = ""
		m.state.SuggestionAlternatives = nil
		m.state.SuggestionIndex = -1
		if m.config.SearchSlashPrefix {
			m.searchInput.SetValue("/")
			m.state.SearchQuery = "/"
			m.updatePathSuggestions()
		}
		m.searchInput.Focus()
		return m, textinput.Blink

	case "up", "k":
		if m.state.SelectedIndex > 0 {
			m.state.SelectedIndex--
			m.adjustScroll()
		}
		return m, nil

	case "down", "j":
		if m.state.SelectedIndex < m.visibleItemCount()-1 {
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
		if m.state.SelectedIndex >= m.visibleItemCount() {
			m.state.SelectedIndex = m.visibleItemCount() - 1
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
		m.state.SelectedIndex = m.visibleItemCount() - 1
		if m.state.SelectedIndex < 0 {
			m.state.SelectedIndex = 0
		}
		m.adjustScroll()
		return m, nil

	case "t":
		// Toggle tree/list view
		switch m.state.Mode {
		case ViewModeList:
			m.state.Mode = ViewModeTree
			m.buildTree()
		case ViewModeTree:
			m.state.Mode = ViewModeList
		}
		// Update PreviousMode when in browse mode (not describe)
		m.state.PreviousMode = m.state.Mode
		return m, nil

	case "r":
		// Manual refresh cache
		return m, func() tea.Msg { return backgroundRefreshStartMsg{} }

	case "s":
		// Cycle through sort options: name -> modified -> version -> name
		switch m.state.SortType {
		case SortByName:
			m.state.SortType = SortByModified
			m.state.SortAscending = false // modified defaults newest-first
		case SortByModified:
			m.state.SortType = SortByVersion
			m.state.SortAscending = false // version defaults highest-first
		case SortByVersion:
			m.state.SortType = SortByName
			m.state.SortAscending = true // name defaults A-Z
		}
		m.sortEntries()
		return m, nil

	case "S":
		// Toggle sort direction
		m.state.SortAscending = !m.state.SortAscending
		m.sortEntries()
		return m, nil

	case "f":
		// Cycle through type filters: All -> SecureString -> String -> StringList -> All
		switch m.state.FilterType {
		case FilterAll:
			m.state.FilterType = FilterSecureString
		case FilterSecureString:
			m.state.FilterType = FilterString
		case FilterString:
			m.state.FilterType = FilterStringList
		case FilterStringList:
			m.state.FilterType = FilterAll
		}
		m.filterEntries()
		return m, nil

	case " ":
		// Toggle expand/collapse in tree view
		if m.state.Mode == ViewModeTree && len(m.state.TreeNodes) > 0 {
			if m.state.SelectedIndex < len(m.state.TreeNodes) {
				node := &m.state.TreeNodes[m.state.SelectedIndex]
				if node.IsDir {
					m.state.ExpandedPaths[node.Path] = !m.state.ExpandedPaths[node.Path]
					m.buildTree()
				}
			}
		}
		return m, nil

	case "d", "enter":
		// Describe selected item
		entry := m.getSelectedEntry()
		if entry != nil {
			// Trim whitespace from parameter name in case of encoding issues
			paramName := strings.TrimSpace(entry.Name)
			if paramName == "" {
				identity, generation := entry.Identity, m.state.DescribeGeneration
				return m, func() tea.Msg {
					return resourceErrorMsg{identity: identity, generation: generation, message: "invalid resource name"}
				}
			}
			selected := *entry
			m.state.DescribeEntry = &selected
			m.state.DescribeGeneration++
			m.state.DescribeParamName = paramName
			m.state.DescribeIdentity = entry.Identity
			m.state.DescribeLoading = true
			m.state.DescribeValue = ""
			m.state.DescribeValueKind = ""
			m.state.DescribeValueVersionID = ""
			m.state.DescribeValueError = ""
			m.state.DescribeSelectionChanged = false
			m.state.DescribeHistory = nil
			m.state.HistoryIndex = 0
			m.state.HistoryScrollOffset = 0
			m.state.ValueScrollOffset = 0
			m.state.ValueHorizontalScroll = 0
			m.state.PreviousMode = m.state.Mode // Store current mode to restore later
			m.state.Mode = ViewModeDescribe
			// Opening detail is metadata-only for both providers. Values require an
			// explicit reveal or copy action.
			m.state.DescribeMasked = true
			return m, m.loadDescribe(entry.Identity, paramName, m.state.DescribeGeneration)
		}
		return m, nil

	case "c":
		// Copy secret value
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.copySecret(entry.Identity, entry.Name)
		}
		return m, nil

	case "e":
		// Edit
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.editSecret(entry.Identity, entry.Name)
		}
		return m, nil

	case "delete":
		// Delete (requires confirmation)
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.initiateDelete(entry.Identity, entry.Name)
		}
		return m, nil

	case "m":
		// Move/rename
		entry := m.getSelectedEntry()
		if entry != nil {
			m.state.Confirm = ConfirmState{
				Active:   true,
				Action:   "move",
				Target:   entry.Name,
				Identity: entry.Identity,
			}
		}
		return m, nil

	case "p":
		// Copy
		entry := m.getSelectedEntry()
		if entry != nil {
			m.state.Confirm = ConfirmState{
				Active:   true,
				Action:   "copy",
				Target:   entry.Name,
				Identity: entry.Identity,
			}
		}
		return m, nil
	}

	return m, nil
}

// getSelectedEntry returns the selected entry based on current view mode
func (m *Model) getSelectedEntry() *cache.CacheEntry {
	if m.state.Mode == ViewModeTree {
		// In tree view, get from TreeNodes
		if m.state.SelectedIndex >= 0 && m.state.SelectedIndex < len(m.state.TreeNodes) {
			node := m.state.TreeNodes[m.state.SelectedIndex]
			// Only return entry if it's not a directory
			if !node.IsDir && node.Entry != nil {
				return node.Entry
			}
		}
		return nil
	}

	// In list view, get from FilteredItems
	if m.state.SelectedIndex >= 0 && m.state.SelectedIndex < len(m.state.FilteredItems) {
		entry := m.state.FilteredItems[m.state.SelectedIndex]
		return &entry
	}
	return nil
}

func (m *Model) visibleItemCount() int {
	if m.state.Mode == ViewModeTree {
		return len(m.state.TreeNodes)
	}
	return len(m.state.FilteredItems)
}

// handleDescribeKeys handles keys in describe view
func (m Model) handleDescribeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle label input mode first
	if m.state.LabelInputActive {
		return m.handleLabelInput(msg)
	}
	// Handle tag input mode
	if m.state.TagInputActive {
		return m.handleTagInput(msg)
	}

	switch msg.String() {
	case "esc", "q":
		m.state.DescribeGeneration++
		m.state.DescribeLoading = false
		m.state.DescribeParamName = ""
		m.state.DescribeValue = ""
		m.state.DescribeValueVersionID = ""
		m.state.DescribeHistory = nil
		m.state.DescribeEntry = nil
		m.state.DescribeIdentity = aws.ResourceIdentity{}
		m.state.Mode = m.state.PreviousMode
		// Reset scroll offsets
		m.state.HistoryScrollOffset = 0
		m.state.ValueScrollOffset = 0
		m.state.ValueHorizontalScroll = 0
		return m, nil

	case "x":
		// Toggle masked/unmasked
		m.state.DescribeMasked = !m.state.DescribeMasked
		if !m.state.DescribeMasked && m.state.DescribeValue == "" {
			return m.updateSelectedVersion()
		}
		return m, nil

	case "w":
		// Toggle line wrapping
		m.state.ValueLineWrap = !m.state.ValueLineWrap
		m.state.ValueHorizontalScroll = 0 // Reset horizontal scroll when toggling wrap
		return m, nil

	case "c":
		// Copy value
		if !m.state.DescribeLoading && m.state.DescribeValueKind != "" {
			kind := m.state.DescribeValueKind
			if kind == "" {
				kind = aws.ValueText
			}
			return m, m.copyResourceValue(m.state.DescribeIdentity, m.state.DescribeGeneration, m.state.DescribeValue, kind)
		}
		return m, nil

	case "C":
		// Copy parameter name/path
		if !m.state.DescribeLoading && m.state.DescribeParamName != "" {
			return m, m.copyResourceValue(m.state.DescribeIdentity, m.state.DescribeGeneration, m.state.DescribeParamName, aws.ValueText)
		}
		return m, nil

	case "e":
		// Edit parameter
		if !m.state.DescribeLoading && m.state.DescribeParamName != "" {
			return m, m.editSecret(m.state.DescribeIdentity, m.state.DescribeParamName)
		}
		return m, nil

	case "tab":
		if len(m.state.DescribeHistory) == 0 {
			return m, nil
		}
		// Navigate to older version (increase index), loop to beginning
		if m.state.HistoryIndex < len(m.state.DescribeHistory)-1 {
			m.state.HistoryIndex++
		} else {
			m.state.HistoryIndex = 0
		}
		// Update value and trigger lazy load if needed
		return m.updateSelectedVersion()

	case "shift+tab":
		if len(m.state.DescribeHistory) == 0 {
			return m, nil
		}
		// Navigate to newer version (decrease index), loop to end
		if m.state.HistoryIndex > 0 {
			m.state.HistoryIndex--
		} else {
			m.state.HistoryIndex = len(m.state.DescribeHistory) - 1
		}
		// Update value and trigger lazy load if needed
		return m.updateSelectedVersion()

	case "g":
		// Jump to latest version (go to latest)
		if m.state.HistoryIndex != 0 {
			m.state.HistoryIndex = 0
			// Update value and trigger lazy load if needed
			return m.updateSelectedVersion()
		}
		return m, nil

	case "up":
		// Scroll value up
		if m.state.ValueScrollOffset > 0 {
			m.state.ValueScrollOffset--
		}
		return m, nil

	case "down":
		// Scroll value down
		m.state.ValueScrollOffset++
		return m, nil

	case "left":
		// Scroll horizontally left
		if !m.state.ValueLineWrap && m.state.ValueHorizontalScroll > 0 {
			m.state.ValueHorizontalScroll--
		}
		return m, nil

	case "right":
		// Scroll horizontally right
		if !m.state.ValueLineWrap {
			m.state.ValueHorizontalScroll++
		}
		return m, nil

	case "a":
		// Add label to current version
		if len(m.state.DescribeHistory) > 0 {
			m.state.LabelInputActive = true
			m.state.LabelAction = "add"
			m.state.LabelInput = ""
			m.state.LabelError = ""
			m.state.LabelSuggestions = util.SuggestLabels()
			m.state.LabelSuggestionIndex = -1
		}
		return m, nil

	case "r":
		// Remove label from current version
		if len(m.state.DescribeHistory) > 0 && m.state.HistoryIndex < len(m.state.DescribeHistory) {
			entry := m.state.DescribeHistory[m.state.HistoryIndex]
			if len(entry.Labels) > 0 {
				m.state.LabelInputActive = true
				m.state.LabelAction = "remove"
				m.state.LabelInput = ""
				m.state.LabelError = ""
				m.state.LabelSuggestions = entry.Labels // Show only labels on this version
				m.state.LabelSuggestionIndex = 0        // Pre-select first
			} else {
				m.state.ErrorMessage = "No labels on this version"
			}
		}
		return m, nil

	case "m":
		// Move label to current version
		if len(m.state.DescribeHistory) > 0 {
			// Collect all unique labels from all versions
			seen := make(map[string]bool)
			var allLabels []string
			for _, h := range m.state.DescribeHistory {
				for _, l := range h.Labels {
					if !seen[l] {
						allLabels = append(allLabels, l)
						seen[l] = true
					}
				}
			}
			if len(allLabels) > 0 {
				m.state.LabelInputActive = true
				m.state.LabelAction = "move"
				m.state.LabelInput = ""
				m.state.LabelError = ""
				m.state.LabelSuggestions = allLabels
				m.state.LabelSuggestionIndex = 0
			} else {
				m.state.ErrorMessage = "No labels to move"
			}
		}
		return m, nil

	case "T":
		// Add tag to parameter
		if m.state.DescribeEntry != nil {
			m.state.TagInputActive = true
			m.state.TagAction = "add"
			m.state.TagInput = ""
			m.state.TagError = ""
			m.state.TagSuggestions = nil
			m.state.TagSuggestionIndex = -1
		}
		return m, nil

	case "D":
		// Remove tag from parameter
		if m.state.DescribeEntry != nil && len(m.state.DescribeEntry.Tags) > 0 {
			var keys []string
			for k := range m.state.DescribeEntry.Tags {
				keys = append(keys, k)
			}
			m.state.TagInputActive = true
			m.state.TagAction = "remove"
			m.state.TagInput = ""
			m.state.TagError = ""
			m.state.TagSuggestions = keys
			m.state.TagSuggestionIndex = 0
		} else {
			m.state.ErrorMessage = "No tags on this parameter"
		}
		return m, nil

	case "pgup":
		// Scroll value up by page
		m.state.ValueScrollOffset -= 10
		if m.state.ValueScrollOffset < 0 {
			m.state.ValueScrollOffset = 0
		}
		return m, nil

	case "pgdown":
		// Scroll value down by page
		m.state.ValueScrollOffset += 10
		return m, nil
	}

	return m, nil
}

// updateSelectedVersion updates the displayed value and triggers lazy loading if needed
func (m Model) updateSelectedVersion() (tea.Model, tea.Cmd) {
	if m.state.HistoryIndex < 0 || m.state.HistoryIndex >= len(m.state.DescribeHistory) {
		return m, nil
	}

	entry := &m.state.DescribeHistory[m.state.HistoryIndex]

	// Adjust scroll to keep selection visible
	if m.state.HistoryIndex < m.state.HistoryScrollOffset {
		m.state.HistoryScrollOffset = m.state.HistoryIndex
	}
	maxVisible := 10 // Should match the rendering logic
	if m.state.HistoryIndex >= m.state.HistoryScrollOffset+maxVisible {
		m.state.HistoryScrollOffset = m.state.HistoryIndex - maxVisible + 1
	}

	// Reset value scroll when changing versions
	m.state.ValueScrollOffset = 0

	if entry.ValueLoaded {
		// Value already loaded, just update display
		m.state.DescribeValue = entry.Value
		m.state.DescribeValueKind = entry.ValueKind
		m.state.DescribeValueVersionID = entry.VersionID
		if m.state.DescribeValueKind == "" {
			m.state.DescribeValueKind = aws.ValueText
		}
		return m, nil
	}
	// Value not loaded, show loading message and trigger fetch
	m.state.DescribeValue = "Loading..."

	// Determine which versions need to be loaded
	batchSize := m.config.DescribeVersionBatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	// Load a batch starting from current index
	var versionsToLoad []int64
	for i := m.state.HistoryIndex; i < len(m.state.DescribeHistory) && len(versionsToLoad) < batchSize; i++ {
		if !m.state.DescribeHistory[i].ValueLoaded {
			versionsToLoad = append(versionsToLoad, m.state.DescribeHistory[i].Version)
		}
	}

	if len(versionsToLoad) > 0 {
		return m, m.loadVersionValues(m.state.DescribeParamName, versionsToLoad)
	}

	return m, nil
}

// updatePathSuggestions updates path-based suggestions for autocomplete
func (m *Model) updatePathSuggestions() {
	query := m.state.SearchQuery

	// Clear suggestions if query is empty
	if query == "" {
		m.state.CurrentSuggestion = ""
		m.state.SuggestionAlternatives = nil
		m.state.SuggestionIndex = -1
		return
	}

	// Find all unique path segments that start with the query
	segmentMap := make(map[string]bool)

	for _, e := range m.state.Entries {
		// Check if this entry starts with the query
		if strings.HasPrefix(e.Name, query) {
			// Find the next segment after the query
			remaining := e.Name[len(query):]

			// If query ends with /, find next segment
			// If query doesn't end with /, complete current segment up to next /
			var nextSegment string
			if strings.HasSuffix(query, "/") {
				// Find next slash
				slashIdx := strings.Index(remaining, "/")
				if slashIdx > 0 {
					nextSegment = query + remaining[:slashIdx+1]
				} else if remaining != "" {
					// No more slashes, this is the final segment
					nextSegment = e.Name
				}
			} else {
				// Complete current segment
				slashIdx := strings.Index(remaining, "/")
				if slashIdx >= 0 {
					nextSegment = query + remaining[:slashIdx+1]
				} else {
					// No slash found, suggest the full name
					nextSegment = e.Name
				}
			}

			if nextSegment != "" && nextSegment != query {
				segmentMap[nextSegment] = true
			}
		}
	}

	// Convert map to sorted slice
	var alternatives []string
	for segment := range segmentMap {
		alternatives = append(alternatives, segment)
	}

	// Sort alternatives alphabetically
	if len(alternatives) > 1 {
		for i := 0; i < len(alternatives)-1; i++ {
			for j := i + 1; j < len(alternatives); j++ {
				if alternatives[j] < alternatives[i] {
					alternatives[i], alternatives[j] = alternatives[j], alternatives[i]
				}
			}
		}
	}

	m.state.SuggestionAlternatives = alternatives

	if len(alternatives) > 0 {
		// Set to first alternative or maintain current index if valid
		if m.state.SuggestionIndex < 0 || m.state.SuggestionIndex >= len(alternatives) {
			m.state.SuggestionIndex = 0
		}
		m.state.CurrentSuggestion = alternatives[m.state.SuggestionIndex]
	} else {
		m.state.CurrentSuggestion = ""
		m.state.SuggestionIndex = -1
	}
}

// filterEntries filters and sorts entries based on search query, type filter, and sort order
func (m *Model) filterEntries() {
	selectedIdentity := aws.ResourceIdentity{}
	if entry := m.getSelectedEntry(); entry != nil {
		selectedIdentity = entry.Identity
	}
	var filtered []cache.CacheEntry
	for _, e := range m.state.Entries {
		// Apply search filter
		if m.state.SearchQuery != "" && !matchSearch(m.state.SearchQuery, e.Name) {
			continue
		}
		// Apply type filter
		if m.state.FilterType != FilterAll && e.Type != m.state.FilterType.String() {
			continue
		}
		filtered = append(filtered, e)
	}
	if m.state.SearchQuery == "" && m.state.FilterType == FilterAll {
		m.state.FilteredItems = m.state.Entries
	} else {
		m.state.FilteredItems = filtered
		m.state.ScrollOffset = 0 // Reset scroll to top when filtering
	}

	// Apply sorting
	m.sortEntries()
	m.buildTree()
	if selectedIdentity != (aws.ResourceIdentity{}) {
		if m.state.Mode == ViewModeTree {
			for i, n := range m.state.TreeNodes {
				if !n.IsDir && n.Identity == selectedIdentity {
					m.state.SelectedIndex = i
					break
				}
			}
		} else {
			for i := range m.state.FilteredItems {
				if m.state.FilteredItems[i].Identity == selectedIdentity {
					m.state.SelectedIndex = i
					break
				}
			}
		}
	}

	// Reset selection if out of bounds
	if m.state.SelectedIndex >= len(m.state.FilteredItems) {
		m.state.SelectedIndex = len(m.state.FilteredItems) - 1
	}
	if m.state.SelectedIndex < 0 {
		m.state.SelectedIndex = 0
	}
	m.adjustScroll()
}

// sortEntries sorts FilteredItems based on the current sort type and direction
func (m *Model) sortEntries() {
	if len(m.state.FilteredItems) == 0 {
		m.buildTree()
		return
	}

	asc := m.state.SortAscending

	switch m.state.SortType {
	case SortByName:
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				left, right := m.state.FilteredItems[j], m.state.FilteredItems[i]
				less := left.Name < right.Name || left.Name == right.Name && left.Identity.Backend < right.Identity.Backend
				if asc == less {
					m.state.FilteredItems[i], m.state.FilteredItems[j] = m.state.FilteredItems[j], m.state.FilteredItems[i]
				}
			}
		}
	case SortByModified:
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				less := m.state.FilteredItems[j].LastModifiedDate.Before(m.state.FilteredItems[i].LastModifiedDate)
				if asc == less {
					m.state.FilteredItems[i], m.state.FilteredItems[j] = m.state.FilteredItems[j], m.state.FilteredItems[i]
				}
			}
		}
	case SortByVersion:
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				less := m.state.FilteredItems[j].Version < m.state.FilteredItems[i].Version
				if asc == less {
					m.state.FilteredItems[i], m.state.FilteredItems[j] = m.state.FilteredItems[j], m.state.FilteredItems[i]
				}
			}
		}
	}
	if m.state.Mode == ViewModeTree {
		m.buildTree()
	}
}

// getSortLabel returns a human-readable label for the current sort type
func (m *Model) getSortLabel() string {
	switch m.state.SortType {
	case SortByName:
		return "name"
	case SortByModified:
		return "modified"
	case SortByVersion:
		return "version"
	default:
		return "unknown"
	}
}

// sortIndicator returns ▲ or ▼ for the active sort column, empty for others
func (m Model) sortIndicator(col SortType) string {
	if m.state.SortType != col {
		return ""
	}
	if m.state.SortAscending {
		return " ▲"
	}
	return " ▼"
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
	count := len(m.state.FilteredItems)
	if m.state.Mode == ViewModeTree {
		count = len(m.state.TreeNodes)
	}
	if count == 0 {
		m.state.SelectedIndex, m.state.ScrollOffset = 0, 0
		return
	}
	if m.state.SelectedIndex < 0 {
		m.state.SelectedIndex = 0
	}
	if m.state.SelectedIndex >= count {
		m.state.SelectedIndex = count - 1
	}
	if m.state.ScrollOffset < 0 {
		m.state.ScrollOffset = 0
	}
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
	// Subtract: title(1) + search(1) + header(1) + separator(1) + footer(1) + status(1) + help(1) = 7 lines
	rows := m.state.Height - 7
	if rows < 5 {
		rows = 5
	}
	return rows
}

// buildTree builds tree structure from entries
func (m *Model) buildTree() {
	m.state.TreeNodes = buildTreeNodes(m.state.FilteredItems, m.state.ExpandedPaths)
	m.adjustScroll()
}

// loadDescribe loads describe data for a parameter
func (m Model) loadDescribe(identity aws.ResourceIdentity, name string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Validate parameter name
		if name == "" {
			return resourceErrorMsg{identity: identity, generation: generation, message: "invalid resource name"}
		}
		// Parameter detail is metadata-only. Values are loaded by reveal/copy.
		param, err := m.client.GetParameterMetadata(ctx, name)
		if err != nil {
			// Check if it's an auth/network error
			errMsg := err.Error()
			if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "credential") ||
				strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "network") {
				return resourceErrorMsg{identity: identity, generation: generation, message: "unable to retrieve value; check AWS credentials and network connection"}
			}
			return resourceErrorMsg{identity: identity, generation: generation, message: "failed to load parameter: " + err.Error()}
		}

		var historyEntries []HistoryEntry

		allVersions, err := m.client.GetParameterHistory(ctx, name, 50, false)
		if err != nil {
			return describeLoadedMsg{name: name, identity: identity, generation: generation, history: []HistoryEntry{{Version: param.Version, Modified: param.LastModifiedDate.Format(time.RFC3339)}}}
		}
		for _, h := range allVersions {
			historyEntries = append(historyEntries, HistoryEntry{Version: h.Version, Modified: h.LastModifiedDate.Format(time.RFC3339), Labels: h.Labels})
		}

		sort.Slice(historyEntries, func(i, j int) bool { return historyEntries[i].Version > historyEntries[j].Version })
		return describeLoadedMsg{name: name, identity: identity, generation: generation,
			history: historyEntries,
		}
	}
}

// copySecret copies secret value to clipboard
func (m Model) copySecret(identity aws.ResourceIdentity, name string) tea.Cmd {
	generation := m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			// Check if it's an auth/network error
			errMsg := err.Error()
			if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "credential") ||
				strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "network") {
				return parameterValueLoadedMsg{identity: identity, generation: generation, err: fmt.Errorf("unable to retrieve value; check AWS credentials and network connection")}
			}
			return parameterValueLoadedMsg{identity: identity, generation: generation, err: err}
		}
		return parameterValueLoadedMsg{identity: identity, generation: generation, value: param.Value}
	}
}

func (m Model) copyResourceValue(identity aws.ResourceIdentity, generation uint64, value string, kind aws.ValueKind) tea.Cmd {
	return func() tea.Msg {
		message, err := m.clipboard.CopyWithMessage(value)
		if err != nil {
			return resourceErrorMsg{identity: identity, generation: generation, message: "copy failed: " + err.Error()}
		}
		return resourceStatusMsg{identity: identity, generation: generation, message: fmt.Sprintf("copied %s value (%s)", kind, message)}
	}
}

// editSecret opens an editor to edit the parameter
func (m Model) editSecret(identity aws.ResourceIdentity, name string) tea.Cmd {
	generation := m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get current value
		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			return editPreparedMsg{identity: identity, generation: generation, err: fmt.Errorf("failed to get parameter: %w", err)}
		}
		metadata, err := m.client.GetParameterMetadata(ctx, param.Name)
		if err != nil {
			return editPreparedMsg{identity: identity, generation: generation, err: fmt.Errorf("failed to read parameter protection metadata: %w", err)}
		}
		param.KMSKeyID, param.Description, param.Tier = metadata.KMSKeyID, metadata.Description, metadata.Tier
		param.AllowedPattern, param.Policies, param.DataType = metadata.AllowedPattern, metadata.Policies, metadata.DataType

		// Determine file extension based on content
		ext := ".txt"
		trimmedVal := strings.TrimSpace(param.Value)
		if strings.HasPrefix(trimmedVal, "{") {
			ext = ".json"
		} else if strings.HasPrefix(trimmedVal, "<") {
			ext = ".xml"
		}

		// Prepare a secure file. Update executes its process through Bubble Tea.
		editor := util.NewEditor(util.EditorConfig{})
		session, err := editor.Prepare(param.Value, ext)
		if err != nil {
			return editPreparedMsg{identity: identity, generation: generation, err: fmt.Errorf("editor error: %w", err)}
		}
		return editPreparedMsg{identity: identity, generation: generation, name: name, original: *param, session: session}
	}
}

// initiateDelete starts the delete confirmation flow
func (m *Model) initiateDelete(identity aws.ResourceIdentity, name string) tea.Cmd {
	m.state.Confirm = ConfirmState{
		Active:      true,
		Action:      "delete",
		Target:      name,
		Identity:    identity,
		ConfirmText: "delete me",
	}

	return nil
}

// deleteSecret performs the actual deletion
func (m Model) deleteSecret(identity aws.ResourceIdentity, name string) tea.Cmd {
	generation := m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := m.client.DeleteParameter(ctx, name)
		if err != nil {
			return deleteCompleteMsg{identity: identity, generation: generation, name: name, err: err}
		}

		return deleteCompleteMsg{identity: identity, generation: generation, name: name}
	}
}

// moveSecret moves/renames a parameter
func (m Model) moveSecret(identity aws.ResourceIdentity, source, target string) tea.Cmd {
	generation := m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := m.client.Transfer(ctx, aws.TransferInput{Source: source, Destination: target, Move: true})
		msg := moveCompleteMsg{identity: identity, generation: generation, source: source, target: target, destination: result.Destination, err: err}
		if result.DestinationWritten && !result.SourceDeleted {
			msg.warning = "destination was created; source was retained: " + err.Error()
			msg.err = nil
		}
		return msg
	}
}

// copySecretAs copies a parameter to a new name
func (m Model) copySecretAs(identity aws.ResourceIdentity, source, target string) tea.Cmd {
	generation := m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := m.client.Transfer(ctx, aws.TransferInput{Source: source, Destination: target})
		return copyCompleteMsg{identity: identity, generation: generation, source: source, target: target, destination: result.Destination, err: err}
	}
}

// handleConfirmKeys handles keyboard input during confirmation
func (m Model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state.Confirm = ConfirmState{}
		return m, nil

	case "enter":
		// Handle different confirmation types
		switch m.state.Confirm.Action {
		case "delete":
			// For delete, check confirmation text
			if m.state.Confirm.Input == m.state.Confirm.ConfirmText {
				name, identity := m.state.Confirm.Target, m.state.Confirm.Identity
				m.state.Confirm = ConfirmState{}
				return m, m.deleteSecret(identity, name)
			}
			m.state.Confirm.ErrorMsg = "Incorrect confirmation text"
			return m, nil
		case "move":
			// For move, target is the new name
			if m.state.Confirm.Input == "" {
				m.state.Confirm.ErrorMsg = "Target name cannot be empty"
				return m, nil
			}
			source, identity := m.state.Confirm.Target, m.state.Confirm.Identity
			target := m.state.Confirm.Input
			m.state.Confirm = ConfirmState{}
			return m, m.moveSecret(identity, source, target)
		case "copy":
			// For copy, target is the new name
			if m.state.Confirm.Input == "" {
				m.state.Confirm.ErrorMsg = "Target name cannot be empty"
				return m, nil
			}
			source, identity := m.state.Confirm.Target, m.state.Confirm.Identity
			target := m.state.Confirm.Input
			m.state.Confirm = ConfirmState{}
			return m, m.copySecretAs(identity, source, target)
		}
		return m, nil

	case "backspace":
		if len(m.state.Confirm.Input) > 0 {
			m.state.Confirm.Input = m.state.Confirm.Input[:len(m.state.Confirm.Input)-1]
			m.state.Confirm.ErrorMsg = ""
		}
		return m, nil

	default:
		// Add character to input
		if len(msg.String()) == 1 {
			m.state.Confirm.Input += msg.String()
			m.state.Confirm.ErrorMsg = ""
		}
		return m, nil
	}
}

// loadVersionValues loads values for specific versions
func (m Model) loadVersionValues(paramName string, versions []int64) tea.Cmd {
	identity, generation := m.state.DescribeIdentity, m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		versionMap := make(map[int64]string)
		for _, version := range versions {
			param, err := m.client.GetParameterByVersion(ctx, paramName, version, true)
			if err != nil {
				return resourceErrorMsg{identity: identity, generation: generation, message: "failed to load version value: " + err.Error()}
			}
			versionMap[version] = param.Value
		}
		return versionValuesLoadedMsg{name: paramName, identity: identity, generation: generation, versions: versionMap}
	}
}

// handleLabelInput handles input during label operations
func (m Model) handleLabelInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel label input
		m.state.LabelInputActive = false
		m.state.LabelInput = ""
		m.state.LabelError = ""
		return m, nil

	case "enter":
		// Submit label
		label := m.state.LabelInput
		if m.state.LabelSuggestionIndex >= 0 && m.state.LabelSuggestionIndex < len(m.state.LabelSuggestions) {
			label = m.state.LabelSuggestions[m.state.LabelSuggestionIndex]
		}

		if label == "" {
			m.state.LabelError = "Label cannot be empty"
			return m, nil
		}

		// Validate for add action
		if m.state.LabelAction == "add" {
			if err := util.ValidateLabel(label); err != nil {
				m.state.LabelError = err.Error()
				return m, nil
			}
		}

		// Bounds check before accessing history
		if m.state.HistoryIndex < 0 || m.state.HistoryIndex >= len(m.state.DescribeHistory) {
			m.state.LabelError = "No version selected"
			return m, nil
		}

		m.state.LabelInputActive = false
		entry := m.state.DescribeHistory[m.state.HistoryIndex]

		return m, m.executeLabelAction(m.state.LabelAction, label, entry.Version)

	case "tab", "down":
		// Next suggestion
		if len(m.state.LabelSuggestions) > 0 {
			m.state.LabelSuggestionIndex = (m.state.LabelSuggestionIndex + 1) % len(m.state.LabelSuggestions)
		}
		return m, nil

	case "shift+tab", "up":
		// Previous suggestion
		if len(m.state.LabelSuggestions) > 0 {
			m.state.LabelSuggestionIndex--
			if m.state.LabelSuggestionIndex < 0 {
				m.state.LabelSuggestionIndex = len(m.state.LabelSuggestions) - 1
			}
		}
		return m, nil

	case "backspace":
		if len(m.state.LabelInput) > 0 {
			m.state.LabelInput = m.state.LabelInput[:len(m.state.LabelInput)-1]
			m.state.LabelSuggestionIndex = -1
			m.updateLabelSuggestions()
		}
		return m, nil

	default:
		// Add character to input
		if len(msg.String()) == 1 {
			m.state.LabelInput += msg.String()
			m.state.LabelSuggestionIndex = -1
			m.updateLabelSuggestions()

			// Real-time validation for add
			if m.state.LabelAction == "add" {
				if err := util.ValidateLabel(m.state.LabelInput); err != nil {
					m.state.LabelError = err.Error()
				} else {
					m.state.LabelError = ""
				}
			}
		}
		return m, nil
	}
}

// updateLabelSuggestions filters suggestions based on current input
func (m *Model) updateLabelSuggestions() {
	if m.state.LabelInput == "" {
		if m.state.LabelAction == "add" {
			m.state.LabelSuggestions = util.SuggestLabels()
		}
		return
	}

	var filtered []string
	input := strings.ToLower(m.state.LabelInput)

	var source []string
	if m.state.LabelAction == "add" {
		source = util.SuggestLabels()
	} else {
		// For remove/move, use unique labels from history
		seen := make(map[string]bool)
		for _, h := range m.state.DescribeHistory {
			for _, l := range h.Labels {
				if !seen[l] {
					source = append(source, l)
					seen[l] = true
				}
			}
		}
	}

	for _, s := range source {
		if strings.Contains(strings.ToLower(s), input) {
			filtered = append(filtered, s)
		}
	}
	m.state.LabelSuggestions = filtered
}

// executeLabelAction performs the label operation
func (m Model) executeLabelAction(action, label string, version int64) tea.Cmd {
	identity, generation := m.state.DescribeIdentity, m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		paramName := m.state.DescribeParamName
		if paramName == "" {
			return labelCompleteMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("no parameter selected")}
		}

		switch action {
		case "add":
			input := &aws.LabelParameterInput{
				Name:    paramName,
				Version: version,
				Labels:  []string{label},
			}
			output, err := m.client.LabelParameterVersion(ctx, input)
			if err != nil {
				return labelCompleteMsg{identity: identity, generation: generation, action: action, err: err}
			}
			if len(output.InvalidLabels) > 0 {
				return labelCompleteMsg{
					identity: identity, generation: generation,
					action: action,
					err:    fmt.Errorf("invalid labels: %v", output.InvalidLabels),
				}
			}
			return labelCompleteMsg{identity: identity, generation: generation, action: action, label: label, version: version}

		case "remove":
			input := &aws.UnlabelParameterInput{
				Name:    paramName,
				Version: version,
				Labels:  []string{label},
			}
			err := m.client.UnlabelParameterVersion(ctx, input)
			if err != nil {
				return labelCompleteMsg{identity: identity, generation: generation, action: action, err: err}
			}
			return labelCompleteMsg{identity: identity, generation: generation, action: action, label: label, version: version}

		case "move":
			// Moving a label is just adding it to the new version
			// AWS automatically removes it from the old version
			input := &aws.LabelParameterInput{
				Name:    paramName,
				Version: version,
				Labels:  []string{label},
			}
			_, err := m.client.LabelParameterVersion(ctx, input)
			if err != nil {
				return labelCompleteMsg{identity: identity, generation: generation, action: action, err: err}
			}
			return labelCompleteMsg{identity: identity, generation: generation, action: action, label: label, version: version}
		}

		return labelCompleteMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("unknown action: %s", action)}
	}
}

// handleTagInput handles input during tag operations
func (m Model) handleTagInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state.TagInputActive = false
		m.state.TagInput = ""
		m.state.TagError = ""
		return m, nil

	case "enter":
		input := strings.TrimSpace(m.state.TagInput)
		if input == "" {
			m.state.TagError = "Input cannot be empty"
			return m, nil
		}

		paramName := m.state.DescribeParamName
		if paramName == "" {
			m.state.TagError = "No parameter selected"
			return m, nil
		}

		m.state.TagInputActive = false
		m.state.TagInput = ""
		m.state.TagError = ""

		return m, m.executeTagAction(m.state.TagAction, input)

	case "tab":
		// Cycle through suggestions
		if len(m.state.TagSuggestions) > 0 {
			m.state.TagSuggestionIndex = (m.state.TagSuggestionIndex + 1) % len(m.state.TagSuggestions)
			m.state.TagInput = m.state.TagSuggestions[m.state.TagSuggestionIndex]
		}
		return m, nil

	case "backspace":
		if len(m.state.TagInput) > 0 {
			m.state.TagInput = m.state.TagInput[:len(m.state.TagInput)-1]
		}
		m.state.TagError = ""
		return m, nil

	default:
		ch := msg.String()
		if len(ch) == 1 {
			m.state.TagInput += ch
			m.state.TagError = ""
		}
		return m, nil
	}
}

// executeTagAction performs the tag add/remove operation
func (m Model) executeTagAction(action, input string) tea.Cmd {
	identity, generation := m.state.DescribeIdentity, m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		paramName := m.state.DescribeParamName
		if paramName == "" {
			return tagCompleteMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("no parameter selected")}
		}

		switch action {
		case "add":
			// Parse key=value
			parts := strings.SplitN(input, "=", 2)
			if len(parts) != 2 || parts[0] == "" {
				return tagCompleteMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("format must be key=value")}
			}
			key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			err := m.client.AddTagsToResource(ctx, paramName, map[string]string{key: value})
			if err != nil {
				return tagCompleteMsg{identity: identity, generation: generation, action: action, err: err}
			}
			return tagCompleteMsg{identity: identity, generation: generation, action: "add", key: key}

		case "remove":
			key := strings.TrimSpace(input)
			err := m.client.RemoveTagsFromResource(ctx, paramName, []string{key})
			if err != nil {
				return tagCompleteMsg{identity: identity, generation: generation, action: action, err: err}
			}
			return tagCompleteMsg{identity: identity, generation: generation, action: "remove", key: key}
		}

		return tagCompleteMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("unknown action: %s", action)}
	}
}

// refreshTags refreshes tags for the current parameter
func (m Model) refreshTags() tea.Cmd {
	name, identity, generation := m.state.DescribeParamName, m.state.DescribeIdentity, m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		tags, err := m.client.GetParameterTags(ctx, name)
		if err != nil {
			return resourceErrorMsg{identity: identity, generation: generation, message: fmt.Sprintf("failed to refresh tags: %v", err)}
		}

		return tagsRefreshMsg{name: name, identity: identity, generation: generation, tags: tags}
	}
}

// refreshHistory refreshes the parameter history to show updated labels
func (m Model) refreshHistory() tea.Cmd {
	name, identity, generation := m.state.DescribeParamName, m.state.DescribeIdentity, m.state.DescribeGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		history, err := m.client.GetParameterHistory(ctx, name, 50, false)
		if err != nil {
			return resourceErrorMsg{identity: identity, generation: generation, message: fmt.Sprintf("failed to refresh history: %v", err)}
		}

		return historyRefreshMsg{name: name, identity: identity, generation: generation, history: history}
	}
}

// convertHistory converts AWS history to UI history entries
func convertHistory(awsHistory []aws.ParameterHistory) []HistoryEntry {
	var entries []HistoryEntry
	for _, h := range awsHistory {
		entries = append(entries, HistoryEntry{
			Version: h.Version,
			// A metadata response may include ciphertext. Only an explicit
			// decrypted version read sets ValueLoaded.
			Value:       "",
			Modified:    h.LastModifiedDate.Format("2006-01-02 15:04"),
			ValueLoaded: false,
			Labels:      h.Labels,
		})
	}
	return entries
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	var view string
	switch m.state.Mode {
	case ViewModeDescribe:
		view = m.renderDescribeView()
	default:
		view = m.renderBrowseView()
	}

	// Overlay confirm dialog if active
	if m.state.Confirm.Active {
		dialog := m.renderConfirmDialog()
		// Center the dialog
		view = centerDialog(dialog, m.state.Width, m.state.Height)
	}

	return view
}
