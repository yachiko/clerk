# Task 19: Parameter Version Labels

## Objective
Implement AWS SSM Parameter Store label management within the describe view, allowing users to view, add, move, and remove labels from parameter versions.

## Prerequisites
- Task 12 completed (browse UI core)
- Task 13 completed (browse UI views)
- Task 14 completed (browse UI actions)
- Task 03 completed (AWS SSM client)

## Background

AWS Systems Manager Parameter Store supports **labels** on parameter versions. Labels are string aliases that can be attached to specific versions, enabling:
- Semantic versioning (e.g., `prod`, `staging`, `dev`)
- Rollback markers (e.g., `last-known-good`, `rollback-point`)
- Deployment tracking (e.g., `deployed-2024-01-15`, `release-v2.3.1`)

### AWS SSM Label Constraints

| Constraint                       | Limit                                             |
| -------------------------------- | ------------------------------------------------- |
| Max labels per parameter version | 10                                                |
| Max label length                 | 100 characters                                    |
| Allowed characters               | `a-zA-Z0-9_.-`                                    |
| Label uniqueness                 | Must be unique across ALL versions of a parameter |
| Reserved prefix                  | `aws:` (cannot be used)                           |
| Case sensitivity                 | Labels are case-sensitive                         |

### Key Behaviors
1. **Label Uniqueness**: A label can only exist on ONE version at a time. To move a label from v1 to v2, you must first remove it from v1.
2. **Atomic Move**: AWS provides `LabelParameterVersion` API that can move a label in one operation (removes from old version, adds to new).
3. **No Version Deletion**: Removing all labels from a version does NOT delete the version.
4. **History Impact**: Labels appear in `GetParameterHistory` response.

## Analysis & Recommendations

### Use Cases

1. **Environment Promotion**: Mark version 5 as `prod`, version 7 as `staging` - deployment scripts can reference `/app/config:prod` instead of version numbers.

2. **Rollback Safety**: Before deploying, label current prod version as `rollback-point`. If deployment fails, revert to that label.

3. **Audit Trail**: Label versions with deployment dates or release tags for compliance and debugging.

### Caveats & Edge Cases

| Caveat                                  | Impact           | Mitigation                                           |
| --------------------------------------- | ---------------- | ---------------------------------------------------- |
| Label already exists on another version | API error        | Show which version has the label, offer to move it   |
| Max 10 labels per version               | Cannot add more  | Show warning, suggest removing unused labels         |
| Network latency                         | UI feels slow    | Show loading indicator, optimistic updates           |
| Label on deleted parameter              | Orphaned state   | N/A - deleting parameter removes all versions/labels |
| Concurrent modifications                | Race condition   | Refresh history after label operations               |
| Long labels truncate in UI              | Poor readability | Show tooltip/full label on hover or selection        |

### UI/UX Recommendations

1. **Display Labels in Version History**: Show labels as colored badges next to each version.

3. **Keyboard Shortcuts**:
   - `a` - Add label to currently selected version
   - `r` - Remove label from version (with selection if multiple)
   - `m` - Move existing label to current version (shows label picker)

3. **Visual Feedback**:
   - Labels displayed as `[prod]` `[staging]` badges in distinct colors
   - Selected version's labels highlighted
   - Error states clearly indicated

4. **Input Validation**: 
   - Real-time validation as user types label name
   - Show character count (0/100)
   - Prevent submission of invalid labels

5. **Confirmation Dialogs**:
   - Moving a label: "Move label 'prod' from v3 to v5?"
   - Removing a label: "Remove label 'staging' from v5?"

## Command Specification

### Labels in Get Command (Enhancement)

```
clerk get "/app/config:prod"     # Get version with label "prod"
clerk get "/app/config@prod"     # Alternative syntax (@ already used for version numbers)
```

**Recommendation**: Use `:label` syntax to differentiate from `@version` (numeric).

### Labels in Describe View

#### Label Display Strategy

Since AWS allows up to 10 labels per version (each up to 100 chars), the version history column could easily overflow. The solution:

