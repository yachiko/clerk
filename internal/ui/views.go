package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/util"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("252"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	searchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("252"))

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230"))

	maskedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	versionSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1)

	versionNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Padding(0, 1)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(1, 2).
			Width(50)

)

const (
	// MaxInlineLabels is the maximum number of labels to show inline in version history
	MaxInlineLabels = 2
	// MaxLabelBadgeWidth is the maximum width for a label badge before truncation
	MaxLabelBadgeWidth = 12
)

// renderBrowseView renders the main browse view
func (m Model) renderBrowseView() string {
	var lines []string

	// Title bar (fixed at top)
	mode := "LIST"
	if m.state.Mode == ViewModeTree {
		mode = "TREE"
	}
	title := titleStyle.Render(fmt.Sprintf(" CLERK - %s ", mode))
	lines = append(lines, title)

	// Search bar (always visible, fixed at top)
	if m.state.SearchActive {
		searchLine := "  " + searchStyle.Render("🔍 ") + m.searchInput.View()

		// Show suggestion as ghost text
		if m.state.CurrentSuggestion != "" && m.state.CurrentSuggestion != m.state.SearchQuery {
			// Show the part that would be completed in dim style
			ghostText := m.state.CurrentSuggestion[len(m.state.SearchQuery):]
			searchLine += dimStyle.Render(ghostText)
		}

		lines = append(lines, searchLine)
	} else if m.state.SearchQuery != "" {
		lines = append(lines, dimStyle.Render("  Filter: "+m.state.SearchQuery+" (/ to edit)"))
	} else {
		lines = append(lines, "")
	}

	// Header - varies based on width
	showModified := m.state.Width >= 100 // Show modified column if width >= 100
	var header string
	var nameWidth int

	if showModified {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) + 3 (spaces) + 16 (MODIFIED) + 2 (padding) = 47
		nameWidth = m.state.Width - 47 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20 // Minimum width for name
		}
		header = headerStyle.Render(fmt.Sprintf("  %-*s   %-12s   %8s   %16s  ", nameWidth, "NAME", "TYPE", "VERSION", "MODIFIED"))
	} else {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) = 26
		nameWidth = m.state.Width - 26 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20 // Minimum width for name
		}
		header = headerStyle.Render(fmt.Sprintf("  %-*s   %-12s   %8s", nameWidth, "NAME", "TYPE", "VERSION"))
	}
	lines = append(lines, header)
	lines = append(lines, "  "+strings.Repeat("─", m.state.Width-4))

	// Items - calculate how many we can show
	visible := m.visibleRows()
	if visible < 1 {
		visible = 10
	}

	if len(m.state.FilteredItems) == 0 {
		lines = append(lines, dimStyle.Render("    No parameters found"))
		// Pad with empty lines to fill space
		for i := len(lines); i < m.state.Height-2; i++ {
			lines = append(lines, "")
		}
	} else {
		start := m.state.ScrollOffset
		end := start + visible
		if end > len(m.state.FilteredItems) {
			end = len(m.state.FilteredItems)
		}

		var itemLines []string
		if m.state.Mode == ViewModeTree {
			b := &strings.Builder{}
			m.renderTreeItems(b, start, end, showModified)
			itemLines = strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		} else {
			b := &strings.Builder{}
			m.renderListItems(b, start, end, showModified)
			itemLines = strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		}
		lines = append(lines, itemLines...)

		// Pad with empty lines to fill space
		for len(lines) < m.state.Height-2 {
			lines = append(lines, "")
		}
	}

	// Footer (always at bottom)
	lines = append(lines, "  "+strings.Repeat("─", m.state.Width-4))

	// Status/Error message
	var statusLine string
	if m.state.ErrorMessage != "" {
		statusLine = errorStyle.Render("  ✗ " + m.state.ErrorMessage)
	} else if m.state.StatusMessage != "" {
		statusLine = statusStyle.Render("  ✓ " + m.state.StatusMessage)
	} else {
		// Stats with offline mode indicator
		stats := fmt.Sprintf("%d/%d parameters", len(m.state.FilteredItems), len(m.state.Entries))

		// Add offline mode indicator or cache age
		if m.state.OfflineMode {
			stats += " [Offline Mode]"
		} else if m.state.CacheAge > 0 {
			// Show cache age if older than 1 minute
			if m.state.CacheAge > time.Minute {
				cacheAge := formatDuration(m.state.CacheAge)
				stats += fmt.Sprintf(" [Cached: %s ago]", cacheAge)
			}
		}

		statusLine = dimStyle.Render("  " + stats)
	}
	lines = append(lines, statusLine)

	// Help line - varies by view mode
	var help string
	sortLabel := m.getSortLabel()
	if m.state.Mode == ViewModeTree {
		help = fmt.Sprintf("↑↓:navigate  d:describe  e:edit  c:copy  m:move  p:copy  space:expand  s:sort(%s)  r:refresh  /:search  q:quit", sortLabel)
	} else {
		help = fmt.Sprintf("↑↓:navigate  d:describe  e:edit  c:copy  m:move  p:copy  t:tree  s:sort(%s)  r:refresh  /:search  q:quit", sortLabel)
	}
	lines = append(lines, helpStyle.Render("  "+help+"  "))

	return strings.Join(lines, "\n")
}

