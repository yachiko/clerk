package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yachiko/clerk/internal/cache"
)

func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	got, _ := m.Update(msg)
	return got.(Model)
}

func TestDescribeResultsRequireCurrentIdentityAndGeneration(t *testing.T) {
	m := Model{state: State{Mode: ViewModeDescribe, DescribeParamName: "/b", DescribeGeneration: 2}}
	m = updateModel(t, m, describeLoadedMsg{name: "/a", generation: 1, value: "a"})
	if m.state.DescribeValue != "" {
		t.Fatal("stale result changed the active detail")
	}
	m = updateModel(t, m, describeLoadedMsg{name: "/b", generation: 2, value: "b"})
	if m.state.DescribeValue != "b" {
		t.Fatal("current result was not accepted")
	}
	m = updateModel(t, m, versionValuesLoadedMsg{name: "/a", generation: 1, versions: map[int64]string{1: "wrong"}})
	if m.state.DescribeValue != "b" {
		t.Fatal("stale version result changed the active detail")
	}
}

func TestHistoryNavigationAndNarrowRenderingAreSafe(t *testing.T) {
	m := Model{state: State{Mode: ViewModeDescribe, Width: 0, Height: 0}}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.state.HistoryIndex != 0 {
		t.Fatal("empty history produced an invalid index")
	}
	_ = m.renderDescribeView()
	m.state.Mode = ViewModeList
	_ = m.renderBrowseView()
}

func TestTreeUsesVisibleRowsForNavigation(t *testing.T) {
	entries := []cache.CacheEntry{{Name: "/a/b/leaf", Type: "String"}}
	m := Model{state: State{Mode: ViewModeTree, FilteredItems: entries, ExpandedPaths: map[string]bool{"/a": true, "/a/b": true}, Height: 20}}
	m.buildTree()
	if len(m.state.TreeNodes) != 3 {
		t.Fatalf("got %d visible nodes, want 3", len(m.state.TreeNodes))
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.state.SelectedIndex != 1 {
		t.Fatalf("down selected %d, want directory row 1", m.state.SelectedIndex)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.state.SelectedIndex != 2 {
		t.Fatalf("end selected %d, want final tree row", m.state.SelectedIndex)
	}
}
