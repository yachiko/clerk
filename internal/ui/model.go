package ui

import (
	"context"
	"fmt"
	"os"
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
			PreviousMode:  ViewModeList,
			ExpandedPaths: make(map[string]bool),
			SortType:      SortByName,
			SortAscending: true,
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
		tea.EnableMouseAllMotion,
		m.loadEntries,
		m.checkBackgroundRefresh,
	)
}

// loadEntries loads entries from cache
func (m Model) loadEntries() tea.Msg {
	entries := m.cache.GetAll()
	// Update cache age
	m.state.CacheAge = m.cache.GetAge()
	return entriesLoadedMsg{entries: entries}
}

// checkBackgroundRefresh checks if cache should be refreshed in background
func (m Model) checkBackgroundRefresh() tea.Msg {
	// Skip if auto-refresh is disabled
	if !m.config.BrowseAutoRefresh {
		return nil
	}

	// Check if cache is empty (first run)
	if len(m.cache.GetAll()) == 0 {
		return backgroundRefreshStartMsg{}
	}

	// Check cache age
	cacheAge := m.cache.GetAge()
	if cacheAge < m.config.BrowseRefreshCooldown {
		// Cache is fresh, no refresh needed
		return nil
	}

	// Cache is stale, trigger background refresh
	return backgroundRefreshStartMsg{}
}

// doBackgroundRefresh performs cache refresh in background with live progress
func (m Model) doBackgroundRefresh() tea.Cmd {
	return func() tea.Msg {
		// This will be replaced by the streaming version
		return startRefreshWithProgress(m.cache, m.client, m.config)
	}
}

// startRefreshWithProgress starts the refresh and returns a sub for progress updates
func startRefreshWithProgress(cacheMgr *cache.Manager, client *aws.Client, cfg *config.Config) tea.Msg {
	// Create a channel for progress
	progressCh := make(chan int, 100)

	// Start refresh in background
	go func() {
		ctx := context.Background()

		progressCallback := func(current, total int) {
			select {
			case progressCh <- current:
			default:
				// Channel full, skip this update
			}
		}

		// Use the actual region from the client, not config
		err := cacheMgr.Refresh(ctx, client, client.GetRegion(), cfg.ParallelFetches, progressCallback)

		// Signal completion
		close(progressCh)

		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Background refresh failed: %v\n", err)
		}
	}()

	// Return a message that starts listening for progress
	return refreshProgressChannelMsg{ch: progressCh}
}

// refreshProgressChannelMsg carries the progress channel
type refreshProgressChannelMsg struct {
	ch chan int
}

// waitForProgress returns a command that waits for the next progress update
func waitForProgress(ch chan int) tea.Cmd {
	return func() tea.Msg {
		count, ok := <-ch
		if !ok {
			// Channel closed, refresh is done
			return backgroundRefreshCompleteMsg{loadFromCache: true}
		}
		return backgroundRefreshProgressMsg{current: count, ch: ch}
	}
}

type entriesLoadedMsg struct {
	entries []cache.CacheEntry
}

type backgroundRefreshStartMsg struct{}
type backgroundRefreshProgressMsg struct {
	current int
	ch      chan int
}
type backgroundRefreshCompleteMsg struct {
	// loadFromCache signals that the handler should re-read entries from
	// the cache manager rather than carrying them on the message.
	loadFromCache bool
}

type statusMsg string
type errorMsg string
type clearStatusMsg struct{}
type describeLoadedMsg struct {
	value   string
	history []HistoryEntry
}

type versionValuesLoadedMsg struct {
	versions map[int64]string // version -> value mapping
}

type editCompleteMsg struct {
	name     string
	newValue string
	version  int64
	err      error
}

type deleteCompleteMsg struct {
	name string
	err  error
}

type moveCompleteMsg struct {
	source string
	target string
	err    error
}

type copyCompleteMsg struct {
	source string
	target string
	err    error
}

// Label operation messages
type labelCompleteMsg struct {
	action  string
	label   string
	version int64
	err     error
}

type tagCompleteMsg struct {
	action string
	key    string
	err    error
}

type tagsRefreshMsg struct {
	tags map[string]string
}