// renderListItems renders items in flat list view
func (m Model) renderListItems(b *strings.Builder, start, end int, showModified bool) {
	// Calculate name width based on terminal width
	var nameWidth int
	if showModified {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) + 3 (spaces) + 16 (MODIFIED) + 2 (padding) = 47
		nameWidth = m.state.Width - 47 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20
		}
	} else {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) = 26
		nameWidth = m.state.Width - 26 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20
		}
	}

	for i := start; i < end; i++ {
		entry := m.state.FilteredItems[i]

		var line string
		if showModified {
			name := truncateString(entry.Name, nameWidth-2) // -2 for padding
			modifiedStr := entry.LastModifiedDate.Format("2006-01-02 15:04")
			line = fmt.Sprintf("  %-*s   %-12s   %8d   %16s  ", nameWidth, name, entry.Type, entry.Version, modifiedStr)
		} else {
			name := truncateString(entry.Name, nameWidth-2) // -2 for padding
			line = fmt.Sprintf("  %-*s   %-12s   %8d", nameWidth, name, entry.Type, entry.Version)
		}

		if i == m.state.SelectedIndex {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}
}

// renderTreeItems renders items in tree view
func (m Model) renderTreeItems(b *strings.Builder, start, end int, showModified bool) {
	if len(m.state.TreeNodes) == 0 {
		return
	}

	// Calculate name width based on terminal width and fixed columns
	var nameWidth int
	if showModified {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) + 3 (spaces) + 16 (MODIFIED) + 2 (padding) = 47
		nameWidth = m.state.Width - 47 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20
		}
	} else {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) = 26
		nameWidth = m.state.Width - 26 - 2 // -2 for left padding
		if nameWidth < 20 {
			nameWidth = 20
		}
	}

	for i := start; i < end && i < len(m.state.TreeNodes); i++ {
		node := m.state.TreeNodes[i]

		// Build indentation and prefix
		indent := strings.Repeat("  ", node.Depth)
		var prefix string
		if node.IsDir {
			if node.Expanded {
				prefix = "▼ "
			} else {
				prefix = "▶ "
			}
		} else {
			prefix = "  "
		}

		// Build line content
		var line string
		if node.IsDir {
			dirName := node.Name + "/"
			if node.ChildCount > 0 {
				dirName += fmt.Sprintf(" (%d)", node.ChildCount)
			}
			// For directories, use dynamic width considering indent
			dirNameWidth := nameWidth - len(indent) - 2
			if dirNameWidth < 10 {
				dirNameWidth = 10
			}
			line = fmt.Sprintf("  %s%s%-*s", indent, prefix, dirNameWidth, dirName)
		} else {
			entry := node.Entry
			// Account for indent when calculating available space
			availableWidth := nameWidth - len(indent) - 2
			if availableWidth < 10 {
				availableWidth = 10
			}

			if showModified {
				name := truncateString(node.Name, availableWidth-2)
				modifiedStr := entry.LastModifiedDate.Format("2006-01-02 15:04")
				line = fmt.Sprintf("  %s%s%-*s   %-12s   %8d   %16s  ", indent, prefix, availableWidth, name, entry.Type, entry.Version, modifiedStr)
			} else {
				name := truncateString(node.Name, availableWidth-2)
				line = fmt.Sprintf("  %s%s%-*s   %-12s   %8d", indent, prefix, availableWidth, name, entry.Type, entry.Version)
			}
		}

		if i == m.state.SelectedIndex {
			b.WriteString(selectedStyle.Render(line))
		} else if node.IsDir {
			b.WriteString(dimStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}
}

// renderDescribeView renders the describe view
func (m Model) renderDescribeView() string {
	var output []string

	if m.state.DescribeEntry == nil {
		return "No entry selected"
	}

	entry := m.state.DescribeEntry

	// Title (fixed at top)
	title := titleStyle.Render(" DESCRIBE ")
	output = append(output, title)

	// Empty line (equivalent to search bar in browse view for layout consistency)
	output = append(output, "")

	// Parameter info box (equivalent to header in browse view)
	box := m.renderDescribeBox(entry)
	output = append(output, box)

	// Separator line (matching browse view structure)
	output = append(output, "  "+strings.Repeat("─", m.state.Width-4))

	// Calculate panel dimensions
	// Left panel: MarginLeft(2) + Width(35) = 37 total
	// Right panel: Width(rightWidth) + MarginRight(2) = rightWidth + 2 total
	// Total: 37 + rightWidth + 2 = m.state.Width
	// So: rightWidth = m.state.Width - 39
	leftWidth := 35                  // Version history panel width
	rightWidth := m.state.Width - 43 // Account for left panel (37) and right margin (2)
	if rightWidth < 40 {
		rightWidth = 40
	}

	// Calculate available height for panels
	// Title(1) + empty(1) + header(1-2) + separator(1) + panels + separator(1) + status(1) + help(1) = Height
	// Header is now compact: 1-2 lines instead of 5-7
	panelHeight := m.state.Height - 8
	if panelHeight < 10 {
		panelHeight = 10
	}

	// Render left panel (version history)
	leftPanel := m.renderVersionHistoryPanel(leftWidth, panelHeight)

	// Render right panel (value)
	rightPanel := m.renderValuePanel(rightWidth, panelHeight)

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	output = append(output, panels)

	// Calculate how many lines we have so far to properly pad
	linesUsed := 1 + 1 + strings.Count(box, "\n") + 1 + 1 + strings.Count(panels, "\n") + 1 + 1 + 1
	// Pad to fill space (similar to browse view)
	for linesUsed < m.state.Height {
		output = append(output, "")
		linesUsed++
	}

	// Footer separator (matching browse view structure)
	output = append(output, "  "+strings.Repeat("─", m.state.Width-4))

	// Status/Error message
	var statusLine string
	if m.state.ErrorMessage != "" {
		statusLine = errorStyle.Render("  ✗ " + m.state.ErrorMessage)
	} else if m.state.StatusMessage != "" {
		statusLine = statusStyle.Render("  ✓ " + m.state.StatusMessage)
	} else if m.state.OfflineMode {
		statusLine = dimStyle.Render("  [Offline Mode - Values unavailable]")
	} else {
		// Show empty line to match browse view when there's no status
		statusLine = dimStyle.Render("  ")
	}
	output = append(output, statusLine)

	// Help (matching browse view structure - same formatting as browse)
	help := m.renderDescribeHelp()
	output = append(output, helpStyle.Render("  "+help+"  "))

	view := strings.Join(output, "\n")

	// Overlay label input dialog if active
	if m.state.LabelInputActive {
		dialog := renderLabelInput(m)
		return centerDialog(dialog, m.state.Width, m.state.Height)
	}

	return view
}

// renderVersionHistoryPanel renders the version history panel
func (m Model) renderVersionHistoryPanel(width, height int) string {
	var lines []string

	// Header
	header := labelStyle.Render("VERSION HISTORY")
	lines = append(lines, header)
	lines = append(lines, "")

	if len(m.state.DescribeHistory) == 0 {
		lines = append(lines, dimStyle.Render("Loading..."))
	} else {
		// Calculate how many items fit in the panel
		maxVisible := height - 4 // Subtract header and padding
		if maxVisible < 1 {
			maxVisible = 1
		}

		start := m.state.HistoryScrollOffset
		end := start + maxVisible
		if end > len(m.state.DescribeHistory) {
			end = len(m.state.DescribeHistory)
		}

		for i := start; i < end; i++ {
			h := m.state.DescribeHistory[i]
			versionStr := fmt.Sprintf("v%d - %s", h.Version, h.Modified)

			// Add label badges if present
			labelBadges := ""
			if len(h.Labels) > 0 {
				labelBadges = " " + renderInlineBadges(h.Labels)
			}

			displayStr := versionStr + labelBadges

			if i == m.state.HistoryIndex {
				lines = append(lines, selectedStyle.Render("▸ "+displayStr))
			} else {
				lines = append(lines, normalStyle.Render("  "+displayStr))
			}
		}
	}

	// Pad to fill panel height
	for len(lines) < height-2 {
		lines = append(lines, "")
	}

	// Add scroll indicator at bottom if needed
	if len(m.state.DescribeHistory) > 0 {
		scrollInfo := fmt.Sprintf("%d/%d", m.state.HistoryIndex+1, len(m.state.DescribeHistory))
		lines = append(lines, dimStyle.Render(scrollInfo))
	} else {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	// Flat style without borders
	panelStyle := lipgloss.NewStyle().
		MarginLeft(2).
		Width(width).
		Height(height)

	return panelStyle.Render(content)
}

// renderValuePanel renders the value panel
func (m Model) renderValuePanel(width, height int) string {
	var lines []string

	// Header
	header := labelStyle.Render("VALUE")
	if m.state.DescribeMasked {
		header += " " + dimStyle.Render("(masked)")
	}
	lines = append(lines, header)

	// Show labels for current version if available
	if len(m.state.DescribeHistory) > 0 && m.state.HistoryIndex < len(m.state.DescribeHistory) {
		currentLabels := m.state.DescribeHistory[m.state.HistoryIndex].Labels
		if len(currentLabels) > 0 {
			labelLine := renderFullLabels(currentLabels)
			lines = append(lines, labelLine)
		}
	}

	lines = append(lines, "")

	if m.state.DescribeValue == "" {
		lines = append(lines, dimStyle.Render("Loading..."))
	} else {
		value := m.state.DescribeValue
		if m.state.DescribeMasked {
			value = util.MaskValue(value)
		}

		// Split value into lines
		valueLines := strings.Split(value, "\n")

		// Calculate available lines for content
		availableLines := height - 5 // Subtract header, padding, scroll indicator
		if availableLines < 1 {
			availableLines = 1
		}

		start := m.state.ValueScrollOffset
		end := start + availableLines
		if start >= len(valueLines) {
			start = len(valueLines) - 1
			if start < 0 {
				start = 0
			}
			m.state.ValueScrollOffset = start
		}
		if end > len(valueLines) {
			end = len(valueLines)
		}

		// Render visible lines with wrapping or horizontal scrolling
		contentWidth := width - 4 // Account for border and padding
		for i := start; i < end; i++ {
			line := valueLines[i]

			if m.state.ValueLineWrap {
				// Wrap mode: split long lines to fit width
				if len(line) <= contentWidth {
					if m.state.DescribeMasked {
						lines = append(lines, maskedStyle.Render(line))
					} else {
						lines = append(lines, valueStyle.Render(line))
					}
				} else {
					// Wrap at contentWidth boundaries
					for pos := 0; pos < len(line); pos += contentWidth {
						endPos := pos + contentWidth
						if endPos > len(line) {
							endPos = len(line)
						}
						chunk := line[pos:endPos]
						if m.state.DescribeMasked {
							lines = append(lines, maskedStyle.Render(chunk))
						} else {
							lines = append(lines, valueStyle.Render(chunk))
						}
					}
				}
			} else {
				// Scroll mode: apply horizontal offset
				if len(line) > contentWidth {
					hscroll := m.state.ValueHorizontalScroll
					if hscroll < len(line) {
						endPos := hscroll + contentWidth
						if endPos > len(line) {
							endPos = len(line)
						}
						visiblePart := line[hscroll:endPos]
						if m.state.DescribeMasked {
							lines = append(lines, maskedStyle.Render(visiblePart))
						} else {
							lines = append(lines, valueStyle.Render(visiblePart))
						}
					} else {
						lines = append(lines, "")
					}
				} else {
					if m.state.DescribeMasked {
						lines = append(lines, maskedStyle.Render(line))
					} else {
						lines = append(lines, valueStyle.Render(line))
					}
				}
			}
		}
	}

	// Pad to fill panel height
	for len(lines) < height-2 {
		lines = append(lines, "")
	}

	// Add scroll indicator at bottom if needed
	if m.state.DescribeValue != "" {
		valueLines := strings.Split(m.state.DescribeValue, "\n")
		if len(valueLines) > 1 {
			scrollInfo := fmt.Sprintf("lines %d-%d/%d",
				m.state.ValueScrollOffset+1,
				min(m.state.ValueScrollOffset+height-5, len(valueLines)),
				len(valueLines))

			// Add horizontal scroll indicator if applicable
			if !m.state.ValueLineWrap && m.state.ValueHorizontalScroll > 0 {
				scrollInfo += fmt.Sprintf(" (col %d+)", m.state.ValueHorizontalScroll)
			}

			lines = append(lines, dimStyle.Render(scrollInfo))
		} else {
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	// Flat style without borders
	panelStyle := lipgloss.NewStyle().
		MarginRight(2).
		Width(width).
		Height(height)

	return panelStyle.Render(content)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderDescribeBox renders the parameter info as a compact header line
func (m Model) renderDescribeBox(entry *cache.CacheEntry) string {
	// Use the same layout as browse view data rows (not header)
	showModified := m.state.Width >= 100
	var nameWidth int
	var line string

	if showModified {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) + 3 (spaces) + 16 (MODIFIED) + 2 (padding) = 47
		nameWidth = m.state.Width - 47 - 2
		if nameWidth < 20 {
			nameWidth = 20
		}
		name := entry.Name
		if len(name) > nameWidth-2 {
			name = name[:nameWidth-2]
		}
		modifiedStr := entry.LastModifiedDate.Format("2006-01-02 15:04")
		// Format first without styling (same as browse view)
		line = fmt.Sprintf("  %-*s   %-12s   %8d   %16s  ",
			nameWidth,
			name,
			entry.Type,
			entry.Version,
			modifiedStr)
	} else {
		// Fixed: 3 (spaces) + 12 (TYPE) + 3 (spaces) + 8 (VERSION) = 26
		nameWidth = m.state.Width - 26 - 2
		if nameWidth < 20 {
			nameWidth = 20
		}
		name := entry.Name
		if len(name) > nameWidth-2 {
			name = name[:nameWidth-2]
		}
		// Format first without styling (same as browse view)
		line = fmt.Sprintf("  %-*s   %-12s   %8d",
			nameWidth,
			name,
			entry.Type,
			entry.Version)
	}

	// Apply styling to the whole line (like browse view does for normal rows)
	info := normalStyle.Render(line)

	// Add tags on second line if present
	if len(entry.Tags) > 0 {
		var tagPairs []string
		for k, v := range entry.Tags {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
		}
		tagsLine := dimStyle.Render("  Tags: " + strings.Join(tagPairs, ", "))
		return info + "\n" + tagsLine
	}

	return info
}

// renderConfirmDialog renders the confirmation dialog overlay
func (m Model) renderConfirmDialog() string {
	var b strings.Builder

	action := m.state.Confirm.Action
	if action == "delete" {
		b.WriteString(warningStyle.Render("⚠ CONFIRM DELETE"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("You are about to delete:\n%s\n\n", m.state.Confirm.Target))
		b.WriteString(warningStyle.Render("This action cannot be undone!"))
		b.WriteString("\n\n")
		b.WriteString(promptStyle.Render("Type 'delete me' to confirm: "))
		b.WriteString(inputStyle.Render(m.state.Confirm.Input))
	} else if action == "move" {
		b.WriteString(warningStyle.Render("MOVE/RENAME PARAMETER"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("From: %s\n\n", m.state.Confirm.Target))
		b.WriteString(promptStyle.Render("To: "))
		b.WriteString(inputStyle.Render(m.state.Confirm.Input))
	} else if action == "copy" {
		b.WriteString(warningStyle.Render("COPY PARAMETER"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("From: %s\n\n", m.state.Confirm.Target))
		b.WriteString(promptStyle.Render("To: "))
		b.WriteString(inputStyle.Render(m.state.Confirm.Input))
	}

	b.WriteString("█") // cursor

	if m.state.Confirm.ErrorMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(m.state.Confirm.ErrorMsg))
	}

	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Press Esc to cancel"))

	return dialogStyle.Render(b.String())
}

// truncateString truncates a string to max length with ellipsis
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// centerDialog centers a dialog on the screen
func centerDialog(content string, width, height int) string {
	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		content,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("236")),
	)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// styleLabelBadge returns a consistent style for a label badge based on label name hash
// Same label will always have the same color across all parameters
func styleLabelBadge(label string) lipgloss.Style {
	// Color palette for labels (distinct, readable colors)
	colors := []struct {
		bg string
		fg string
	}{
		{"34", "230"},  // green
		{"214", "0"},   // orange
		{"33", "230"},  // blue
		{"198", "230"}, // pink
		{"226", "0"},   // yellow
		{"51", "0"},    // cyan
		{"141", "230"}, // purple
		{"208", "0"},   // dark orange
	}

	// Simple hash function for consistent color selection
	hash := 0
	for _, c := range label {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	colorIdx := hash % len(colors)

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors[colorIdx].fg)).
		Background(lipgloss.Color(colors[colorIdx].bg)).
		Padding(0, 1).
		Bold(true)
}

// renderInlineBadges renders labels as compact badges for version history (max 2, truncated)
func renderInlineBadges(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	var badges []string
	count := len(labels)
	if count > MaxInlineLabels {
		count = MaxInlineLabels
	}

	for i := 0; i < count; i++ {
		label := labels[i]
		if len(label) > MaxLabelBadgeWidth {
			label = label[:MaxLabelBadgeWidth-1] + "…"
		}
		badge := styleLabelBadge(labels[i]).Render(label)
		badges = append(badges, badge)
	}

	result := strings.Join(badges, " ")
	if len(labels) > MaxInlineLabels {
		overflow := fmt.Sprintf(" +%d", len(labels)-MaxInlineLabels)
		result += dimStyle.Render(overflow)
	}

	return result
}

// renderFullLabels renders all labels as badges for the value panel header
func renderFullLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	var badges []string
	for _, label := range labels {
		badge := styleLabelBadge(label).Render(label)
		badges = append(badges, badge)
	}

	return strings.Join(badges, " ")
}

// renderLabelInput renders the label input dialog
func renderLabelInput(m Model) string {
	var b strings.Builder

	// Title based on action
	var title string
	switch m.state.LabelAction {
	case "add":
		title = "ADD LABEL"
	case "remove":
		title = "REMOVE LABEL"
	case "move":
		title = "MOVE LABEL"
	default:
		title = "LABEL ACTION"
	}
	b.WriteString(labelStyle.Render(title))
	b.WriteString("\n\n")

	// Version context
	if len(m.state.DescribeHistory) > 0 && m.state.HistoryIndex < len(m.state.DescribeHistory) {
		currentVersion := m.state.DescribeHistory[m.state.HistoryIndex].Version
		b.WriteString(fmt.Sprintf("Version: %d\n", currentVersion))
		b.WriteString(fmt.Sprintf("Parameter: %s\n\n", m.state.DescribeEntry.Name))
	}

	// Input field
	b.WriteString(promptStyle.Render("Label: "))
	b.WriteString(inputStyle.Render(m.state.LabelInput))
	b.WriteString("█") // cursor
	b.WriteString("\n\n")

	// Suggestions
	if len(m.state.LabelSuggestions) > 0 {
		b.WriteString(dimStyle.Render("Suggestions (Tab to cycle):\n"))
		for i, suggestion := range m.state.LabelSuggestions {
			if i == m.state.LabelSuggestionIndex {
				b.WriteString(selectedStyle.Render("▸ " + suggestion))
			} else {
				b.WriteString(dimStyle.Render("  " + suggestion))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Error message
	if m.state.LabelError != "" {
		b.WriteString(errorStyle.Render(m.state.LabelError))
		b.WriteString("\n\n")
	}

	// Help text
	b.WriteString(dimStyle.Render("Enter:submit  Tab:next  Esc:cancel"))

	return dialogStyle.Render(b.String())
}

// renderDescribeHelp returns the help text for describe view
func (m Model) renderDescribeHelp() string {
	return "x:mask  c:copy  e:edit  tab/⇧tab:version  g:latest  a:add-label  r:remove-label  m:move-label  ↑↓:scroll  ←→:horiz  w:wrap  esc:back  q:quit"
}
