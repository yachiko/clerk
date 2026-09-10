package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

type fakeSecretsManager struct {
	metadata                            []aws.SecretMetadata
	versions                            []aws.SecretVersion
	value                               *aws.SecretDetail
	listErr, versionErr                 error
	valueErr                            error
	listCalls, versionCalls, valueCalls int
}

func (f *fakeSecretsManager) ListSecrets(context.Context) ([]aws.SecretMetadata, error) {
	f.listCalls++
	return f.metadata, f.listErr
}
func (f *fakeSecretsManager) ListSecretVersionIds(context.Context, string) ([]aws.SecretVersion, error) {
	f.versionCalls++
	return f.versions, f.versionErr
}
func (f *fakeSecretsManager) GetSecretValue(context.Context, string, aws.SecretValueSelector) (*aws.SecretDetail, error) {
	f.valueCalls++
	return f.value, f.valueErr
}

func resourceID(backend aws.Backend, canonical string) aws.ResourceIdentity {
	return aws.ResourceIdentity{Partition: "aws", AccountID: "123456789012", Region: "us-east-1", Backend: backend, CanonicalID: canonical}
}

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

func TestEqualNamesRemainQualifiedAndVisiblyLabeled(t *testing.T) {
	ssmID, smID := resourceID(aws.BackendSSM, "/shared"), resourceID(aws.BackendSecretsManager, "arn:shared")
	entries := []cache.CacheEntry{{Identity: ssmID, Name: "/shared", Type: "SecureString"}, {Identity: smID, Name: "/shared", Type: "Secret"}}
	m := Model{state: State{Mode: ViewModeList, FilteredItems: entries, Entries: entries, ExpandedPaths: map[string]bool{}, Width: 100, Height: 20}, scope: ssmID}
	list := m.renderBrowseView()
	if !strings.Contains(list, "[SSM] /shared") || !strings.Contains(list, "[SM] /shared") {
		t.Fatalf("backend labels missing from list:\n%s", list)
	}
	m.state.Mode = ViewModeTree
	m.buildTree()
	if len(m.state.TreeNodes) != 2 || m.state.TreeNodes[0].Identity == m.state.TreeNodes[1].Identity {
		t.Fatalf("tree conflated equal names: %#v", m.state.TreeNodes)
	}
	tree := m.renderBrowseView()
	if !strings.Contains(tree, "[SSM]") || !strings.Contains(tree, "[SM]") {
		t.Fatalf("backend labels missing from tree:\n%s", tree)
	}
}

func TestScopeAndBackendAreVisibleInDetailAndConfirmation(t *testing.T) {
	id := resourceID(aws.BackendSSM, "/prod/key")
	entry := cache.CacheEntry{Identity: id, Name: "/prod/key", Type: "SecureString"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, Width: 120, Height: 24}, scope: id}
	view := m.renderDescribeView()
	for _, text := range []string{"DESCRIBE SSM", id.AccountID, id.Region, "[SSM] /prod/key"} {
		if !strings.Contains(view, text) {
			t.Fatalf("detail missing %q", text)
		}
	}
	m.state.Confirm = ConfirmState{Active: true, Action: "delete", Target: entry.Name, Identity: id}
	if dialog := m.renderConfirmDialog(); !strings.Contains(dialog, "[SSM] /prod/key") {
		t.Fatalf("confirmation lacks backend label: %s", dialog)
	}
}

