package ui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

type fakeSecretsManager struct {
	metadata []aws.SecretMetadata
	versions []aws.SecretVersion
	value    *aws.SecretDetail

	listErr, versionErr, valueErr error
	mutationErr                   error
	listCalls, versionCalls       int
	valueCalls                    int
	createRequests                []aws.CreateSecretRequest
	versionRequests               []aws.PutSecretValueRequest
	tagRequests                   []aws.TagSecretRequest
	untagRequests                 []aws.UntagSecretRequest
	deleteRequests                []aws.DeleteSecretRequest
	restoreRequests               []aws.RestoreSecretRequest
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
func (f *fakeSecretsManager) CreateSecret(_ context.Context, request aws.CreateSecretRequest) (*aws.CreateSecretResult, error) {
	f.createRequests = append(f.createRequests, request)
	return &aws.CreateSecretResult{Name: request.Name}, f.mutationErr
}
func (f *fakeSecretsManager) PutSecretValue(_ context.Context, request aws.PutSecretValueRequest) (*aws.PutSecretValueResult, error) {
	f.versionRequests = append(f.versionRequests, request)
	return &aws.PutSecretValueResult{}, f.mutationErr
}
func (f *fakeSecretsManager) TagResource(_ context.Context, request aws.TagSecretRequest) error {
	f.tagRequests = append(f.tagRequests, request)
	return f.mutationErr
}
func (f *fakeSecretsManager) UntagResource(_ context.Context, request aws.UntagSecretRequest) error {
	f.untagRequests = append(f.untagRequests, request)
	return f.mutationErr
}
func (f *fakeSecretsManager) DeleteSecret(_ context.Context, request aws.DeleteSecretRequest) (*aws.DeleteSecretResult, error) {
	f.deleteRequests = append(f.deleteRequests, request)
	return &aws.DeleteSecretResult{}, f.mutationErr
}
func (f *fakeSecretsManager) RestoreSecret(_ context.Context, request aws.RestoreSecretRequest) (*aws.RestoreSecretResult, error) {
	f.restoreRequests = append(f.restoreRequests, request)
	return &aws.RestoreSecretResult{}, f.mutationErr
}

func resourceID(backend aws.Backend, canonical string) aws.ResourceIdentity {
	return aws.ResourceIdentity{Partition: "aws", AccountID: "123456789012", Region: "us-east-1", Backend: backend, CanonicalID: canonical}
}

func updateSSM(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func updateSM(t *testing.T, m SecretsManagerModel, msg tea.Msg) (SecretsManagerModel, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	return updated.(SecretsManagerModel), cmd
}

func smTestModel(provider *fakeSecretsManager, entries ...cache.CacheEntry) SecretsManagerModel {
	cfg := config.DefaultConfig()
	scope := resourceID(aws.BackendSecretsManager, "")
	m := NewSecretsManagerModel(provider, nil, cfg, scope)
	m.entries = entries
	m.ready, m.width, m.height = true, 120, 25
	m.filter()
	return m
}

func TestSSMDescribeResultsRequireCurrentIdentityAndGeneration(t *testing.T) {
	id := resourceID(aws.BackendSSM, "/b")
	m := Model{state: State{Mode: ViewModeDescribe, DescribeParamName: "/b", DescribeIdentity: id, DescribeGeneration: 2}}
	m = updateSSM(t, m, describeLoadedMsg{identity: resourceID(aws.BackendSSM, "/a"), name: "/a", generation: 1, value: "a"})
	if m.state.DescribeValue != "" {
		t.Fatal("stale result changed active SSM detail")
	}
	m = updateSSM(t, m, describeLoadedMsg{identity: id, name: "/b", generation: 2, value: "b"})
	if m.state.DescribeValue != "b" {
		t.Fatal("current result was not accepted")
	}
}

func TestSSMTreeNavigationAndNarrowRenderingAreSafe(t *testing.T) {
	entries := []cache.CacheEntry{{Identity: resourceID(aws.BackendSSM, "/a/b/leaf"), Name: "/a/b/leaf", Type: "String"}}
	m := Model{state: State{Mode: ViewModeTree, FilteredItems: entries, ExpandedPaths: map[string]bool{"/a": true, "/a/b": true}, Height: 20}}
	m.buildTree()
	m = updateSSM(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.state.SelectedIndex != 2 {
		t.Fatalf("selected %d, want final tree row", m.state.SelectedIndex)
	}
	m.state.Width, m.state.Height = 0, 0
	_ = m.renderBrowseView()
}

func TestResourceViewsUseSharedScopeTitle(t *testing.T) {
	scope := resourceID(aws.BackendSSM, "/secret")
	title := "Clerk | account 123456789012 | region us-east-1"
	entry := cache.CacheEntry{Identity: scope, Name: "/secret", Type: "String"}
	views := []string{
		Model{scope: scope, state: State{Mode: ViewModeList, Width: 120, Height: 20}}.renderBrowseView(),
		Model{scope: scope, state: State{Mode: ViewModeTree, Width: 120, Height: 20}}.renderBrowseView(),
		Model{scope: scope, state: State{Mode: ViewModeDescribe, Width: 120, Height: 20, DescribeEntry: &entry}}.renderDescribeView(),
	}
	for _, view := range views {
		if !strings.Contains(view, title) {
			t.Fatalf("view missing shared title %q", title)
		}
		for _, legacy := range []string{"CLERK -", "DESCRIBE SSM", " LIST ", " TREE "} {
			if strings.Contains(view, legacy) {
				t.Fatalf("view contains legacy title text %q", legacy)
			}
		}
	}
}

func TestSecretsInventoryIsMetadataOnlyAndUsesOneSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	provider := &fakeSecretsManager{metadata: []aws.SecretMetadata{{Identity: id, Name: "secret", ARN: id.CanonicalID}}}
	cfg := config.DefaultConfig()
	manager, err := cache.NewManagerForBackend(cfg, id.Partition, id.Region, id.AccountID, aws.BackendSecretsManager)
	if err != nil {
		t.Fatal(err)
	}
	m := NewSecretsManagerModel(provider, manager, cfg, id)
	msg := m.refresh(2)().(smRefreshMsg)
	if provider.valueCalls != 0 || provider.listCalls != 1 || len(msg.entries) != 1 {
		t.Fatalf("list=%d values=%d entries=%d", provider.listCalls, provider.valueCalls, len(msg.entries))
	}
	if got := manager.GetAll(); len(got) != 1 || got[0].Identity != id {
		t.Fatalf("snapshot not replaced: %#v", got)
	}
}

func TestSecretsViewHasProviderRelevantActionsOnly(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	view := m.View()
	for _, expected := range []string{"Clerk | account 123456789012 | region us-east-1", "ROTATION", "TAGS", "MODIFIED", "new-version", "lifecycle", "restore"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("SM view missing %q", expected)
		}
	}
	for _, unsupported := range []string{"CLERK - LIST", "CLERK - DETAIL", "DESCRIBE SSM", "type", "move", "copy-to", "label", "backend"} {
		if strings.Contains(view, unsupported) {
			t.Fatalf("SM view exposes unsupported action %q", unsupported)
		}
	}
	before := m
	m, cmd := updateSM(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil || m.prompt.action != "" || m.status != before.status || m.err != before.err {
		t.Fatal("unsupported move key entered an action path")
	}
}

func TestSecretsRendererMatchesSSMShellAndKeepsSecretPanels(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	changed := time.Date(2026, time.January, 2, 3, 4, 0, 0, time.UTC)
	rotation := true
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret", LastModifiedDate: changed, Tags: map[string]string{"env": "prod"}})
	m.metadata[id] = aws.SecretMetadata{Identity: id, Name: "secret", ARN: id.CanonicalID, Description: "database credentials", KMSKeyID: "alias/secrets", Tags: map[string]string{"env": "prod"}, LastChangedDate: &changed, RotationEnabled: &rotation}
	m.search.SetValue("secret")
	m.filter()

	list := m.View()
	for _, expected := range []string{"Filter: secret (/ to edit)", "────────────────", "1/1 Secrets Manager secrets", "↑↓:navigate"} {
		if !strings.Contains(list, expected) {
			t.Fatalf("SM list missing SSM shell signal %q", expected)
		}
	}

	m.mode, m.detailIdentity = smDetail, id
	m.versions = []aws.SecretVersion{{VersionID: "current-version", VersionStages: []string{"AWSCURRENT"}, CreatedDate: &changed}}
	detail := m.View()
	for _, expected := range []string{"Clerk | account 123456789012 | region us-east-1", "VERSION HISTORY", "VALUE", "Tags: env=prod", "current-version", "new-version", "lifecycle"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("SM detail missing provider panel or action %q", expected)
		}
	}
	for _, unexpected := range []string{"CLERK - DETAIL", "SECRET VALUE", "ARN: arn:secret", "Description: database credentials", "KMS: alias/secrets", "Rotation: enabled", "Lifecycle:"} {
		if strings.Contains(detail, unexpected) {
			t.Fatalf("SM detail has metadata panel clutter %q", unexpected)
		}
	}
	for _, line := range strings.Split(detail, "\n") {
		if strings.Contains(line, "VERSION HISTORY") || strings.Contains(line, "current-version") || strings.Contains(line, "Press x to reveal") {
			if strings.HasPrefix(line, "    ") {
				t.Fatalf("SM panel content has extra indentation: %q", line)
			}
		}
	}
	if strings.Count(detail, "────────────────") < 2 {
		t.Fatal("SM detail is missing its content and footer separators")
	}

}

func TestSecretsDetailRendererMatchesSSMGeometryAndHelp(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	changed := time.Date(2026, time.January, 2, 3, 4, 0, 0, time.UTC)
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.width, m.height = 120, 25
	m.mode, m.detailIdentity = smDetail, id
	m.versions = []aws.SecretVersion{{VersionID: "v1", VersionStages: []string{"CURRENT"}, CreatedDate: &changed}, {VersionID: "v2", VersionStages: []string{"PREVIOUS"}, CreatedDate: &changed}}
	m.versionIndex = 1
	m.valueLoaded = true
	m.value = aws.NewBinaryValue(id, []byte{0, 1, 2})

	detail := m.View()
	lines := strings.Split(detail, "\n")
	if len(lines) != m.height {
		t.Fatalf("detail lines=%d, want %d", len(lines), m.height)
	}
	if !strings.Contains(lines[4], "VERSION HISTORY") || !strings.Contains(lines[4], "VALUE (binary, base64) (masked)") {
		t.Fatalf("panel headings do not share SSM placement: %q", lines[4])
	}
	if !strings.Contains(lines[6], "  v1 [CURRENT]") || !strings.Contains(lines[7], "▸ v2 [PREVIOUS]") {
		t.Fatalf("version rows do not use SSM indentation/marker: %q / %q", lines[6], lines[7])
	}
}

func TestSecretsDetailFitsTerminalAtAllWidths(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	changed := time.Date(2026, time.January, 2, 3, 4, 0, 0, time.UTC)
	for _, size := range [][2]int{{120, 25}, {80, 24}, {42, 14}} {
		m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: strings.Repeat("long-secret-name-", 10)})
		m.width, m.height = size[0], size[1]
		m.mode, m.detailIdentity = smDetail, id
		m.metadata[id] = aws.SecretMetadata{Identity: id, Tags: map[string]string{"environment": "production"}, LastChangedDate: &changed}
		m.versions = []aws.SecretVersion{{VersionID: strings.Repeat("version-", 12), VersionStages: []string{"AWSCURRENT"}, CreatedDate: &changed}}
		m.valueLoaded, m.masked = true, false
		m.value = aws.NewTextValue(id, strings.Repeat("very-long-value ", 30))

		lines := strings.Split(m.View(), "\n")
		if len(lines) > m.height {
			t.Errorf("%dx%d detail uses %d rows", m.width, m.height, len(lines))
		}
		for _, line := range lines {
			if got := lipgloss.Width(line); got > m.width {
				t.Errorf("%dx%d detail line width=%d: %q", m.width, m.height, got, line)
			}
		}
		title, info, tags := strings.Index(m.View(), "Clerk"), strings.Index(m.View(), "long-secret"), strings.Index(m.View(), "Tags:")
		if title < 0 || info < 0 || tags < 0 || title >= info || info >= tags {
			t.Errorf("%dx%d title/info/tags order is invalid", m.width, m.height)
		}
	}
}