type historyRefreshMsg struct {
	history []aws.ParameterHistory
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
		// Handle mouse wheel scrolling
		if m.state.Mode == ViewModeDescribe {
			// Describe view: scroll value vertically or horizontally
			switch msg.Type {
			case tea.MouseWheelUp:
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
			case tea.MouseWheelDown:
				// Shift + wheel: horizontal scroll
				if msg.Shift && !m.state.ValueLineWrap {
					m.state.ValueHorizontalScroll++
				} else {
					// Normal wheel: vertical scroll
					m.state.ValueScrollOffset++
				}
				return m, nil
			}
		} else if m.state.Mode == ViewModeList || m.state.Mode == ViewModeTree {
			// Browse view: scroll through entries
			switch msg.Type {
			case tea.MouseWheelUp:
				if m.state.SelectedIndex > 0 {
					m.state.SelectedIndex--
					m.adjustScroll()
				}
				return m, nil
			case tea.MouseWheelDown:
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
		m.state.CacheAge = m.cache.GetAge()
		m.filterEntries()
		return m, nil

	case backgroundRefreshStartMsg:
		// Show appropriate message based on whether cache is empty
		if len(m.state.Entries) == 0 {
			m.state.StatusMessage = "Loading parameters from AWS..."
		} else {
			m.state.StatusMessage = "Refreshing cache in background..."
		}
		return m, m.doBackgroundRefresh()

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
		// Load entries from cache
		entries := m.cache.GetAll()

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

	case clearStatusMsg:
		m.state.StatusMessage = ""
		m.state.ErrorMessage = ""
		return m, nil

	case describeLoadedMsg:
		m.state.DescribeValue = msg.value
		m.state.DescribeHistory = msg.history
		m.state.HistoryIndex = 0
		// Store param name for lazy loading
		if m.state.DescribeEntry != nil {
			m.state.DescribeParamName = m.state.DescribeEntry.Name
		}
		return m, nil

	case versionValuesLoadedMsg:
		// Update history entries with loaded values
		for i := range m.state.DescribeHistory {
			if value, ok := msg.versions[m.state.DescribeHistory[i].Version]; ok {
				m.state.DescribeHistory[i].Value = value
				m.state.DescribeHistory[i].ValueLoaded = true
			}
		}
		// Update current displayed value if it was loaded
		if m.state.HistoryIndex < len(m.state.DescribeHistory) {
			m.state.DescribeValue = m.state.DescribeHistory[m.state.HistoryIndex].Value
		}
		return m, nil

	case editCompleteMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return errorMsg(msg.err.Error()) }
		}

		// Update cache
		for i := range m.state.Entries {
			if m.state.Entries[i].Name == msg.name {
				m.state.Entries[i].Version = msg.version
				m.state.Entries[i].LastModifiedDate = time.Now()
				break
			}
		}
		m.filterEntries()

		// Update cache manager
		if entry, ok := m.cache.Get(msg.name); ok {
			entry.Version = msg.version
			entry.LastModifiedDate = time.Now()
			_ = m.cache.Update(*entry)
		}

		return m, func() tea.Msg {
			return statusMsg(fmt.Sprintf("Updated %s to version %d", msg.name, msg.version))
		}

	case deleteCompleteMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return errorMsg("Delete failed: " + msg.err.Error()) }
		}

		// Remove from cache
		_ = m.cache.Delete(msg.name)

		// Reload entries from cache
		return m, tea.Batch(
			m.loadEntries,
			func() tea.Msg {
				return statusMsg(fmt.Sprintf("Deleted %s", msg.name))
			},
		)

	case moveCompleteMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return errorMsg("Move failed: " + msg.err.Error()) }
		}

		// Remove from cache
		_ = m.cache.Delete(msg.source)

		// Reload entries from cache
		return m, tea.Batch(
			m.loadEntries,
			func() tea.Msg {
				return statusMsg(fmt.Sprintf("Moved %s to %s", msg.source, msg.target))
			},
		)

	case copyCompleteMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return errorMsg("Copy failed: " + msg.err.Error()) }
		}

		// Reload entries from cache
		return m, tea.Batch(
			m.loadEntries,
			func() tea.Msg {
				return statusMsg(fmt.Sprintf("Copied %s to %s", msg.source, msg.target))
			},
		)

	case labelCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("Label %s failed: %v", msg.action, msg.err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Refresh history to show updated labels
		m.state.StatusMessage = fmt.Sprintf("Label '%s' %sed on v%d", msg.label, msg.action, msg.version)

		// Trigger history refresh
		return m, m.refreshHistory()

	case tagCompleteMsg:
		if msg.err != nil {
			m.state.ErrorMessage = fmt.Sprintf("Tag %s failed: %v", msg.action, msg.err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Refresh tags on the entry
		m.state.StatusMessage = fmt.Sprintf("Tag '%s' %sed", msg.key, msg.action)
		return m, tea.Batch(
			m.refreshTags(),
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			}),
		)

	case tagsRefreshMsg:
		if m.state.DescribeEntry != nil {
			m.state.DescribeEntry.Tags = msg.tags
		}
		return m, nil

	case historyRefreshMsg:
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
			m.state.Mode = m.state.PreviousMode
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
		} else if m.state.Mode == ViewModeTree {
			m.state.Mode = ViewModeList
		}
		// Update PreviousMode when in browse mode (not describe)
		m.state.PreviousMode = m.state.Mode
		return m, nil

	case "r":
		// Manual refresh cache
		m.state.StatusMessage = "Refreshing cache..."
		return m, m.doBackgroundRefresh()

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
				return m, func() tea.Msg {
					return errorMsg("Invalid parameter name")
				}
			}
			m.state.DescribeEntry = entry
			m.state.DescribeValue = "" // Clear previous value to show "Loading..."
			m.state.DescribeHistory = nil
			m.state.HistoryIndex = 0
			m.state.HistoryScrollOffset = 0
			m.state.ValueScrollOffset = 0
			m.state.ValueHorizontalScroll = 0
			m.state.PreviousMode = m.state.Mode // Store current mode to restore later
			m.state.Mode = ViewModeDescribe
			// Use config to determine if value should be masked by default
			m.state.DescribeMasked = !m.config.DecryptByDefault
			return m, m.loadDescribe(paramName)
		}
		return m, nil

	case "c":
		// Copy secret value
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.copySecret(entry.Name)
		}
		return m, nil

	case "e":
		// Edit
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.editSecret(entry.Name)
		}
		return m, nil

	case "delete":
		// Delete (requires confirmation)
		entry := m.getSelectedEntry()
		if entry != nil {
			return m, m.initiateDelete(entry.Name)
		}
		return m, nil

	case "m":
		// Move/rename
		entry := m.getSelectedEntry()
		if entry != nil {
			m.state.Confirm = ConfirmState{
				Active: true,
				Action: "move",
				Target: entry.Name,
			}
		}
		return m, nil

	case "p":
		// Copy
		entry := m.getSelectedEntry()
		if entry != nil {
			m.state.Confirm = ConfirmState{
				Active: true,
				Action: "copy",
				Target: entry.Name,
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
		m.state.Mode = m.state.PreviousMode
		// Reset scroll offsets
		m.state.HistoryScrollOffset = 0
		m.state.ValueScrollOffset = 0
		m.state.ValueHorizontalScroll = 0
		return m, nil

	case "x":
		// Toggle masked/unmasked
		m.state.DescribeMasked = !m.state.DescribeMasked
		return m, nil

	case "w":
		// Toggle line wrapping
		m.state.ValueLineWrap = !m.state.ValueLineWrap
		m.state.ValueHorizontalScroll = 0 // Reset horizontal scroll when toggling wrap
		return m, nil

	case "c":
		// Copy value
		if m.state.DescribeValue != "" {
			return m, m.copyValue(m.state.DescribeValue)
		}
		return m, nil

	case "C":
		// Copy parameter name/path
		if m.state.DescribeParamName != "" {
			return m, m.copyValue(m.state.DescribeParamName)
		}
		return m, nil

	case "e":
		// Edit parameter
		if m.state.DescribeParamName != "" {
			return m, m.editSecret(m.state.DescribeParamName)
		}
		return m, nil

	case "tab":
		// Navigate to older version (increase index), loop to beginning
		if m.state.HistoryIndex < len(m.state.DescribeHistory)-1 {
			m.state.HistoryIndex++
		} else {
			m.state.HistoryIndex = 0
		}
		// Update value and trigger lazy load if needed
		return m.updateSelectedVersion()

	case "shift+tab":
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
	if m.state.HistoryIndex >= len(m.state.DescribeHistory) {
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

	// Reset selection if out of bounds
	if m.state.SelectedIndex >= len(m.state.FilteredItems) {
		m.state.SelectedIndex = len(m.state.FilteredItems) - 1
	}
	if m.state.SelectedIndex < 0 {
		m.state.SelectedIndex = 0
	}
}

// sortEntries sorts FilteredItems based on the current sort type and direction
func (m *Model) sortEntries() {
	if len(m.state.FilteredItems) == 0 {
		return
	}

	asc := m.state.SortAscending

	switch m.state.SortType {
	case SortByName:
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				less := m.state.FilteredItems[j].Name < m.state.FilteredItems[i].Name
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
}

// loadDescribe loads describe data for a parameter
func (m Model) loadDescribe(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Validate parameter name
		if name == "" {
			return errorMsg("Invalid parameter name")
		}

		// Get current value
		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			// Check if it's an auth/network error
			errMsg := err.Error()
			if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "credential") ||
				strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "network") {
				return errorMsg("Unable to retrieve secret value. Check your AWS credentials and network connection.")
			}
			return errorMsg("Failed to load parameter: " + err.Error())
		}

		var historyEntries []HistoryEntry

		// Check if we have cached version history
		if cacheEntry, ok := m.cache.Get(name); ok && len(cacheEntry.VersionHistory) > 0 {
			// Use cached history metadata and fetch values
			allVersions, err := m.client.GetParameterHistory(ctx, name, 50, true)
			if err != nil {
				// Continue without history
				return describeLoadedMsg{
					value:   param.Value,
					history: []HistoryEntry{{Version: param.Version, Value: param.Value, Modified: param.LastModifiedDate.Format(time.RFC3339), ValueLoaded: true}},
				}
			}

			// Build history entries with all values
			for _, h := range allVersions {
				historyEntries = append(historyEntries, HistoryEntry{
					Version:     h.Version,
					Value:       h.Value,
					Modified:    h.LastModifiedDate.Format(time.RFC3339),
					ValueLoaded: true,
					Labels:      h.Labels,
				})
			}
		} else {
			// No cached history, fetch from AWS
			allVersions, err := m.client.GetParameterHistory(ctx, name, 50, true)
			if err != nil {
				// Continue without history
				return describeLoadedMsg{
					value:   param.Value,
					history: []HistoryEntry{{Version: param.Version, Value: param.Value, Modified: param.LastModifiedDate.Format(time.RFC3339), ValueLoaded: true}},
				}
			}

			// Build history entries with all values
			for _, h := range allVersions {
				historyEntries = append(historyEntries, HistoryEntry{
					Version:     h.Version,
					Value:       h.Value,
					Modified:    h.LastModifiedDate.Format(time.RFC3339),
					ValueLoaded: true,
					Labels:      h.Labels,
				})
			}
		}

		// Reverse to show newest first
		for i, j := 0, len(historyEntries)-1; i < j; i, j = i+1, j-1 {
			historyEntries[i], historyEntries[j] = historyEntries[j], historyEntries[i]
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
			// Check if it's an auth/network error
			errMsg := err.Error()
			if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "credential") ||
				strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "network") {
				return errorMsg("Unable to retrieve secret value. Check your AWS credentials and network connection.")
			}
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

