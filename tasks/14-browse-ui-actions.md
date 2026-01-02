# Task 14: Browse UI - Edit and Delete Actions

## Objective
Implement edit and delete functionality within the browse UI, including opening external editor and confirmation dialogs.

## Prerequisites
- Task 12 completed (browse UI core framework)
- Task 13 completed (browse UI views)
- Task 05 completed (utility modules - editor)

## Deliverables

### 1. Create Edit Functionality

Update file `internal/ui/model.go` to add edit handling:

```go
// Add to model.go - new message types
type editCompleteMsg struct {
	name     string
	newValue string
	version  int64
	err      error
}

// Add to handleBrowseKeys in model.go
case "e":
	// Edit selected item
	if len(m.state.FilteredItems) > 0 {
		entry := m.state.FilteredItems[m.state.SelectedIndex]
		return m, m.editSecret(entry.Name)
	}
	return m, nil

// Add edit command function
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
		if strings.HasPrefix(strings.TrimSpace(param.Value), "{") {
			ext = ".json"
		} else if strings.HasPrefix(strings.TrimSpace(param.Value), "<") {
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

// Add to Update function to handle edit result
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
```

### 2. Create Delete Functionality with Confirmation

Create file `internal/ui/confirm.go`:

```go
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmState represents confirmation dialog state
type ConfirmState struct {
	Active      bool
	Action      string // "delete"
	Target      string // parameter name
	ConfirmText string // text user must type
	Input       string // current input
	ErrorMsg    string
}

// deleteConfirmMsg signals delete confirmation requested
type deleteConfirmMsg struct {
	name string
}

// deleteCompleteMsg signals delete operation completed
type deleteCompleteMsg struct {
	name string
	err  error
}

// Add to State struct in types.go
// Confirm ConfirmState

// renderConfirmDialog renders the confirmation dialog overlay
func (m Model) renderConfirmDialog() string {
	if !m.state.Confirm.Active {
		return ""
	}

	var b strings.Builder

	// Dialog box style
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Width(50)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)

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

// handleConfirmKeys handles keyboard input during confirmation
func (m Model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state.Confirm = ConfirmState{}
		return m, nil

	case "enter":
		if m.state.Confirm.Input == m.state.Confirm.ConfirmText {
			// Confirmed - execute delete
			name := m.state.Confirm.Target
			m.state.Confirm = ConfirmState{}
			return m, m.deleteSecret(name)
		}
		m.state.Confirm.ErrorMsg = "Incorrect confirmation text"
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

// Add to handleBrowseKeys in model.go
// case "delete", "backspace":
//     if len(m.state.FilteredItems) > 0 {
//         entry := m.state.FilteredItems[m.state.SelectedIndex]
//         return m, m.initiateDelete(entry.Name)
//     }
//     return m, nil
```

### 3. Update Model to Handle Confirm State

Add to `internal/ui/model.go`:

```go
// Update handleKeyPress to check for confirm state first
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle confirmation dialog first
	if m.state.Confirm.Active {
		return m.handleConfirmKeys(msg)
	}

	// ... rest of existing handleKeyPress
}

// Add to Update function
case deleteCompleteMsg:
	if msg.err != nil {
		return m, func() tea.Msg { return errorMsg("Delete failed: " + msg.err.Error()) }
	}

	// Remove from local entries
	for i := range m.state.Entries {
		if m.state.Entries[i].Name == msg.name {
			m.state.Entries = append(m.state.Entries[:i], m.state.Entries[i+1:]...)
			break
		}
	}
	m.filterEntries()

	// Remove from cache
	_ = m.cache.Delete(msg.name)

	return m, func() tea.Msg {
		return statusMsg(fmt.Sprintf("Deleted %s", msg.name))
	}

// Update View to show confirm dialog overlay
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
		view = lipgloss.Place(
			m.state.Width, m.state.Height,
			lipgloss.Center, lipgloss.Center,
			dialog,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("236")),
		)
	}

	return view
}
```

### 4. Update Types

Update `internal/ui/types.go` to include confirm state:

```go
// State represents the UI state
type State struct {
	// ... existing fields ...

	// Confirmation dialog
	Confirm ConfirmState
}
```

## Acceptance Criteria

### Edit
- [ ] `e` key opens external editor with current value
- [ ] Editor uses $EDITOR environment variable
- [ ] File extension matches content type (json, xml, txt)
- [ ] No update if content unchanged
- [ ] Status message shows new version on success
- [ ] Error message on editor or update failure
- [ ] Temp file is securely deleted after edit
- [ ] Local cache is updated after edit

### Delete
- [ ] `delete`/`backspace` key initiates delete confirmation
- [ ] Confirmation dialog appears as overlay
- [ ] User must type parameter name (last segment) to confirm
- [ ] Incorrect text shows error message
- [ ] `Esc` cancels deletion
- [ ] Successful delete removes from list and cache
- [ ] Status message confirms deletion
- [ ] Error message on delete failure

## Visual Examples

### Delete Confirmation Dialog
```
╔══════════════════════════════════════════════════╗
║ ⚠ CONFIRM DELETE                                 ║
║                                                  ║
║ You are about to delete:                         ║
║ /dev/old_secret                                  ║
║                                                  ║
║ This action cannot be undone!                    ║
║                                                  ║
║ Type 'old_secret' to confirm: old_se█            ║
║                                                  ║
║ Press Esc to cancel                              ║
╚══════════════════════════════════════════════════╝
```

### After Successful Edit
```
✓ Updated /dev/db_password to version 6
```

### After Successful Delete
```
✓ Deleted /dev/old_secret
```

## Notes

- Edit temporarily exits alt screen to show editor
- After editor closes, UI returns to browse mode
- Delete confirmation prevents accidental deletions
- Confirmation text is case-sensitive
- Both operations update local state immediately for responsiveness
- Cache updates are best-effort (failure doesn't fail the operation)
- Consider adding undo functionality in future (restore from history)