| Column | Display Strategy |
|--------|------------------|
| Version History (left) | Max 2 labels inline, truncated at 12 chars, `+N` indicator for more |
| Value Panel (right) | Full labels list for selected version |

This keeps the version column at a predictable width (~45-50 chars) while still showing all label details.

```
┌──────────────────────────────────────────────┬──────────────────────────────────────┐
│ VERSION HISTORY                              │ VALUE (v4)                           │
│ ──────────────────────────────────────────── │ ──────────────────────────────────── │
│   v5  2024-01-15 14:32  [prod] [current]     │ Labels: staging, qa-approved,        │
│ > v4  2024-01-10 09:15  [staging] +2         │         release-2.3.1                │
│   v3  2024-01-05 11:00  [rollback-p…]        │                                      │
│   v2  2023-12-20 16:45                       │ MySecretPassword123!                 │
│   v1  2023-12-15 10:30                       │                                      │
├──────────────────────────────────────────────┴──────────────────────────────────────┤
│ a:add label  r:remove label  m:move label  tab/shift+tab:version  g:latest  esc:back│
└─────────────────────────────────────────────────────────────────────────────────────┘

Legend:
  [staging] +2    = has 3 labels total, showing 1 + count of remaining
  [rollback-p…]   = label truncated (full: "rollback-point")
```

## Deliverables

### 1. Extend AWS Types

Update file `internal/aws/types.go`:

```go
// Add new types for label operations

// LabelParameterInput represents input for labeling a parameter version
type LabelParameterInput struct {
	Name    string
	Version int64
	Labels  []string
}

// LabelParameterOutput represents output from label operation
type LabelParameterOutput struct {
	InvalidLabels []string // Labels that couldn't be applied
	Version       int64
}

// UnlabelParameterInput represents input for removing labels
type UnlabelParameterInput struct {
	Name    string
	Version int64
	Labels  []string
}
```

### 2. Add Label Operations to SSM Client

Update file `internal/aws/ssm.go` - add the following methods:

```go
// LabelParameterVersion adds or moves labels to a parameter version
// If a label already exists on another version, it will be moved
func (c *Client) LabelParameterVersion(ctx context.Context, input *LabelParameterInput) (*LabelParameterOutput, error) {
	ssmInput := &ssm.LabelParameterVersionInput{
		Name:            aws.String(input.Name),
		ParameterVersion: aws.Int64(input.Version),
		Labels:          input.Labels,
	}

	output, err := c.ssm.LabelParameterVersion(ctx, ssmInput)
	if err != nil {
		return nil, fmt.Errorf("failed to label parameter version: %w", err)
	}

	return &LabelParameterOutput{
		InvalidLabels: output.InvalidLabels,
		Version:       input.Version,
	}, nil
}

// UnlabelParameterVersion removes labels from a parameter version
func (c *Client) UnlabelParameterVersion(ctx context.Context, input *UnlabelParameterInput) error {
	ssmInput := &ssm.UnlabelParameterVersionInput{
		Name:            aws.String(input.Name),
		ParameterVersion: aws.Int64(input.Version),
		Labels:          input.Labels,
	}

	_, err := c.ssm.UnlabelParameterVersion(ctx, ssmInput)
	if err != nil {
		return fmt.Errorf("failed to unlabel parameter version: %w", err)
	}

	return nil
}

// GetParameterByLabel retrieves a parameter version by label
func (c *Client) GetParameterByLabel(ctx context.Context, name, label string, withDecryption bool) (*Parameter, error) {
	labeledName := fmt.Sprintf("%s:%s", name, label)
	input := &ssm.GetParameterInput{
		Name:           aws.String(labeledName),
		WithDecryption: aws.Bool(withDecryption),
	}

	output, err := c.ssm.GetParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter by label: %w", err)
	}

	p := output.Parameter
	return &Parameter{
		Name:             aws.ToString(p.Name),
		Value:            aws.ToString(p.Value),
		Type:             string(p.Type),
		Version:          p.Version,
		LastModifiedDate: aws.ToTime(p.LastModifiedDate),
		ARN:              aws.ToString(p.ARN),
	}, nil
}

// FindLabelVersion finds which version has a specific label
func (c *Client) FindLabelVersion(ctx context.Context, name, label string) (int64, error) {
	history, err := c.GetParameterHistory(ctx, name, 50, false)
	if err != nil {
		return 0, err
	}

	for _, h := range history {
		for _, l := range h.Labels {
			if l == label {
				return h.Version, nil
			}
		}
	}

	return 0, fmt.Errorf("label %q not found on any version", label)
}
```