func TestSecretsDetailRendererOmitsEmptyTags(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.mode, m.detailIdentity = smDetail, id
	if detail := m.View(); strings.Contains(detail, "Tags:") || strings.Contains(detail, "(none)") {
		t.Fatalf("detail rendered empty tags: %q", detail)
	}
}

func TestSecretsDetailLoadsMetadataWithoutValue(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	provider := &fakeSecretsManager{versions: []aws.SecretVersion{{VersionID: "v1", VersionStages: []string{"AWSCURRENT"}}}}
	m := smTestModel(provider, cache.CacheEntry{Identity: id, Name: "secret"})
	updated, cmd := m.openDetail()
	m = updated.(SecretsManagerModel)
	if cmd == nil {
		t.Fatal("detail did not request version metadata")
	}
	m, _ = updateSM(t, m, cmd())
	if provider.versionCalls != 1 || provider.valueCalls != 0 || m.valueLoaded {
		t.Fatalf("versions=%d values=%d loaded=%v", provider.versionCalls, provider.valueCalls, m.valueLoaded)
	}
}

func TestSecretsStaleAndCrossIdentityValuesAreRejected(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.mode, m.detailIdentity, m.detailGeneration = smDetail, id, 4
	m, _ = updateSM(t, m, smValueMsg{identity: id, generation: 3, versionID: "v1", value: aws.NewTextValue(id, "stale")})
	m, _ = updateSM(t, m, smValueMsg{identity: resourceID(aws.BackendSecretsManager, "arn:other"), generation: 4, versionID: "v1", value: aws.NewTextValue(id, "wrong")})
	if m.valueLoaded {
		t.Fatal("stale response populated detail")
	}
}