// editSecret opens an editor to edit the parameter
func (m Model) editSecret(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Get current value
		param, err := m.client.GetParameter(ctx, name, true)
		if err != nil {
			return editCompleteMsg{err: fmt.Errorf("failed to get parameter: %w", err)}
		}

		// Determine file extension based on content
		ext := ".txt"
		trimmedVal := strings.TrimSpace(param.Value)
		if strings.HasPrefix(trimmedVal, "{") {
			ext = ".json"
		} else if strings.HasPrefix(trimmedVal, "<") {
			ext = ".xml"
		}

		// Open in editor
		editor := util.NewEditor(util.EditorConfig{})
		newValue, err := editor.Edit(param.Value, ext)
		if err != nil {
			return editCompleteMsg{err: fmt.Errorf("editor error: %w", err)}
		}

		// Check if value changed
		newValue = strings.TrimSpace(newValue)
		if newValue == param.Value {
			return statusMsg("No changes made")
		}

		// Update parameter
		input := &aws.PutParameterInput{
			Name:      name,
			Value:     newValue,
			Type:      param.Type,
			Overwrite: true,
		}

		output, err := m.client.PutParameter(ctx, input)
		if err != nil {
			return editCompleteMsg{err: fmt.Errorf("failed to update: %w", err)}
		}

		return editCompleteMsg{
			name:     name,
			newValue: newValue,
			version:  output.Version,
		}
	}
}