### 3. Add Label Validation Utility

Create file `internal/util/labels.go`:

```go
package util

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxLabelLength     = 100
	MaxLabelsPerVersion = 10
)

var (
	// labelRegex matches valid label characters: a-zA-Z0-9_.-
	labelRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	
	// reservedPrefix that cannot be used
	reservedPrefix = "aws:"
)

// ValidateLabel validates a label string
func ValidateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("label cannot be empty")
	}

	if len(label) > MaxLabelLength {
		return fmt.Errorf("label exceeds maximum length of %d characters", MaxLabelLength)
	}

	if strings.HasPrefix(strings.ToLower(label), reservedPrefix) {
		return fmt.Errorf("label cannot start with reserved prefix %q", reservedPrefix)
	}

	if !labelRegex.MatchString(label) {
		return fmt.Errorf("label contains invalid characters (allowed: a-zA-Z0-9_.-)")
	}

	return nil
}

// ValidateLabels validates a slice of labels
func ValidateLabels(labels []string) error {
	if len(labels) > MaxLabelsPerVersion {
		return fmt.Errorf("cannot add more than %d labels per version", MaxLabelsPerVersion)
	}

	seen := make(map[string]bool)
	for _, label := range labels {
		if err := ValidateLabel(label); err != nil {
			return err
		}
		if seen[label] {
			return fmt.Errorf("duplicate label: %s", label)
		}
		seen[label] = true
	}

	return nil
}

// SuggestLabels returns common label suggestions
func SuggestLabels() []string {
	return []string{
		"prod",
		"staging",
		"dev",
		"test",
		"current",
		"previous",
		"rollback-point",
		"last-known-good",
		"deprecated",
	}
}
```

### 4. Update UI Types for Label State

Update file `internal/ui/types.go`:

```go
// Add to State struct
type State struct {
	// ... existing fields ...

	// Label management state
	LabelInput       string   // Current label input text
	LabelInputActive bool     // Whether label input is active
	LabelAction      string   // "add", "remove", "move"
	LabelError       string   // Validation error for label input
	LabelSuggestions []string // Suggested labels for autocomplete
	LabelSuggestionIndex int  // Currently selected suggestion
	
	// For move operation - which label to move
	MoveLabelSource  string // Label being moved
	MoveLabelVersion int64  // Version to move label from
}

// Update HistoryEntry to include labels
type HistoryEntry struct {
	Version  int64
	Value    string
	Modified string
	Labels   []string // Add this field
}
```

### 5. Add Label Messages

Update file `internal/ui/model.go` - add new message types:

```go
// Label operation messages
type labelInputMsg struct {
	action string // "add", "remove", "move"
}

type labelCompleteMsg struct {
	action  string
	label   string
	version int64
	err     error
}

type labelValidationMsg struct {
	valid bool
	err   string
}
```

### 6. Add Label Key Handlers

Update file `internal/ui/model.go` - add to `handleDescribeKeys`:

```go
func (m Model) handleDescribeKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle label input mode first
	if m.state.LabelInputActive {
		return m.handleLabelInput(msg)
	}

	switch msg.String() {
	// ... existing cases ...

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
		if len(m.state.DescribeHistory) > 0 {
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
			// Collect all labels from all versions
			var allLabels []string
			for _, h := range m.state.DescribeHistory {
				allLabels = append(allLabels, h.Labels...)
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
	}

	return m, nil
}

// handleLabelInput handles input during label operations
func (m Model) handleLabelInput(msg tea.KeyMsg) (Model, tea.Cmd) {
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
		// For remove/move, use labels from history
		for _, h := range m.state.DescribeHistory {
			source = append(source, h.Labels...)
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

		paramName := m.state.DescribeEntry.Name

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
```