func TestSecretsBinaryValueUsesBase64(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:binary")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "binary"})
	m.mode, m.detailIdentity, m.detailGeneration = smDetail, id, 1
	m, _ = updateSM(t, m, smValueMsg{identity: id, generation: 1, versionID: "v1", value: aws.NewBinaryValue(id, []byte{0, 1, 2, 255})})
	if got := displaySecretValue(m.value); got != "AAEC/w==" {
		t.Fatalf("binary display = %q", got)
	}
}

func TestSecretsBinaryWritesRequireFilebPath(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:binary")
	provider := &fakeSecretsManager{}
	m := smTestModel(provider, cache.CacheEntry{Identity: id, Name: "binary"})
	msg := m.writeBinary("version", id, 1, "binary", "value.bin")().(smMutationMsg)
	if msg.err == nil || len(provider.versionRequests) != 0 {
		t.Fatal("non-fileb binary source reached provider")
	}
	path := t.TempDir() + "/value.bin"
	if err := os.WriteFile(path, []byte{0, 255}, 0600); err != nil {
		t.Fatal(err)
	}
	msg = m.writeBinary("version", id, 1, "binary", "fileb://"+path)().(smMutationMsg)
	if msg.err != nil || len(provider.versionRequests) != 1 || string(provider.versionRequests[0].Value.Binary) != string([]byte{0, 255}) {
		t.Fatalf("binary write failed: %#v requests=%#v", msg, provider.versionRequests)
	}
}