// initiateDelete starts the delete confirmation flow
func (m *Model) initiateDelete(name string) tea.Cmd {
	m.state.Confirm = ConfirmState{
		Active:      true,
		Action:      "delete",
		Target:      name,
		ConfirmText: "delete me",
	}

	return nil
}

// deleteSecret performs the actual deletion
func (m Model) deleteSecret(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := m.client.DeleteParameter(ctx, name)
		if err != nil {
			return deleteCompleteMsg{name: name, err: err}
		}

		return deleteCompleteMsg{name: name}
	}
}

// moveSecret moves/renames a parameter
func (m Model) moveSecret(source, target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get source parameter
		param, err := m.client.GetParameter(ctx, source, true)
		if err != nil {
			return moveCompleteMsg{source: source, target: target, err: fmt.Errorf("failed to get source: %w", err)}
		}

		// Create new parameter at target location
		input := &aws.PutParameterInput{
			Name:      target,
			Value:     param.Value,
			Type:      param.Type,
			Overwrite: true,
		}
		if len(param.Tags) > 0 {
			input.Tags = param.Tags
		}

		_, err = m.client.PutParameter(ctx, input)
		if err != nil {
			return moveCompleteMsg{source: source, target: target, err: fmt.Errorf("failed to create target: %w", err)}
		}

		// Delete source parameter
		err = m.client.DeleteParameter(ctx, source)
		if err != nil {
			return moveCompleteMsg{source: source, target: target, err: fmt.Errorf("failed to delete source: %w", err)}
		}

		return moveCompleteMsg{source: source, target: target}
	}
}