func TestCrossBackendDetailResponsesAreDiscarded(t *testing.T) {
	ssmID, smID := resourceID(aws.BackendSSM, "/same"), resourceID(aws.BackendSecretsManager, "arn:same")
	entry := cache.CacheEntry{Identity: smID, Name: "/same", Type: "Secret"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: smID, DescribeParamName: "/same", DescribeGeneration: 7}}
	m = updateModel(t, m, describeLoadedMsg{identity: ssmID, name: "/same", generation: 7, value: "wrong"})
	if m.state.DescribeValue != "" {
		t.Fatal("stale SSM response populated SM detail")
	}
	m = updateModel(t, m, secretValueLoadedMsg{identity: smID, generation: 6, value: aws.NewTextValue(smID, "wrong")})
	if m.state.DescribeValue != "" {
		t.Fatal("stale SM generation populated detail")
	}
	ssmEntry := cache.CacheEntry{Identity: ssmID, Name: "/same", Type: "SecureString"}
	m.state.DescribeEntry, m.state.DescribeIdentity = &ssmEntry, ssmID
	m = updateModel(t, m, secretValueLoadedMsg{identity: smID, generation: 7, value: aws.NewTextValue(smID, "wrong")})
	if m.state.DescribeValue != "" {
		t.Fatal("stale SM response populated SSM detail")
	}
}

func TestSecretsManagerDetailDoesNotFetchValueAndDenialPreservesMetadata(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	created := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	provider := &fakeSecretsManager{metadata: []aws.SecretMetadata{{Identity: id, Name: "shared", ARN: id.CanonicalID, Description: "metadata survives"}}, versions: []aws.SecretVersion{{VersionID: "opaque", VersionStages: []string{"AWSCURRENT"}, CreatedDate: &created}}}
	entry := cache.CacheEntry{Identity: id, Name: "shared", Type: "Secret"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeParamName: "shared", DescribeGeneration: 3, DescribeLoading: true}, secrets: provider, secretMetadata: map[aws.ResourceIdentity]aws.SecretMetadata{}, scope: id}
	msg := m.loadDescribe(id, entry.Name, 3)()
	if provider.valueCalls != 0 || provider.listCalls != 1 || provider.versionCalls != 1 {
		t.Fatalf("detail calls: list=%d versions=%d values=%d", provider.listCalls, provider.versionCalls, provider.valueCalls)
	}
	m = updateModel(t, m, msg)
	provider.valueErr = errors.New("access denied")
	m = updateModel(t, m, m.loadSelectedSecretValue(false)())
	if m.state.DescribeEntry == nil || m.secretMetadata[id].Description != "metadata survives" || m.state.DescribeValueError == "" {
		t.Fatal("value denial discarded metadata or was not localized")
	}
}

func TestSecretsManagerInventoryDoesNotFetchValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := resourceID(aws.BackendSecretsManager, "arn:inventory")
	provider := &fakeSecretsManager{metadata: []aws.SecretMetadata{{Identity: id, Name: "inventory", ARN: id.CanonicalID}}}
	cfg := &config.Config{ParallelFetches: 1}
	manager, err := cache.NewManagerForBackend(cfg, id.Partition, id.Region, id.AccountID, aws.BackendSecretsManager)
	if err != nil {
		t.Fatal(err)
	}
	m := NewCombinedModel(nil, provider, map[aws.Backend]*cache.Manager{aws.BackendSecretsManager: manager}, cfg, aws.BackendSecretsManager, id)
	msg := m.refreshInventories(context.Background())()
	if provider.valueCalls != 0 {
		t.Fatalf("inventory fetched %d values", provider.valueCalls)
	}
	result := msg.(inventoriesRefreshedMsg)
	if len(result.results) != 1 || len(result.results[0].entries) != 1 {
		t.Fatalf("unexpected inventory result: %#v", result)
	}
}

func TestPartialProviderFailureRetainsSuccessfulRows(t *testing.T) {
	id := resourceID(aws.BackendSSM, "/available")
	m := Model{state: State{Mode: ViewModeList, ExpandedPaths: map[string]bool{}}, backend: aws.BackendAll, caches: map[aws.Backend]*cache.Manager{}, secretMetadata: map[aws.ResourceIdentity]aws.SecretMetadata{}}
	m = updateModel(t, m, inventoriesRefreshedMsg{results: []inventoryProviderResult{{backend: aws.BackendSSM, entries: []cache.CacheEntry{{Identity: id, Name: "/available"}}}, {backend: aws.BackendSecretsManager, err: errors.New("denied")}}})
	if len(m.state.Entries) != 1 || m.state.Entries[0].Identity != id {
		t.Fatal("successful provider rows were discarded")
	}
	if !strings.Contains(m.state.ErrorMessage, "SM: denied") || !strings.Contains(m.state.ErrorMessage, "successful rows retained") {
		t.Fatalf("partial failure not visible: %q", m.state.ErrorMessage)
	}
}