### 7. Handle Label Operation Results

Update file `internal/ui/model.go` - add to `Update` function:

```go
case labelCompleteMsg:
	if msg.err != nil {
		m.state.ErrorMessage = fmt.Sprintf("Label %s failed: %v", msg.action, msg.err)
		return m, clearErrorAfter(3 * time.Second)
	}

	// Refresh history to show updated labels
	m.state.StatusMessage = fmt.Sprintf("Label '%s' %sed on v%d", msg.label, msg.action, msg.version)
	
	// Trigger history refresh
	return m, m.refreshHistory()

// Add helper command to refresh history
func (m Model) refreshHistory() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		history, err := m.client.GetParameterHistory(ctx, m.state.DescribeEntry.Name, 50, false)
		if err != nil {
			return errorMsg(fmt.Sprintf("Failed to refresh history: %v", err))
		}

		return historyRefreshMsg{history: history}
	}
}

type historyRefreshMsg struct {
	history []aws.ParameterHistory
}

// Handle in Update:
case historyRefreshMsg:
	m.state.DescribeHistory = convertHistory(msg.history)
	return m, clearStatusAfter(2 * time.Second)
```

### 8. Update Describe View Rendering

Update file `internal/ui/views.go` - modify `renderDescribeView`:

```go
// Label display constants
const (
	MaxInlineLabels    = 2  // Max labels shown in version history column
	MaxLabelBadgeWidth = 12 // Truncate label text at this width
)

// Add label styles
var (
	labelBadgeStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("63")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1).
		MarginRight(1)

	labelProdStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("34")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1).
		MarginRight(1)

	labelStagingStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("214")).
		Foreground(lipgloss.Color("0")).
		Padding(0, 1).
		MarginRight(1)

	labelInputStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1).
		Width(40)

	suggestionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	suggestionSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230"))

	labelOverflowStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)
)

// styleLabelBadge applies the appropriate style to a label
func styleLabelBadge(label string) string {
	var style lipgloss.Style
	switch strings.ToLower(label) {
	case "prod", "production":
		style = labelProdStyle
	case "staging", "stage":
		style = labelStagingStyle
	default:
		style = labelBadgeStyle
	}
	return style.Render(label)
}

// renderInlineLabels renders truncated label badges for version history column
// Shows max 2 labels with truncation, plus "+N" indicator if more exist
func renderInlineLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	var badges []string
	showCount := len(labels)
	if showCount > MaxInlineLabels {
		showCount = MaxInlineLabels
	}

	for i := 0; i < showCount; i++ {
		label := labels[i]
		// Truncate long labels with ellipsis
		if len(label) > MaxLabelBadgeWidth {
			label = label[:MaxLabelBadgeWidth-1] + "…"
		}
		badges = append(badges, styleLabelBadge(label))
	}

	result := strings.Join(badges, "")

	// Show +N indicator if more labels exist
	if len(labels) > MaxInlineLabels {
		remaining := len(labels) - MaxInlineLabels
		result += labelOverflowStyle.Render(fmt.Sprintf(" +%d", remaining))
	}

	return result
}

// renderFullLabels renders all labels (for value panel)
func renderFullLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	var badges []string
	for _, label := range labels {
		badges = append(badges, styleLabelBadge(label))
	}

	return strings.Join(badges, "")
}

// In renderVersionHistory, update version line rendering to use truncated labels:
func (m Model) renderVersionHistory() string {
	var b strings.Builder
	
	for i, entry := range m.state.DescribeHistory {
		prefix := "  "
		style := versionNormalStyle
		if i == m.state.HistoryIndex {
			prefix = "> "
			style = versionSelectedStyle
		}

		// Format: > v5  2024-01-15 14:32  [prod] [stagi…] +1
		versionStr := fmt.Sprintf("v%d", entry.Version)
		dateStr := entry.Modified
		labelsStr := renderInlineLabels(entry.Labels) // Use truncated inline labels

		line := fmt.Sprintf("%s%-4s  %s  %s", prefix, versionStr, dateStr, labelsStr)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

// renderValuePanel renders the right panel with value and full labels
func (m Model) renderValuePanel() string {
	var b strings.Builder

	if len(m.state.DescribeHistory) == 0 {
		return dimStyle.Render("No version selected")
	}

	entry := m.state.DescribeHistory[m.state.HistoryIndex]

	// Header with version
	header := fmt.Sprintf("VALUE (v%d)", entry.Version)
	b.WriteString(labelStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 36))
	b.WriteString("\n")

	// Show ALL labels for selected version (full, not truncated)
	if len(entry.Labels) > 0 {
		b.WriteString(dimStyle.Render("Labels: "))
		b.WriteString(strings.Join(entry.Labels, ", "))
		b.WriteString("\n\n")
	}

	// Value content
	value := entry.Value
	if m.state.DescribeMasked {
		value = util.MaskValue(value)
	}
	b.WriteString(valueStyle.Render(value))

	return b.String()
}

// renderLabelInput renders the label input dialog
func (m Model) renderLabelInput() string {
	var b strings.Builder

	// Title based on action
	var title string
	switch m.state.LabelAction {
	case "add":
		title = "Add Label"
	case "remove":
		title = "Remove Label"
	case "move":
		title = "Move Label to v" + fmt.Sprintf("%d", m.state.DescribeHistory[m.state.HistoryIndex].Version)
	}

	b.WriteString(labelStyle.Render(title))
	b.WriteString("\n\n")

	// Input field
	inputDisplay := m.state.LabelInput
	if inputDisplay == "" {
		inputDisplay = dimStyle.Render("Type or select a label...")
	}
	b.WriteString(labelInputStyle.Render(inputDisplay))
	
	// Show character count for add
	if m.state.LabelAction == "add" {
		count := fmt.Sprintf(" %d/100", len(m.state.LabelInput))
		b.WriteString(dimStyle.Render(count))
	}
	b.WriteString("\n")

	// Error message
	if m.state.LabelError != "" {
		b.WriteString(errorStyle.Render("✗ " + m.state.LabelError))
		b.WriteString("\n")
	}

	// Suggestions
	if len(m.state.LabelSuggestions) > 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Suggestions (↑↓ to navigate, Enter to select):"))
		b.WriteString("\n")
		
		maxShow := 5
		if len(m.state.LabelSuggestions) < maxShow {
			maxShow = len(m.state.LabelSuggestions)
		}
		
		for i := 0; i < maxShow; i++ {
			style := suggestionStyle
			prefix := "  "
			if i == m.state.LabelSuggestionIndex {
				style = suggestionSelectedStyle
				prefix = "> "
			}
			b.WriteString(style.Render(prefix + m.state.LabelSuggestions[i]))
			b.WriteString("\n")
		}
		
		if len(m.state.LabelSuggestions) > maxShow {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ... and %d more", len(m.state.LabelSuggestions)-maxShow)))
		}
	}

	// Help
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Enter: confirm • Esc: cancel"))

	return borderStyle.Render(b.String())
}

// Update main describe render to show label input overlay
func (m Model) renderDescribeView() string {
	// ... existing render code ...

	// If label input is active, render as overlay
	if m.state.LabelInputActive {
		// Render label input dialog centered
		dialog := m.renderLabelInput()
		// Center the dialog
		return m.centerDialog(mainContent, dialog)
	}

	return mainContent
}

// Update help line in describe view to show label shortcuts
func (m Model) renderDescribeHelp() string {
	return helpStyle.Render("c:copy  e:edit  a:add label  r:remove label  m:move label  g:latest  ←→:scroll  esc:back")
}
```