// copySecretAs copies a parameter to a new name
func (m Model) copySecretAs(source, target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get source parameter
		param, err := m.client.GetParameter(ctx, source, true)
		if err != nil {
			return copyCompleteMsg{source: source, target: target, err: fmt.Errorf("failed to get source: %w", err)}
		}

		// Create new parameter at target location
		input := &aws.PutParameterInput{
			Name:      target,
			Value:     param.Value,
			Type:      param.Type,
			Overwrite: true,
		}
		if len(param.Tags) > 0 {
			input.Tags = param.Tags
		}

		_, err = m.client.PutParameter(ctx, input)
		if err != nil {
			return copyCompleteMsg{source: source, target: target, err: fmt.Errorf("failed to copy: %w", err)}
		}

		return copyCompleteMsg{source: source, target: target}
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
		if m.state.Confirm.Action == "delete" {
			// For delete, check confirmation text
			if m.state.Confirm.Input == m.state.Confirm.ConfirmText {
				name := m.state.Confirm.Target
				m.state.Confirm = ConfirmState{}
				return m, m.deleteSecret(name)
			}
			m.state.Confirm.ErrorMsg = "Incorrect confirmation text"
			return m, nil
		} else if m.state.Confirm.Action == "move" {
			// For move, target is the new name
			if m.state.Confirm.Input == "" {
				m.state.Confirm.ErrorMsg = "Target name cannot be empty"
				return m, nil
			}
			source := m.state.Confirm.Target
			target := m.state.Confirm.Input
			m.state.Confirm = ConfirmState{}
			return m, m.moveSecret(source, target)
		} else if m.state.Confirm.Action == "copy" {
			// For copy, target is the new name
			if m.state.Confirm.Input == "" {
				m.state.Confirm.ErrorMsg = "Target name cannot be empty"
				return m, nil
			}
			source := m.state.Confirm.Target
			target := m.state.Confirm.Input
			m.state.Confirm = ConfirmState{}
			return m, m.copySecretAs(source, target)
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get history with decryption (max 50 per API limit)
		history, err := m.client.GetParameterHistory(ctx, paramName, 50, true)
		if err != nil {
			return errorMsg("Failed to load version values: " + err.Error())
		}

		// Build map of version -> value for requested versions
		versionMap := make(map[int64]string)
		for _, h := range history {
			for _, targetVersion := range versions {
				if h.Version == targetVersion {
					versionMap[targetVersion] = h.Value
					break
				}
			}
		}

		return versionValuesLoadedMsg{versions: versionMap}
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		paramName := m.state.DescribeParamName
		if paramName == "" {
			return labelCompleteMsg{action: action, err: fmt.Errorf("no parameter selected")}
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
				return labelCompleteMsg{action: action, err: err}
			}
			if len(output.InvalidLabels) > 0 {
				return labelCompleteMsg{
					action: action,
					err:    fmt.Errorf("invalid labels: %v", output.InvalidLabels),
				}
			}
			return labelCompleteMsg{action: action, label: label, version: version}

		case "remove":
			input := &aws.UnlabelParameterInput{
				Name:    paramName,
				Version: version,
				Labels:  []string{label},
			}
			err := m.client.UnlabelParameterVersion(ctx, input)
			if err != nil {
				return labelCompleteMsg{action: action, err: err}
			}
			return labelCompleteMsg{action: action, label: label, version: version}

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
				return labelCompleteMsg{action: action, err: err}
			}
			return labelCompleteMsg{action: action, label: label, version: version}
		}

		return labelCompleteMsg{action: action, err: fmt.Errorf("unknown action: %s", action)}
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		paramName := m.state.DescribeParamName
		if paramName == "" {
			return tagCompleteMsg{action: action, err: fmt.Errorf("no parameter selected")}
		}

		switch action {
		case "add":
			// Parse key=value
			parts := strings.SplitN(input, "=", 2)
			if len(parts) != 2 || parts[0] == "" {
				return tagCompleteMsg{action: action, err: fmt.Errorf("format must be key=value")}
			}
			key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			err := m.client.AddTagsToResource(ctx, paramName, map[string]string{key: value})
			if err != nil {
				return tagCompleteMsg{action: action, err: err}
			}
			return tagCompleteMsg{action: "add", key: key}

		case "remove":
			key := strings.TrimSpace(input)
			err := m.client.RemoveTagsFromResource(ctx, paramName, []string{key})
			if err != nil {
				return tagCompleteMsg{action: action, err: err}
			}
			return tagCompleteMsg{action: "remove", key: key}
		}

		return tagCompleteMsg{action: action, err: fmt.Errorf("unknown action: %s", action)}
	}
}

// refreshTags refreshes tags for the current parameter
func (m Model) refreshTags() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		tags, err := m.client.GetParameterTags(ctx, m.state.DescribeParamName)
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to refresh tags: %v", err))
		}

		return tagsRefreshMsg{tags: tags}
	}
}

// refreshHistory refreshes the parameter history to show updated labels
func (m Model) refreshHistory() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		history, err := m.client.GetParameterHistory(ctx, m.state.DescribeParamName, 50, false)
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to refresh history: %v", err))
		}

		return historyRefreshMsg{history: history}
	}
}

// convertHistory converts AWS history to UI history entries
func convertHistory(awsHistory []aws.ParameterHistory) []HistoryEntry {
	var entries []HistoryEntry
	for _, h := range awsHistory {
		entries = append(entries, HistoryEntry{
			Version:     h.Version,
			Value:       h.Value,
			Modified:    h.LastModifiedDate.Format("2006-01-02 15:04"),
			ValueLoaded: h.Value != "", // Value is loaded if not empty
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