func TestSecretsManagerBinaryValueIsBase64AndMutationsAreDisabled(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:binary")
	entry := cache.CacheEntry{Identity: id, Name: "binary", Type: "Secret"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeParamName: "binary", DescribeGeneration: 1, DescribeHistory: []HistoryEntry{{VersionID: "v1"}}, Width: 100, Height: 20}}
	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 1, versionID: "v1", value: aws.NewBinaryValue(id, []byte{0, 1, 2, 255})})
	if m.state.DescribeValue != "AAEC/w==" || m.state.DescribeValueKind != aws.ValueBinary {
		t.Fatalf("unsafe binary rendering: %q (%s)", m.state.DescribeValue, m.state.DescribeValueKind)
	}
	updated, cmd := m.handleDescribeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	blocked := updated.(Model)
	if cmd != nil || !strings.Contains(blocked.state.StatusMessage, "SM binary") || !strings.Contains(blocked.state.StatusMessage, "disabled") {
		t.Fatal("SM edit was not blocked before provider access")
	}
}

func TestOutOfOrderSecretVersionsDoNotReplaceSelectedValue(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:versions")
	entry := cache.CacheEntry{Identity: id, Name: "versions"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeGeneration: 4, DescribeHistory: []HistoryEntry{{VersionID: "new"}, {VersionID: "old"}}, HistoryIndex: 0}}

	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 4, requestedVersionID: "old", versionID: "old", value: aws.NewTextValue(id, "stale")})
	if m.state.DescribeValue != "" || !m.state.DescribeHistory[1].ValueLoaded {
		t.Fatal("older response replaced the selected version or was not retained in history")
	}
	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 4, requestedVersionID: "new", versionID: "new", value: aws.NewTextValue(id, "current")})
	if m.state.DescribeValue != "current" {
		t.Fatalf("selected version value = %q", m.state.DescribeValue)
	}
}

func TestEarlyCurrentRevealSelectsAWSCURRENTWhenPendingIsFirst(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:rotation")
	entry := cache.CacheEntry{Identity: id, Name: "rotation"}
	m := Model{state: State{
		Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id,
		DescribeGeneration: 5, DescribeLoading: true,
		DescribeHistory: []HistoryEntry{{VersionID: "pending", Labels: []string{"AWSPENDING"}}, {VersionID: "current", Labels: []string{"AWSCURRENT"}}},
	}}

	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 5, versionID: "current", value: aws.NewTextValue(id, "current value")})
	if m.state.HistoryIndex != 1 || m.state.DescribeValue != "current value" || m.state.DescribeLoading {
		t.Fatalf("history=%d value=%q loading=%v", m.state.HistoryIndex, m.state.DescribeValue, m.state.DescribeLoading)
	}
}

func TestRequestedVersionCopyCompletesAfterVersionNavigation(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:copy-version")
	entry := cache.CacheEntry{Identity: id, Name: "copy-version"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeGeneration: 3, DescribeSelectionChanged: true, DescribeHistory: []HistoryEntry{{VersionID: "new"}, {VersionID: "old"}}, HistoryIndex: 0}}

	updated, cmd := m.Update(secretValueLoadedMsg{identity: id, generation: 3, requestedVersionID: "old", versionID: "old", value: aws.NewTextValue(id, "copy me"), copy: true})
	m = updated.(Model)
	if cmd == nil || m.state.DescribeValue != "" || !m.state.DescribeHistory[1].ValueLoaded {
		t.Fatal("copy was dropped or changed the visible selected version")
	}
}