### 9. Update History Entry Conversion

Update file `internal/ui/model.go` - ensure labels are included when converting history:

```go
// convertHistory converts AWS history to UI history entries
func convertHistory(awsHistory []aws.ParameterHistory) []HistoryEntry {
	var entries []HistoryEntry
	for _, h := range awsHistory {
		entries = append(entries, HistoryEntry{
			Version:  h.Version,
			Value:    h.Value,
			Modified: h.LastModifiedDate.Format("2006-01-02 15:04"),
			Labels:   h.Labels, // Include labels
		})
	}
	return entries
}
```

### 10. Enhance Get Command for Label Support

Update file `internal/cli/get.go`:

```go
func runGet(cmd *cobra.Command, args []string) error {
	// ... existing code ...

	name := args[0]
	var version int64 = -1
	var label string

	// Parse name for version or label
	// Syntax: /path/param@3 (version) or /path/param:prod (label)
	if idx := strings.LastIndex(name, "@"); idx != -1 {
		versionStr := name[idx+1:]
		name = name[:idx]
		
		if versionStr != "latest" {
			v, err := strconv.ParseInt(versionStr, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid version number: %s", versionStr)
			}
			version = v
		}
	} else if idx := strings.LastIndex(name, ":"); idx != -1 && !strings.HasPrefix(name[idx:], "://") {
		// Check it's not a URL scheme
		label = name[idx+1:]
		name = name[:idx]
	}

	// ... rest of function, use label if set ...
	
	var param *aws.Parameter
	var err error
	
	if label != "" {
		param, err = client.GetParameterByLabel(ctx, name, label, !getMask)
	} else if version > 0 {
		param, err = client.GetParameterByVersion(ctx, name, version, !getMask)
	} else {
		param, err = client.GetParameter(ctx, name, !getMask)
	}
	
	// ... rest of function ...
}
```

