# Task 13: Browse UI - Views and Rendering

## Objective
Implement the rendering logic for browse view (list and tree), describe view, and status bar.

## Prerequisites
- Task 12 completed (browse UI core framework)

## Deliverables

### 1. Create View Rendering

Create file `internal/ui/views.go`:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
)

// renderBrowseView renders the main browse view
func (m Model) renderBrowseView() string {
	var b strings.Builder

	// Title bar
	mode := "LIST"
	if m.state.Mode == ViewModeTree {
		mode = "TREE"
	}
	title := titleStyle.Render(fmt.Sprintf(" CLERK - %s ", mode))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Search bar
	if m.state.SearchActive {
		b.WriteString(searchStyle.Render("🔍 "))
		b.WriteString(m.searchInput.View())
	} else if m.state.SearchQuery != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Filter: %s (/ to edit)", m.state.SearchQuery)))
	}
	b.WriteString("\n\n")

	// Header
	header := headerStyle.Render(fmt.Sprintf("%-60s %-12s %8s", "NAME", "TYPE", "VERSION"))
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.state.Width-2))
	b.WriteString("\n")

	// Items
	visible := m.visibleRows()
	if visible < 1 {
		visible = 10
	}

	if len(m.state.FilteredItems) == 0 {
		b.WriteString(dimStyle.Render("\n  No parameters found\n"))
	} else {
		start := m.state.ScrollOffset
		end := start + visible
		if end > len(m.state.FilteredItems) {
			end = len(m.state.FilteredItems)
		}

		if m.state.Mode == ViewModeTree {
			m.renderTreeItems(&b, start, end)
		} else {
			m.renderListItems(&b, start, end)
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.state.Width-2))
	b.WriteString("\n")

	// Status/Error message
	if m.state.ErrorMessage != "" {
		b.WriteString(errorStyle.Render("✗ " + m.state.ErrorMessage))
	} else if m.state.StatusMessage != "" {
		b.WriteString(statusStyle.Render("✓ " + m.state.StatusMessage))
	} else {
		// Stats
		stats := fmt.Sprintf("%d/%d parameters", len(m.state.FilteredItems), len(m.state.Entries))
		b.WriteString(dimStyle.Render(stats))
	}
	b.WriteString("\n")

	// Help line
	help := "↑↓:navigate  d:describe  c:copy  t:tree  /:search  q:quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
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
	var b strings.Builder

	if m.state.DescribeEntry == nil {
		return "No entry selected"
	}

	entry := m.state.DescribeEntry

	// Title
	title := titleStyle.Render(" DESCRIBE ")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Parameter info box
	box := m.renderDescribeBox(entry)
	b.WriteString(box)
	b.WriteString("\n\n")

	// Value section
	b.WriteString(labelStyle.Render("VALUE"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")

	if m.state.DescribeValue == "" {
		b.WriteString(dimStyle.Render("Loading..."))
	} else {
		value := m.state.DescribeValue
		if m.state.DescribeMasked {
			value = util.MaskValue(value)
			b.WriteString(maskedStyle.Render(value))
		} else {
			b.WriteString(valueStyle.Render(value))
		}
	}
	b.WriteString("\n\n")

	// Version history
	b.WriteString(labelStyle.Render("VERSION HISTORY"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")

	if len(m.state.DescribeHistory) == 0 {
		b.WriteString(dimStyle.Render("Loading..."))
	} else {
		for i, h := range m.state.DescribeHistory {
			versionStr := fmt.Sprintf("v%d - %s", h.Version, h.Modified)
			if i == m.state.HistoryIndex {
				b.WriteString(versionSelectedStyle.Render("▸ " + versionStr))
			} else {
				b.WriteString(versionNormalStyle.Render("  " + versionStr))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Status/Error message
	if m.state.ErrorMessage != "" {
		b.WriteString(errorStyle.Render("✗ " + m.state.ErrorMessage))
	} else if m.state.StatusMessage != "" {
		b.WriteString(statusStyle.Render("✓ " + m.state.StatusMessage))
	}
	b.WriteString("\n\n")

	// Help
	help := "x:toggle-mask  c:copy  ↑↓:version  esc:back  q:quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
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
	return borderStyle.Width(60).Render(content)
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
```

### 2. Create Tree Builder

Create file `internal/ui/tree.go`:

```go
package ui

import (
	"sort"
	"strings"

	"github.com/yachiko/clerk/internal/cache"
)

// buildTreeNodes builds a tree structure from cache entries
func buildTreeNodes(entries []cache.CacheEntry, expanded map[string]bool) []TreeNode {
	if len(entries) == 0 {
		return nil
	}

	// Build directory structure
	type dirInfo struct {
		children map[string]*dirInfo
		entries  []*cache.CacheEntry
	}

	root := &dirInfo{children: make(map[string]*dirInfo)}

	for i := range entries {
		entry := &entries[i]
		parts := strings.Split(strings.TrimPrefix(entry.Name, "/"), "/")
		
		current := root
		for j, part := range parts {
			if j == len(parts)-1 {
				// This is the leaf (actual parameter)
				current.entries = append(current.entries, entry)
			} else {
				// This is a directory
				if current.children[part] == nil {
					current.children[part] = &dirInfo{children: make(map[string]*dirInfo)}
				}
				current = current.children[part]
			}
		}
	}

	// Flatten to nodes
	var nodes []TreeNode
	buildNodesRecursive(root, "", 0, expanded, &nodes)

	return nodes
}

func buildNodesRecursive(dir *dirInfo, path string, depth int, expanded map[string]bool, nodes *[]TreeNode) {
	// Sort directory names
	dirNames := make([]string, 0, len(dir.children))
	for name := range dir.children {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)

	// Sort entry names
	sort.Slice(dir.entries, func(i, j int) bool {
		return dir.entries[i].Name < dir.entries[j].Name
	})

	// Process directories first
	for _, name := range dirNames {
		child := dir.children[name]
		childPath := path + "/" + name
		
		isExpanded := expanded[childPath]
		childCount := countEntries(child)

		*nodes = append(*nodes, TreeNode{
			Name:       name,
			Path:       childPath,
			IsDir:      true,
			Depth:      depth,
			Expanded:   isExpanded,
			ChildCount: childCount,
		})

		if isExpanded {
			buildNodesRecursive(child, childPath, depth+1, expanded, nodes)
		}
	}

	// Process entries
	for _, entry := range dir.entries {
		// Get just the name part
		parts := strings.Split(entry.Name, "/")
		name := parts[len(parts)-1]

		*nodes = append(*nodes, TreeNode{
			Name:  name,
			Path:  entry.Name,
			IsDir: false,
			Depth: depth,
			Entry: entry,
		})
	}
}

func countEntries(dir *dirInfo) int {
	count := len(dir.entries)
	for _, child := range dir.children {
		count += countEntries(child)
	}
	return count
}
```

### 3. Add Lipgloss Dependency

Update `go.mod` to include:
```bash
go get github.com/charmbracelet/lipgloss@latest
```

## Acceptance Criteria

- [ ] Browse view shows header with NAME, TYPE, VERSION columns
- [ ] Selected item is highlighted
- [ ] Scrolling works correctly
- [ ] Search bar shows when active
- [ ] Filter indicator shows when search is set
- [ ] Stats show filtered/total count
- [ ] Help line shows available shortcuts
- [ ] Status messages display and auto-clear
- [ ] Error messages display in red
- [ ] Tree view shows directory structure with expand/collapse
- [ ] Tree indentation is correct
- [ ] Describe view shows parameter metadata in box
- [ ] Describe view shows value (masked by default)
- [ ] Describe view shows version history
- [ ] Version navigation highlights current version
- [ ] Long names are truncated with ellipsis

## Visual Examples

### Browse View (List)
```
 CLERK - LIST 

Filter: /dev/* (/ to edit)

NAME                                                         TYPE         VERSION
──────────────────────────────────────────────────────────────────────────────────
▶ /dev/api_key                                              SecureString       3
  /dev/db_password                                          SecureString       5
  /dev/config                                               String             2
  /prod/api_key                                             SecureString       1

──────────────────────────────────────────────────────────────────────────────────
4/15 parameters
↑↓:navigate  d:describe  c:copy  t:tree  /:search  q:quit
```

### Browse View (Tree)
```
 CLERK - TREE 

NAME                                                         TYPE         VERSION
──────────────────────────────────────────────────────────────────────────────────
▼ dev/ (3)
    api_key                                                 SecureString       3
    db_password                                             SecureString       5
    config                                                  String             2
▶ prod/ (1)

──────────────────────────────────────────────────────────────────────────────────
4/15 parameters
↑↓:navigate  d:describe  c:copy  t:tree  /:search  q:quit
```

### Describe View
```
 DESCRIBE 

╭──────────────────────────────────────────────────────────╮
│ Name: /dev/db_password                                   │
│ Type: SecureString                                       │
│ Version: 5                                               │
│ Modified: 2026-01-02 10:30:00                           │
│ Tags: env=dev, team=backend                             │
╰──────────────────────────────────────────────────────────╯

VALUE
────────────────────────────────────────
my**********23

VERSION HISTORY
────────────────────────────────────────
▸ v5 - 2026-01-02T10:30:00Z
  v4 - 2026-01-01T15:00:00Z
  v3 - 2025-12-28T09:00:00Z

x:toggle-mask  c:copy  ↑↓:version  esc:back  q:quit
```

## Notes

- `lipgloss` provides consistent styling across terminals
- Colors use ANSI 256-color codes for broad compatibility
- Box drawing uses Unicode rounded corners
- Tree expand/collapse state is preserved when toggling views
- Version history loads 3 versions initially
- Scrolling is handled separately from selection