func TestSecretsDeletionRequiresValidChoiceAndConfirmation(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:delete")
	provider := &fakeSecretsManager{}
	m := smTestModel(provider, cache.CacheEntry{Identity: id, Name: "delete-me"})
	m.prompt = smPrompt{action: "delete-options", input: "6"}
	updated, cmd := m.submitPrompt()
	m = updated.(SecretsManagerModel)
	if cmd != nil || m.prompt.error == "" {
		t.Fatal("invalid recovery window accepted")
	}
	m.prompt = smPrompt{action: "delete-options", input: "14"}
	updated, _ = m.submitPrompt()
	m = updated.(SecretsManagerModel)
	if m.prompt.action != "delete-confirm" || m.prompt.recoveryDays != 14 {
		t.Fatal("valid recovery window did not advance to confirmation")
	}
	m.prompt.input = "delete delete-me"
	updated, cmd = m.submitPrompt()
	m = updated.(SecretsManagerModel)
	if cmd == nil {
		t.Fatal("confirmed deletion did not produce command")
	}
	msg := cmd().(smMutationMsg)
	if msg.err != nil || len(provider.deleteRequests) != 1 || provider.deleteRequests[0].RecoveryWindowDays != 14 || provider.deleteRequests[0].Permanent {
		t.Fatalf("unexpected delete request: %#v", provider.deleteRequests)
	}
}

