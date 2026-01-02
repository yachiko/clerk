package ui

import (
	"context"
	"fmt"
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
				m.searchInput.Blur()
				return m, nil
			case "enter":
				m.state.SearchActive = false
				m.searchInput.Blur()
				return m, nil
			case "down", "j":
				// Exit search and navigate down
				m.state.SearchActive = false
				m.searchInput.Blur()
				if m.state.SelectedIndex < len(m.state.FilteredItems)-1 {
					m.state.SelectedIndex++
					m.adjustScroll()
				}
				return m, nil
			case "up", "k":
				// Exit search and navigate up
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

			// Update filter on input change
			if m.state.SearchQuery != m.searchInput.Value() {
				m.state.SearchQuery = m.searchInput.Value()
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
		if m.config.SearchSlashPrefix {
			m.searchInput.SetValue("/")
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

	case "s":
		// Cycle through sort options: name -> modified -> version -> name
		switch m.state.SortType {
		case SortByName:
			m.state.SortType = SortByModified
		case SortByModified:
			m.state.SortType = SortByVersion
		case SortByVersion:
			m.state.SortType = SortByName
		}
		m.sortEntries()
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

	case "e":
		// Edit parameter
		if m.state.DescribeParamName != "" {
			return m, m.editSecret(m.state.DescribeParamName)
		}
		return m, nil

	case "tab":
		// Navigate to older version (increase index)
		if m.state.HistoryIndex < len(m.state.DescribeHistory)-1 {
			m.state.HistoryIndex++
			// Update value and trigger lazy load if needed
			return m.updateSelectedVersion()
		}
		return m, nil

	case "shift+tab":
		// Navigate to newer version (decrease index)
		if m.state.HistoryIndex > 0 {
			m.state.HistoryIndex--
			// Update value and trigger lazy load if needed
			return m.updateSelectedVersion()
		}
		return m, nil

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

	case "l":
		// Jump to latest version
		if m.state.HistoryIndex != 0 {
			m.state.HistoryIndex = 0
			// Update value and trigger lazy load if needed
			return m.updateSelectedVersion()
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

// filterEntries filters and sorts entries based on search query and sort order
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

// sortEntries sorts FilteredItems based on the current sort type
func (m *Model) sortEntries() {
	if len(m.state.FilteredItems) == 0 {
		return
	}

	switch m.state.SortType {
	case SortByName:
		// Sort by name (ascending)
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				if m.state.FilteredItems[j].Name < m.state.FilteredItems[i].Name {
					m.state.FilteredItems[i], m.state.FilteredItems[j] = m.state.FilteredItems[j], m.state.FilteredItems[i]
				}
			}
		}
	case SortByModified:
		// Sort by last modified date (newest first)
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				if m.state.FilteredItems[j].LastModifiedDate.After(m.state.FilteredItems[i].LastModifiedDate) {
					m.state.FilteredItems[i], m.state.FilteredItems[j] = m.state.FilteredItems[j], m.state.FilteredItems[i]
				}
			}
		}
	case SortByVersion:
		// Sort by version (highest first)
		for i := 0; i < len(m.state.FilteredItems)-1; i++ {
			for j := i + 1; j < len(m.state.FilteredItems); j++ {
				if m.state.FilteredItems[j].Version > m.state.FilteredItems[i].Version {
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
	// Get confirmation word (last path segment, max 10 chars)
	parts := strings.Split(name, "/")
	confirmText := parts[len(parts)-1]
	if len(confirmText) > 10 {
		confirmText = confirmText[:10]
	}

	m.state.Confirm = ConfirmState{
		Active:      true,
		Action:      "delete",
		Target:      name,
		ConfirmText: confirmText,
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
