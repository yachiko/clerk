package ui

import (
	"fmt"
	"strings"

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
			BorderForeground(lipgloss.Color("62"))

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
		lines = append(lines, searchStyle.Render("🔍 ")+m.searchInput.View())
	} else if m.state.SearchQuery != "" {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("Filter: %s (/ to edit)", m.state.SearchQuery)))
	} else {
		lines = append(lines, "")
	}

	// Header
	header := headerStyle.Render(fmt.Sprintf("%-60s %-12s %8s", "NAME", "TYPE", "VERSION"))
	lines = append(lines, header)
	lines = append(lines, strings.Repeat("─", m.state.Width-2))

	// Items - calculate how many we can show
	visible := m.visibleRows()
	if visible < 1 {
		visible = 10
	}

	if len(m.state.FilteredItems) == 0 {
		lines = append(lines, dimStyle.Render("  No parameters found"))
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
			m.renderTreeItems(b, start, end)
			itemLines = strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		} else {
			b := &strings.Builder{}
			m.renderListItems(b, start, end)
			itemLines = strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		}
		lines = append(lines, itemLines...)

		// Pad with empty lines to fill space
		for len(lines) < m.state.Height-2 {
			lines = append(lines, "")
		}
	}

	// Footer (always at bottom)
	lines = append(lines, strings.Repeat("─", m.state.Width-2))

	// Status/Error message
	var statusLine string
	if m.state.ErrorMessage != "" {
		statusLine = errorStyle.Render("✗ " + m.state.ErrorMessage)
	} else if m.state.StatusMessage != "" {
		statusLine = statusStyle.Render("✓ " + m.state.StatusMessage)
	} else {
		// Stats
		stats := fmt.Sprintf("%d/%d parameters", len(m.state.FilteredItems), len(m.state.Entries))
		statusLine = dimStyle.Render(stats)
	}
	lines = append(lines, statusLine)

	// Help line
	help := "↑↓:navigate  d:describe  c:copy  t:tree  /:search  q:quit"
	lines = append(lines, helpStyle.Render(help))

	return strings.Join(lines, "\n")
}

// renderListItems renders items in flat list view
func (m Model) renderListItems(b *strings.Builder, start, end int) {
	for i := start; i < end; i++ {
		entry := m.state.FilteredItems[i]

		name := truncateString(entry.Name, 58)
		line := fmt.Sprintf("%-60s %-12s %8d", name, entry.Type, entry.Version)

		if i == m.state.SelectedIndex {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}
}

// renderTreeItems renders items in tree view
func (m Model) renderTreeItems(b *strings.Builder, start, end int) {
	if len(m.state.TreeNodes) == 0 {
		return
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
			line = fmt.Sprintf("%s%s%-50s", indent, prefix, dirName)
		} else {
			entry := node.Entry
			name := truncateString(node.Name, 48-node.Depth*2)
			line = fmt.Sprintf("%s%s%-50s %-12s %8d", indent, prefix, name, entry.Type, entry.Version)
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
	output = append(output, "")

	// Parameter info box (fixed at top, full width)
	box := m.renderDescribeBox(entry)
	output = append(output, box)
	output = append(output, "")

	// Calculate panel dimensions
	leftWidth := 35                             // Version history panel width
	rightWidth := m.state.Width - leftWidth - 4 // Value panel width (minus spacing)
	if rightWidth < 40 {
		rightWidth = 40
	}

	// Calculate available height for panels
	// Title(1) + empty(1) + box(~7) + empty(1) + panels + empty(1) + status(1) + help(1) = Height
	panelHeight := m.state.Height - 13
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
	output = append(output, "")

	// Status/Error message
	var statusLine string
	if m.state.ErrorMessage != "" {
		statusLine = errorStyle.Render("✗ " + m.state.ErrorMessage)
	} else if m.state.StatusMessage != "" {
		statusLine = statusStyle.Render("✓ " + m.state.StatusMessage)
	} else {
		statusLine = ""
	}
	output = append(output, statusLine)

	// Help
	help := "x:toggle-mask  c:copy  ↑↓:version  PgUp/PgDn:scroll-value  esc:back  q:quit"
	output = append(output, helpStyle.Render(help))

	return strings.Join(output, "\n")
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
			if i == m.state.HistoryIndex {
				lines = append(lines, selectedStyle.Render("▸ "+versionStr))
			} else {
				lines = append(lines, normalStyle.Render("  "+versionStr))
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

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
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

		// Render visible lines with proper wrapping for width
		contentWidth := width - 4 // Account for border and padding
		for i := start; i < end; i++ {
			line := valueLines[i]
			// Wrap long lines
			if len(line) > contentWidth {
				line = line[:contentWidth-3] + "..."
			}
			if m.state.DescribeMasked {
				lines = append(lines, maskedStyle.Render(line))
			} else {
				lines = append(lines, valueStyle.Render(line))
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
			lines = append(lines, dimStyle.Render(scrollInfo))
		} else {
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
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

// renderDescribeBox renders the parameter info box
func (m Model) renderDescribeBox(entry *cache.CacheEntry) string {
	var lines []string

	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Name:"), entry.Name))
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Type:"), entry.Type))
	lines = append(lines, fmt.Sprintf("%s %d", labelStyle.Render("Version:"), entry.Version))
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Modified:"), entry.LastModifiedDate.Format("2006-01-02 15:04:05")))

	if len(entry.Tags) > 0 {
		var tagPairs []string
		for k, v := range entry.Tags {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
		}
		lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Tags:"), strings.Join(tagPairs, ", ")))
	}

	content := strings.Join(lines, "\n")
	// Use full width minus a small margin
	boxWidth := m.state.Width - 4
	if boxWidth < 60 {
		boxWidth = 60
	}
	return borderStyle.Width(boxWidth).Render(content)
}

// renderConfirmDialog renders the confirmation dialog overlay
func (m Model) renderConfirmDialog() string {
	var b strings.Builder

	b.WriteString(warningStyle.Render("⚠ CONFIRM DELETE"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("You are about to delete:\n%s\n\n", m.state.Confirm.Target))
	b.WriteString(warningStyle.Render("This action cannot be undone!"))
	b.WriteString("\n\n")
	b.WriteString(promptStyle.Render(fmt.Sprintf("Type '%s' to confirm: ", m.state.Confirm.ConfirmText)))
	b.WriteString(inputStyle.Render(m.state.Confirm.Input))
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