## Acceptance Criteria

- [ ] Labels are displayed as colored badges in version history (truncated)
- [ ] Version history shows max 2 labels inline, with `+N` indicator for overflow
- [ ] Long labels (>12 chars) are truncated with `…` in version column
- [ ] Value panel shows ALL labels for selected version (full, not truncated)
- [ ] `a` key opens label input for adding a label to selected version
- [ ] `r` key opens label removal dialog with version's labels
- [ ] `m` key opens move label dialog showing all labels across versions
- [ ] Real-time validation shows errors for invalid label input
- [ ] Suggestions appear and can be navigated with Tab/arrows
- [ ] Label operations update the display after completion
- [ ] Error messages are shown for failed operations
- [ ] `clerk get /param:label` syntax works for fetching by label
- [ ] Labels are correctly fetched and displayed from GetParameterHistory

## Example Usage

```bash
# In describe view:
# 1. Navigate to a version with Tab/Shift+Tab
# 2. Press 'a' to add a label
# 3. Type "prod" or select from suggestions
# 4. Press Enter to apply

# Get by label from CLI:
clerk get "/app/config:prod"
clerk get "/app/config:staging"

# Labels shown in version history (left column - truncated):
# v5  2024-01-15 14:32  [prod] [current]
# v4  2024-01-10 09:15  [staging] +2       ← 3 labels, showing 1 + overflow
# v3  2024-01-05 11:00  [rollback-p…]      ← truncated label

# Labels shown in value panel (right column - full):
# Labels: staging, qa-approved, release-2.3.1
```

## Testing Notes

### Manual Testing
1. Create a parameter with multiple versions
2. Add labels via AWS Console or CLI to verify display
3. Test add/remove/move operations
4. Test error cases (duplicate label, max labels, invalid chars)
5. Test label retrieval with `clerk get /param:label`

### Unit Tests (add to Task 17)
- Label validation utility tests
- Label badge rendering tests
- History entry conversion with labels

### Integration Tests (add to Task 18)
- Label operations against moto server
- Get by label functionality
- Label move atomicity

## Notes

1. **Performance**: Label operations are fast (single API call), but always refresh history after to ensure consistency.

2. **Concurrent Access**: If multiple users modify labels, refresh with `r` key to see latest state.

3. **Label Conventions**: Consider documenting team conventions for label naming (e.g., always use lowercase, environment names like `prod`/`staging`).

4. **AWS Limits**: The 10-label-per-version limit is rarely hit in practice, but UI should gracefully handle it.

5. **Label vs Tag**: Labels are version-specific aliases. Tags are key-value metadata on the parameter itself (not version-specific). Don't confuse them.