func TestSecretsPermanentDeletionMustBeExplicit(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:delete")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.prompt = smPrompt{action: "delete-options", input: "permanent"}
	updated, _ := m.submitPrompt()
	m = updated.(SecretsManagerModel)
	if m.prompt.action != "delete-confirm" || !m.prompt.permanent {
		t.Fatal("explicit permanent selection was not retained")
	}
}

func TestSecretsTagAndRestoreUseNarrowMutationSurface(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	provider := &fakeSecretsManager{}
	m := smTestModel(provider, cache.CacheEntry{Identity: id, Name: "secret"})
	if msg := m.changeTags(id, 0, map[string]string{"env": "prod"}, nil)().(smMutationMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg := m.changeTags(id, 0, nil, []string{"env"})().(smMutationMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg := m.restore(id, 0)().(smMutationMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	if len(provider.tagRequests) != 1 || len(provider.untagRequests) != 1 || len(provider.restoreRequests) != 1 {
		t.Fatalf("mutation calls: add=%d remove=%d restore=%d", len(provider.tagRequests), len(provider.untagRequests), len(provider.restoreRequests))
	}
}

func TestSecretsGSelectsAWSCURRENT(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.mode, m.detailIdentity, m.detailGeneration = smDetail, id, 2
	m.versions = []aws.SecretVersion{{VersionID: "pending", VersionStages: []string{"AWSPENDING"}}, {VersionID: "current", VersionStages: []string{"AWSCURRENT"}}}
	m.versionIndex = 0
	m, _ = updateSM(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.versionIndex != 1 || m.selectedVersionID() != "current" {
		t.Fatalf("selected index=%d version=%q", m.versionIndex, m.selectedVersionID())
	}
}

func TestSecretsValueDenialPreservesMetadata(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:secret")
	meta := aws.SecretMetadata{Identity: id, Name: "secret", ARN: id.CanonicalID, Description: "still visible"}
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "secret"})
	m.metadata[id], m.mode, m.detailIdentity, m.detailGeneration = meta, smDetail, id, 3
	m, _ = updateSM(t, m, smValueMsg{identity: id, generation: 3, err: errors.New("denied")})
	if m.metadata[id].Description != "still visible" || !strings.Contains(m.err, "denied") {
		t.Fatal("value denial discarded metadata or error")
	}
}

func TestSecretsCreateInputSeparatesNameAndBinarySource(t *testing.T) {
	name, source := splitCreateInput("prod/key | fileb:///tmp/value")
	if name != "prod/key" || source != "fileb:///tmp/value" {
		t.Fatalf("name=%q source=%q", name, source)
	}
}

func TestSecretsRefreshFailureRetainsExistingRows(t *testing.T) {
	id := resourceID(aws.BackendSecretsManager, "arn:cached")
	m := smTestModel(&fakeSecretsManager{}, cache.CacheEntry{Identity: id, Name: "cached"})
	m.refreshGen = 2
	m, _ = updateSM(t, m, smRefreshMsg{generation: 2, err: errors.New("denied")})
	if len(m.entries) != 1 || !strings.Contains(m.err, "denied") {
		t.Fatal("refresh failure discarded cached inventory")
	}
}

func TestDeleteCompletionReconcilesAfterSSMSelectionChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	idA, idB := resourceID(aws.BackendSSM, "/a"), resourceID(aws.BackendSSM, "/b")
	cfg := config.DefaultConfig()
	manager, err := cache.NewManagerForBackend(cfg, idA.Partition, idA.Region, idA.AccountID, aws.BackendSSM)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReplaceSnapshot([]cache.CacheEntry{{Identity: idA, Name: "/a"}, {Identity: idB, Name: "/b"}}); err != nil {
		t.Fatal(err)
	}
	m := Model{state: State{Mode: ViewModeList, Entries: manager.GetAll(), FilteredItems: manager.GetAll(), SelectedIndex: 1}, cache: manager}
	m = updateSSM(t, m, deleteCompleteMsg{identity: idA, generation: 1, name: "/a"})
	if _, ok := manager.GetByIdentity(idA); ok || !strings.Contains(m.state.StatusMessage, "deleted") {
		t.Fatal("successful deletion was not reconciled")
	}
}