func TestSecretValueFailuresDoNotLeaveDetailLoading(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:value-error")
	entry := cache.CacheEntry{Identity: id, Name: "value-error"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeParamName: entry.Name, DescribeGeneration: 6, DescribeLoading: true, DescribeHistory: []HistoryEntry{{VersionID: "current", Labels: []string{"AWSCURRENT"}}}}}

	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 6, err: errors.New("denied")})
	if m.state.DescribeLoading || !strings.Contains(m.state.DescribeValueError, "denied") {
		t.Fatal("failed default value request was discarded")
	}

	m.state.DescribeSelectionChanged = true
	m.state.ErrorMessage = ""
	m = updateModel(t, m, secretValueLoadedMsg{identity: id, generation: 6, requestedVersionID: "old", versionID: "old", copy: true, err: errors.New("copy denied")})
	if !strings.Contains(m.state.ErrorMessage, "copy denied") {
		t.Fatal("failed copy was discarded after version navigation")
	}
}

func TestSecretMetadataSurvivesVersionListDenial(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:denied-versions")
	metadata := aws.SecretMetadata{Identity: id, Name: "metadata", ARN: id.CanonicalID, Description: "still visible"}
	entry := cache.CacheEntry{Identity: id, Name: metadata.Name}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeParamName: metadata.Name, DescribeGeneration: 2}, secretMetadata: map[aws.ResourceIdentity]aws.SecretMetadata{}}

	m = updateModel(t, m, secretDetailLoadedMsg{identity: id, generation: 2, metadata: metadata, err: errors.New("versions denied")})
	if m.secretMetadata[id].Description != "still visible" || !strings.Contains(m.state.ErrorMessage, "versions denied") {
		t.Fatal("version denial discarded readable secret metadata")
	}
}

func TestRevealBeforeSecretVersionsLoadFetchesCurrentValue(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:current")
	entry := cache.CacheEntry{Identity: id, Name: "current"}
	provider := &fakeSecretsManager{value: &aws.SecretDetail{Identity: id, Name: entry.Name, ARN: id.CanonicalID, VersionID: "v1", Value: aws.NewTextValue(id, "value")}}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeParamName: entry.Name, DescribeGeneration: 1, DescribeMasked: true}, secrets: provider}

	updated, cmd := m.handleDescribeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("reveal was dropped while version metadata was unavailable")
	}
	m = updateModel(t, m, cmd())
	if provider.valueCalls != 1 || m.state.DescribeValue != "value" {
		t.Fatalf("value calls=%d value=%q", provider.valueCalls, m.state.DescribeValue)
	}
}

func TestEmptyLoadedSecretCanBeCopied(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:empty")
	entry := cache.CacheEntry{Identity: id, Name: "empty"}
	m := Model{state: State{Mode: ViewModeDescribe, DescribeEntry: &entry, DescribeIdentity: id, DescribeGeneration: 1, DescribeValueKind: aws.ValueText}}

	_, cmd := m.handleDescribeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("copy was disabled for a loaded empty value")
	}
}

func TestDeleteCompletionReconcilesAfterSelectionChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	idA, idB := resourceID(aws.BackendSSM, "/a"), resourceID(aws.BackendSSM, "/b")
	cfg := &config.Config{CacheTTL: time.Hour}
	manager, err := cache.NewManagerForBackend(cfg, idA.Partition, idA.Region, idA.AccountID, aws.BackendSSM)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceSnapshot([]cache.CacheEntry{{Identity: idA, Name: "/a"}, {Identity: idB, Name: "/b"}}); err != nil {
		t.Fatal(err)
	}
	m := Model{state: State{Mode: ViewModeList, Entries: manager.GetAll(), FilteredItems: manager.GetAll(), SelectedIndex: 1}, caches: map[aws.Backend]*cache.Manager{aws.BackendSSM: manager}}

	m = updateModel(t, m, deleteCompleteMsg{identity: idA, generation: 1, name: "/a"})
	if _, ok := manager.GetByIdentity(idA); ok || !strings.Contains(m.state.StatusMessage, "deleted") {
		t.Fatal("successful deletion was not reconciled after selection changed")
	}
}
