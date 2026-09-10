package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/util"
)

// SecretsManagerBrowseClient is the narrow Secrets Manager surface used by the browser.
type SecretsManagerBrowseClient interface {
	ListSecrets(context.Context) ([]aws.SecretMetadata, error)
	ListSecretVersionIds(context.Context, string) ([]aws.SecretVersion, error)
	GetSecretValue(context.Context, string, aws.SecretValueSelector) (*aws.SecretDetail, error)
	CreateSecret(context.Context, aws.CreateSecretRequest) (*aws.CreateSecretResult, error)
	PutSecretValue(context.Context, aws.PutSecretValueRequest) (*aws.PutSecretValueResult, error)
	TagResource(context.Context, aws.TagSecretRequest) error
	UntagResource(context.Context, aws.UntagSecretRequest) error
	DeleteSecret(context.Context, aws.DeleteSecretRequest) (*aws.DeleteSecretResult, error)
	RestoreSecret(context.Context, aws.RestoreSecretRequest) (*aws.RestoreSecretResult, error)
}

type smMode int

const (
	smList smMode = iota
	smDetail
)

type smPrompt struct {
	action       string
	input        string
	error        string
	permanent    bool
	recoveryDays int64
}

// SecretsManagerModel is a Secrets Manager-specific browser. It deliberately
// does not contain Parameter Store filters, labels, moves, or copies.
type SecretsManagerModel struct {
	client    SecretsManagerBrowseClient
	cache     *cache.Manager
	config    *config.Config
	clipboard *util.ClipboardManager
	scope     aws.ResourceIdentity

	mode       smMode
	entries    []cache.CacheEntry
	filtered   []cache.CacheEntry
	metadata   map[aws.ResourceIdentity]aws.SecretMetadata
	selected   int
	scroll     int
	searching  bool
	search     textinput.Model
	width      int
	height     int
	ready      bool
	quitting   bool
	refreshing bool
	refreshGen uint64
	statusGen  uint64

	detailIdentity   aws.ResourceIdentity
	detailGeneration uint64
	versions         []aws.SecretVersion
	versionIndex     int
	value            aws.ResourceValue
	valueVersionID   string
	valueLoaded      bool
	valueLoading     bool
	masked           bool
	prompt           smPrompt
	status           string
	err              string
}

type smEntriesLoadedMsg struct{ entries []cache.CacheEntry }
type smRefreshStartMsg struct{}
type smRefreshMsg struct {
	generation uint64
	metadata   []aws.SecretMetadata
	entries    []cache.CacheEntry
	err        error
}
type smDetailMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	versions   []aws.SecretVersion
	err        error
}
type smValueMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	versionID  string
	value      aws.ResourceValue
	copy       bool
	err        error
}
type smEditorMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	action     string
	name       string
	session    *util.EditorSession
	err        error
}
type smMutationMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	action     string
	message    string
	err        error
}
type smCopyMsg struct {
	identity   aws.ResourceIdentity
	generation uint64
	message    string
	err        error
}
type smClearMsg struct{ generation uint64 }

// NewSecretsManagerModel creates a Secrets Manager browser over exactly one cache.
func NewSecretsManagerModel(client SecretsManagerBrowseClient, cacheMgr *cache.Manager, cfg *config.Config, scope aws.ResourceIdentity) SecretsManagerModel {
	search := textinput.New()
	search.Placeholder = "Search secret names..."
	search.CharLimit = 256
	return SecretsManagerModel{
		client: client, cache: cacheMgr, config: cfg,
		clipboard: util.NewClipboardManager(cfg.ClipboardTimeout), scope: scope,
		metadata: make(map[aws.ResourceIdentity]aws.SecretMetadata), search: search, masked: true,
	}
}

func (m SecretsManagerModel) Init() tea.Cmd {
	commands := []tea.Cmd{tea.EnterAltScreen, tea.EnableMouseAllMotion, m.loadCache}
	if m.config.BrowseAutoRefresh {
		commands = append(commands, func() tea.Msg { return smRefreshStartMsg{} })
	}
	return tea.Batch(commands...)
}

func (m SecretsManagerModel) loadCache() tea.Msg {
	if m.cache == nil {
		return smEntriesLoadedMsg{}
	}
	return smEntriesLoadedMsg{entries: m.cache.GetAll()}
}

func (m SecretsManagerModel) refresh(generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		metadata, err := m.client.ListSecrets(ctx)
		if err != nil {
			return smRefreshMsg{generation: generation, err: err}
		}
		entries := make([]cache.CacheEntry, 0, len(metadata))
		for _, secret := range metadata {
			changed := time.Time{}
			if secret.LastChangedDate != nil {
				changed = *secret.LastChangedDate
			} else if secret.CreatedDate != nil {
				changed = *secret.CreatedDate
			}
			entries = append(entries, cache.CacheEntry{Identity: secret.Identity, Name: secret.Name, LastModifiedDate: changed, Tags: secret.Tags, TagsComplete: true})
		}
		if m.cache != nil {
			if err := m.cache.ReplaceSnapshot(entries); err != nil {
				return smRefreshMsg{generation: generation, metadata: metadata, entries: entries, err: err}
			}
		}
		return smRefreshMsg{generation: generation, metadata: metadata, entries: entries}
	}
}

func (m SecretsManagerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.searching {
		if key, ok := message.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc", "enter":
				m.searching = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(message)
			m.filter()
			return m, cmd
		}
	}

	switch msg := message.(type) {
	case tea.KeyMsg:
		if m.prompt.action != "" {
			return m.updatePrompt(msg)
		}
		return m.updateKey(msg)
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case smEntriesLoadedMsg:
		m.entries = msg.entries
		for _, entry := range msg.entries {
			m.metadata[entry.Identity] = aws.SecretMetadata{Identity: entry.Identity, Name: entry.Name, ARN: entry.Identity.CanonicalID, Tags: entry.Tags, LastChangedDate: &entry.LastModifiedDate}
		}
		m.filter()
	case smRefreshStartMsg:
		m.startRefresh()
		return m, m.refresh(m.refreshGen)
	case smRefreshMsg:
		if msg.generation != m.refreshGen {
			return m, nil
		}
		m.refreshing = false
		if msg.err != nil {
			m.setError("Secrets Manager refresh failed: " + msg.err.Error())
			return m, nil
		}
		m.metadata = make(map[aws.ResourceIdentity]aws.SecretMetadata, len(msg.metadata))
		for _, item := range msg.metadata {
			m.metadata[item.Identity] = item
		}
		m.entries = msg.entries
		m.filter()
		return m, m.clearStatusAfter(m.setStatus(fmt.Sprintf("Secrets Manager inventory refreshed: %d secrets", len(m.entries))))
	case smDetailMsg:
		if !m.current(msg.identity, msg.generation) {
			return m, nil
		}
		m.valueLoading = false
		if msg.err != nil {
			m.setError("Version metadata unavailable: " + msg.err.Error())
			return m, nil
		}
		m.versions = msg.versions
		m.versionIndex = m.currentVersionIndex()
	case smValueMsg:
		if !m.current(msg.identity, msg.generation) {
			return m, nil
		}
		m.valueLoading = false
		if msg.err != nil {
			m.setError("Value unavailable: " + msg.err.Error())
			return m, nil
		}
		if msg.copy {
			return m, m.copyValue(msg.identity, msg.generation, displaySecretValue(msg.value))
		}
		if selected := m.selectedVersionID(); selected != "" && selected != msg.versionID {
			return m, nil
		}
		m.value, m.valueVersionID, m.valueLoaded, m.err = msg.value, msg.versionID, true, ""
	case smEditorMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.current(msg.identity, msg.generation) {
			if msg.session != nil {
				_ = msg.session.Close()
			}
			return m, nil
		}
		if msg.err != nil {
			m.setError(msg.err.Error())
			return m, nil
		}
		return m, tea.ExecProcess(msg.session.Command(), func(runErr error) tea.Msg {
			defer func() { _ = msg.session.Close() }()
			if runErr != nil {
				return smMutationMsg{identity: msg.identity, generation: msg.generation, action: msg.action, err: fmt.Errorf("editor: %w", runErr)}
			}
			text, err := msg.session.Read()
			if err != nil {
				return smMutationMsg{identity: msg.identity, generation: msg.generation, action: msg.action, err: err}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if msg.action == "create" {
				_, err = m.client.CreateSecret(ctx, aws.CreateSecretRequest{Name: msg.name, Value: aws.SecretValueInput{Kind: aws.ValueText, Text: text}})
			} else {
				_, err = m.client.PutSecretValue(ctx, aws.PutSecretValueRequest{SecretID: msg.identity.CanonicalID, Value: aws.SecretValueInput{Kind: aws.ValueText, Text: text}})
			}
			return smMutationMsg{identity: msg.identity, generation: msg.generation, action: msg.action, message: msg.name, err: err}
		})
	case smMutationMsg:
		if msg.identity != (aws.ResourceIdentity{}) && !m.current(msg.identity, msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.setError(msg.action + " failed: " + msg.err.Error())
			return m, nil
		}
		m.setStatus(msg.message)
		if msg.action == "delete" || msg.action == "restore" || msg.action == "create" {
			m.mode = smList
		}
		m.startRefresh()
		return m, m.refresh(m.refreshGen)
	case smCopyMsg:
		if !m.current(msg.identity, msg.generation) {
			return m, nil
		}
		if msg.err != nil {
			m.setError("copy failed: " + msg.err.Error())
		} else {
			m.setStatus(msg.message)
		}
	case smClearMsg:
		if msg.generation == m.statusGen {
			m.status, m.err = "", ""
		}
	}
	return m, nil
}

func (m SecretsManagerModel) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := key.String()
	if s == "ctrl+c" || s == "q" {
		if m.mode == smDetail {
			m.closeDetail()
			return m, nil
		}
		if m.clipboard != nil {
			_ = m.clipboard.Close()
		}
		m.quitting = true
		return m, tea.Quit
	}
	if s == "/" && m.mode == smList {
		m.searching = true
		m.search.Focus()
		return m, textinput.Blink
	}
	if s == "r" {
		m.startRefresh()
		return m, m.refresh(m.refreshGen)
	}
	if m.mode == smList {
		return m.updateListKey(s)
	}
	return m.updateDetailKey(s)
}

func (m SecretsManagerModel) updateListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "pgup", "left":
		m.moveSelection(-m.visibleRows())
	case "pgdown", "right":
		m.moveSelection(m.visibleRows())
	case "home":
		m.selected = 0
	case "end":
		m.selected = max(0, len(m.filtered)-1)
	case "d", "enter":
		return m.openDetail()
	case "c":
		if entry := m.selectedEntry(); entry != nil {
			return m, m.loadValue(entry.Identity, "", true, m.detailGeneration)
		}
	case "C":
		if entry := m.selectedEntry(); entry != nil {
			return m, m.copyValue(entry.Identity, m.detailGeneration, entry.Name)
		}
	case "n":
		m.prompt = smPrompt{action: "create"}
	case "e":
		if entry := m.selectedEntry(); entry != nil {
			m.prompt = smPrompt{action: "version"}
		}
	case "T":
		if m.selectedEntry() != nil {
			m.prompt = smPrompt{action: "tag-add"}
		}
	case "D":
		if m.selectedEntry() != nil {
			m.prompt = smPrompt{action: "tag-remove"}
		}
	case "delete":
		if m.selectedEntry() != nil {
			m.prompt = smPrompt{action: "delete-options"}
		}
	case "u":
		if entry := m.selectedEntry(); entry != nil {
			return m, m.restore(entry.Identity, m.detailGeneration)
		}
	}
	m.adjustScroll()
	return m, nil
}

func (m SecretsManagerModel) updateDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.closeDetail()
	case "tab", "j":
		m.moveVersion(1)
	case "shift+tab", "k":
		m.moveVersion(-1)
	case "g":
		m.versionIndex = m.currentVersionIndex()
		m.clearValue()
	case "x":
		m.masked = !m.masked
		if !m.masked && !m.valueLoaded {
			m.valueLoading = true
			return m, m.loadValue(m.detailIdentity, m.selectedVersionID(), false, m.detailGeneration)
		}
	case "c":
		if m.valueLoaded {
			return m, m.copyValue(m.detailIdentity, m.detailGeneration, displaySecretValue(m.value))
		}
		return m, m.loadValue(m.detailIdentity, m.selectedVersionID(), true, m.detailGeneration)
	case "C":
		return m, m.copyValue(m.detailIdentity, m.detailGeneration, m.detailName())
	case "e":
		m.prompt = smPrompt{action: "version"}
	case "T":
		m.prompt = smPrompt{action: "tag-add"}
	case "D":
		m.prompt = smPrompt{action: "tag-remove"}
	case "delete":
		m.prompt = smPrompt{action: "delete-options"}
	case "u":
		return m, m.restore(m.detailIdentity, m.detailGeneration)
	case "up":
		m.moveVersion(-1)
	case "down":
		m.moveVersion(1)
	}
	return m, nil
}

func (m SecretsManagerModel) updatePrompt(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.prompt = smPrompt{}
		return m, nil
	case "backspace":
		if len(m.prompt.input) > 0 {
			m.prompt.input = m.prompt.input[:len(m.prompt.input)-1]
		}
		m.prompt.error = ""
		return m, nil
	case "enter":
		return m.submitPrompt()
	default:
		if key.Type == tea.KeyRunes {
			m.prompt.input += string(key.Runes)
			m.prompt.error = ""
		}
		return m, nil
	}
}

func (m SecretsManagerModel) submitPrompt() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.prompt.input)
	action := m.prompt.action
	switch action {
	case "create":
		name, source := splitCreateInput(input)
		if name == "" {
			m.prompt.error = "Secret name is required"
			return m, nil
		}
		m.prompt = smPrompt{}
		if source != "" {
			return m, m.writeBinary("create", aws.ResourceIdentity{}, m.detailGeneration, name, source)
		}
		return m, m.prepareEditor("create", aws.ResourceIdentity{}, m.detailGeneration, name, "")
	case "version":
		entry := m.activeEntry()
		if entry == nil {
			m.prompt = smPrompt{}
			return m, nil
		}
		m.prompt = smPrompt{}
		if input != "" {
			return m, m.writeBinary("version", entry.Identity, m.detailGeneration, entry.Name, input)
		}
		return m, m.prepareTextVersion(*entry)
	case "tag-add":
		parts := strings.SplitN(input, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			m.prompt.error = "Use key=value"
			return m, nil
		}
		entry := m.activeEntry()
		if entry == nil {
			m.prompt = smPrompt{}
			return m, nil
		}
		m.prompt = smPrompt{}
		return m, m.changeTags(entry.Identity, m.detailGeneration, map[string]string{strings.TrimSpace(parts[0]): strings.TrimSpace(parts[1])}, nil)
	case "tag-remove":
		if input == "" {
			m.prompt.error = "Tag key is required"
			return m, nil
		}
		entry := m.activeEntry()
		if entry == nil {
			m.prompt = smPrompt{}
			return m, nil
		}
		m.prompt = smPrompt{}
		return m, m.changeTags(entry.Identity, m.detailGeneration, nil, []string{input})
	case "delete-options":
		if input == "permanent" {
			m.prompt = smPrompt{action: "delete-confirm", permanent: true}
			return m, nil
		}
		days, err := strconv.ParseInt(input, 10, 64)
		if err != nil || days < 7 || days > 30 {
			m.prompt.error = "Enter a recovery window from 7 to 30, or permanent"
			return m, nil
		}
		m.prompt = smPrompt{action: "delete-confirm", recoveryDays: days}
		return m, nil
	case "delete-confirm":
		entry := m.activeEntry()
		if entry == nil || input != "delete "+entry.Name {
			m.prompt.error = "Type delete " + m.activeName() + " to confirm"
			return m, nil
		}
		request := aws.DeleteSecretRequest{SecretID: entry.Identity.CanonicalID, RecoveryWindowDays: m.prompt.recoveryDays, Permanent: m.prompt.permanent}
		m.prompt = smPrompt{}
		return m, m.delete(entry.Identity, m.detailGeneration, request)
	}
	return m, nil
}

func splitCreateInput(input string) (string, string) {
	parts := strings.SplitN(input, "|", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return name, ""
	}
	return name, strings.TrimSpace(parts[1])
}

func binaryPath(source string) (string, error) {
	if !strings.HasPrefix(source, "fileb://") || strings.TrimPrefix(source, "fileb://") == "" {
		return "", fmt.Errorf("binary input must use fileb://path")
	}
	return strings.TrimPrefix(source, "fileb://"), nil
}

func (m SecretsManagerModel) writeBinary(action string, identity aws.ResourceIdentity, generation uint64, name, source string) tea.Cmd {
	return func() tea.Msg {
		path, err := binaryPath(source)
		if err != nil {
			return smMutationMsg{identity: identity, generation: generation, action: action, err: err}
		}
		value, err := os.ReadFile(path)
		if err != nil {
			return smMutationMsg{identity: identity, generation: generation, action: action, err: fmt.Errorf("read binary file: %w", err)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if action == "create" {
			_, err = m.client.CreateSecret(ctx, aws.CreateSecretRequest{Name: name, Value: aws.SecretValueInput{Kind: aws.ValueBinary, Binary: value}})
		} else {
			_, err = m.client.PutSecretValue(ctx, aws.PutSecretValueRequest{SecretID: identity.CanonicalID, Value: aws.SecretValueInput{Kind: aws.ValueBinary, Binary: value}})
		}
		return smMutationMsg{identity: identity, generation: generation, action: action, message: action + " completed", err: err}
	}
}

func (m SecretsManagerModel) prepareEditor(action string, identity aws.ResourceIdentity, generation uint64, name, content string) tea.Cmd {
	return func() tea.Msg {
		session, err := util.NewEditor(util.EditorConfig{}).Prepare(content, contentExtension(content))
		return smEditorMsg{identity: identity, generation: generation, action: action, name: name, session: session, err: err}
	}
}

func (m SecretsManagerModel) prepareTextVersion(entry cache.CacheEntry) tea.Cmd {
	identity, generation := entry.Identity, m.detailGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		selector := aws.SecretValueSelector{}
		if m.mode == smDetail {
			selector.VersionID = m.selectedVersionID()
		}
		detail, err := m.client.GetSecretValue(ctx, identity.CanonicalID, selector)
		if err != nil {
			return smEditorMsg{identity: identity, generation: generation, action: "version", name: entry.Name, err: err}
		}
		if detail.Identity != identity {
			return smEditorMsg{identity: identity, generation: generation, action: "version", name: entry.Name, err: fmt.Errorf("provider returned a different qualified identity")}
		}
		if detail.Value.Kind != aws.ValueText {
			return smEditorMsg{identity: identity, generation: generation, action: "version", name: entry.Name, err: fmt.Errorf("binary versions require a fileb:// path")}
		}
		session, err := util.NewEditor(util.EditorConfig{}).Prepare(detail.Value.Text, contentExtension(detail.Value.Text))
		return smEditorMsg{identity: identity, generation: generation, action: "version", name: entry.Name, session: session, err: err}
	}
}

func contentExtension(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ".json"
	}
	if strings.HasPrefix(trimmed, "<") {
		return ".xml"
	}
	return ".txt"
}

func (m SecretsManagerModel) changeTags(identity aws.ResourceIdentity, generation uint64, add map[string]string, remove []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		if len(add) > 0 {
			err = m.client.TagResource(ctx, aws.TagSecretRequest{SecretID: identity.CanonicalID, Tags: add})
		} else {
			err = m.client.UntagResource(ctx, aws.UntagSecretRequest{SecretID: identity.CanonicalID, TagKeys: remove})
		}
		return smMutationMsg{identity: identity, generation: generation, action: "tag", message: "Tags updated", err: err}
	}
}

func (m SecretsManagerModel) delete(identity aws.ResourceIdentity, generation uint64, request aws.DeleteSecretRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := m.client.DeleteSecret(ctx, request)
		return smMutationMsg{identity: identity, generation: generation, action: "delete", message: "Secret deletion requested", err: err}
	}
}

func (m SecretsManagerModel) restore(identity aws.ResourceIdentity, generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := m.client.RestoreSecret(ctx, aws.RestoreSecretRequest{SecretID: identity.CanonicalID})
		return smMutationMsg{identity: identity, generation: generation, action: "restore", message: "Secret restored", err: err}
	}
}

func (m SecretsManagerModel) openDetail() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil {
		return m, nil
	}
	m.mode, m.detailIdentity = smDetail, entry.Identity
	m.detailGeneration++
	m.versions, m.versionIndex = nil, 0
	m.masked = true
	m.clearValue()
	identity, generation := entry.Identity, m.detailGeneration
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		versions, err := m.client.ListSecretVersionIds(ctx, identity.CanonicalID)
		return smDetailMsg{identity: identity, generation: generation, versions: versions, err: err}
	}
}

func (m SecretsManagerModel) loadValue(identity aws.ResourceIdentity, versionID string, copyValue bool, generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		detail, err := m.client.GetSecretValue(ctx, identity.CanonicalID, aws.SecretValueSelector{VersionID: versionID})
		if err != nil {
			return smValueMsg{identity: identity, generation: generation, versionID: versionID, copy: copyValue, err: err}
		}
		if detail.Identity != identity {
			return smValueMsg{identity: identity, generation: generation, copy: copyValue, err: fmt.Errorf("provider returned a different qualified identity")}
		}
		return smValueMsg{identity: identity, generation: generation, versionID: detail.VersionID, value: detail.Value, copy: copyValue}
	}
}

func (m SecretsManagerModel) copyValue(identity aws.ResourceIdentity, generation uint64, value string) tea.Cmd {
	return func() tea.Msg {
		message, err := m.clipboard.CopyWithMessage(value)
		return smCopyMsg{identity: identity, generation: generation, message: "Copied (" + message + ")", err: err}
	}
}

func displaySecretValue(value aws.ResourceValue) string {
	if value.Kind == aws.ValueBinary {
		return base64.StdEncoding.EncodeToString(value.Binary)
	}
	return value.Text
}

func (m *SecretsManagerModel) filter() {
	query := m.search.Value()
	m.filtered = m.filtered[:0]
	for _, entry := range m.entries {
		if query == "" || matchSearch(query, entry.Name) {
			m.filtered = append(m.filtered, entry)
		}
	}
	sort.SliceStable(m.filtered, func(i, j int) bool { return m.filtered[i].Name < m.filtered[j].Name })
	if m.selected >= len(m.filtered) {
		m.selected = max(0, len(m.filtered)-1)
	}
	m.adjustScroll()
}

func (m *SecretsManagerModel) startRefresh() {
	m.refreshGen++
	m.refreshing = true
	m.statusGen++
	m.status, m.err = "Refreshing Secrets Manager metadata...", ""
}

func (m *SecretsManagerModel) setStatus(status string) uint64 {
	m.statusGen++
	m.status, m.err = status, ""
	return m.statusGen
}

func (m SecretsManagerModel) clearStatusAfter(generation uint64) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return smClearMsg{generation: generation}
	})
}

func (m *SecretsManagerModel) setError(err string) {
	m.statusGen++
	m.status, m.err = "", err
}

func (m *SecretsManagerModel) moveSelection(delta int) {
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = max(0, len(m.filtered)-1)
	}
}

func (m *SecretsManagerModel) adjustScroll() {
	rows := m.visibleRows()
	if m.selected < m.scroll {
		m.scroll = m.selected
	}
	if m.selected >= m.scroll+rows {
		m.scroll = m.selected - rows + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m SecretsManagerModel) visibleRows() int { return max(5, m.height-7) }

func (m SecretsManagerModel) selectedEntry() *cache.CacheEntry {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return nil
	}
	entry := m.filtered[m.selected]
	return &entry
}

func (m SecretsManagerModel) activeEntry() *cache.CacheEntry {
	if m.mode == smList {
		return m.selectedEntry()
	}
	for _, entry := range m.entries {
		if entry.Identity == m.detailIdentity {
			copy := entry
			return &copy
		}
	}
	return nil
}

func (m SecretsManagerModel) activeName() string {
	if entry := m.activeEntry(); entry != nil {
		return entry.Name
	}
	return "secret"
}

func (m SecretsManagerModel) detailName() string { return m.activeName() }

func (m SecretsManagerModel) current(identity aws.ResourceIdentity, generation uint64) bool {
	if m.mode == smDetail {
		return identity == m.detailIdentity && generation == m.detailGeneration
	}
	entry := m.selectedEntry()
	return entry != nil && entry.Identity == identity
}

func (m *SecretsManagerModel) closeDetail() {
	m.detailGeneration++
	m.mode, m.detailIdentity, m.versions = smList, aws.ResourceIdentity{}, nil
	m.clearValue()
}

func (m *SecretsManagerModel) clearValue() {
	m.value, m.valueVersionID, m.valueLoaded, m.valueLoading = aws.ResourceValue{}, "", false, false
}

func (m *SecretsManagerModel) moveVersion(delta int) {
	if len(m.versions) == 0 {
		return
	}
	m.versionIndex = (m.versionIndex + delta + len(m.versions)) % len(m.versions)
	m.clearValue()
}

func (m SecretsManagerModel) currentVersionIndex() int {
	for i, version := range m.versions {
		for _, stage := range version.VersionStages {
			if stage == "AWSCURRENT" {
				return i
			}
		}
	}
	return 0
}

func (m SecretsManagerModel) selectedVersionID() string {
	if m.versionIndex < 0 || m.versionIndex >= len(m.versions) {
		return ""
	}
	return m.versions[m.versionIndex].VersionID
}

func (m SecretsManagerModel) View() string {
	if !m.ready {
		return "Loading..."
	}
	if m.quitting {
		return ""
	}
	var view string
	if m.mode == smDetail {
		view = m.renderSMDetail()
	} else {
		view = m.renderSMList()
	}
	if m.prompt.action != "" {
		return centerDialog(m.renderSMPrompt(), m.width, m.height)
	}
	return view
}

func (m SecretsManagerModel) renderSMList() string {
	lines := []string{renderScopeTitle(m.scope)}
	if m.searching {
		lines = append(lines, "  "+searchStyle.Render("/ ")+m.search.View())
	} else if m.search.Value() != "" {
		lines = append(lines, dimStyle.Render("  Filter: "+m.search.Value()+" (/ to edit)"))
	} else {
		lines = append(lines, "")
	}
	showModified := m.width >= 100
	showTags := m.width >= 110
	nameWidth := m.width - 15 // indent + spacing + ROTATION
	if showTags {
		nameWidth -= 7
	}
	if showModified {
		nameWidth -= 19
	}
	nameWidth = max(20, nameWidth)
	header := "  " + headerStyle.Render(fmt.Sprintf("%-*s", nameWidth, "NAME")) + "   " + headerStyle.Render(fmt.Sprintf("%-8s", "ROTATION"))
	if showTags {
		header += "   " + headerStyle.Render(fmt.Sprintf("%4s", "TAGS"))
	}
	if showModified {
		header += "   " + headerStyle.Render(fmt.Sprintf("%16s  ", "MODIFIED"))
	}
	lines = append(lines, header, "  "+separatorStyle.Render(strings.Repeat("─", max(0, m.width-4))))
	end := min(len(m.filtered), m.scroll+m.visibleRows())
	for i := m.scroll; i < end; i++ {
		entry := m.filtered[i]
		meta := m.metadata[entry.Identity]
		rotation := "off"
		if meta.RotationEnabled != nil && *meta.RotationEnabled {
			rotation = "on"
		}
		if meta.DeletedDate != nil {
			rotation = "deleting"
		}
		line := fmt.Sprintf("  %-*s   %-8s", nameWidth, truncateString(entry.Name, nameWidth), rotation)
		if showTags {
			line += fmt.Sprintf("   %4s", tagCountStr(entry))
		}
		if showModified {
			line += fmt.Sprintf("   %16s  ", entry.LastModifiedDate.Format("2006-01-02 15:04"))
		}
		if i == m.selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.filtered) == 0 {
		lines = append(lines, dimStyle.Render("    No Secrets Manager secrets found"))
	}
	for len(lines) < m.height-2 {
		lines = append(lines, "")
	}
	lines = append(lines, "  "+separatorStyle.Render(strings.Repeat("─", max(0, m.width-4))))
	lines = append(lines, m.smStatus(), "  "+renderHelp("↑↓", "navigate", "d", "details", "n", "create", "e", "new-version", "T/D", "tags", "Delete", "lifecycle", "u", "restore", "c", "copy-value", "C", "copy-name", "r", "refresh", "/", "search", "q", "quit")+"  ")
	return strings.Join(lines, "\n")
}

func (m SecretsManagerModel) renderSMDetail() string {
	meta := m.metadata[m.detailIdentity]
	title := fmt.Sprintf("  Clerk | account %s | region %s", m.scope.AccountID, m.scope.Region)
	lines := []string{dimStyle.Render(truncateString(title, max(0, m.width))), ""}
	nameWidth := max(0, m.width-4)
	modified := ""
	if meta.LastChangedDate != nil {
		modified = meta.LastChangedDate.Format("2006-01-02 15:04")
	}
	rotation := "off"
	if meta.RotationEnabled != nil && *meta.RotationEnabled {
		rotation = "on"
	}
	if m.width >= 75 {
		// Account for the indent, column gaps, rotation, tag count, and timestamp.
		nameWidth = max(0, m.width-41)
	} else if m.width >= 50 {
		nameWidth = max(0, m.width-22)
	}
	info := "  " + nameColStyle.Render(fmt.Sprintf("%-*s", nameWidth, truncateString(m.detailName(), nameWidth)))
	if m.width >= 50 {
		info += "   " + typeColStyle.Render(fmt.Sprintf("%-8s", rotation)) + "   " + tagColStyle.Render(fmt.Sprintf("%4d", len(meta.Tags)))
	}
	if m.width >= 75 {
		info += "   " + modifiedColStyle.Render(fmt.Sprintf("%16s", modified))
	}
	info += "  "
	lines = append(lines, info)
	if len(meta.Tags) > 0 {
		tags := "  Tags: " + m.smTagsLine(meta.Tags, max(0, m.width-8))
		lines = append(lines, dimStyle.Render(truncateString(tags, max(0, m.width))))
	}
	lines = append(lines, "  "+separatorStyle.Render(strings.Repeat("─", max(0, m.width-4))))

	leftWidth, rightWidth := 35, m.width-43
	if m.width <= 79 {
		available := max(2, m.width-4)
		leftWidth = min(35, available/2)
		rightWidth = available - leftWidth
	}
	// Header rows plus the separator, status, and help consume the remaining space.
	panelHeight := max(0, m.height-len(lines)-3)
	if panelHeight >= 4 {
		left := m.renderSMVersionsPanel(leftWidth, panelHeight)
		right := m.renderSMValuePanel(rightWidth, panelHeight)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, left, right))
	}
	lines = append(lines, "  "+separatorStyle.Render(strings.Repeat("─", max(0, m.width-4))))
	lines = append(lines, m.smDetailStatus(), "  "+m.smDetailHelp()+"  ")
	return strings.Join(lines, "\n")
}

func (m SecretsManagerModel) smDetailHelp() string {
	full := renderHelp(
		"x", "mask", "c", "copy-val", "C", "copy-name", "e", "new-version",
		"tab/shift+tab", "version", "g", "AWSCURRENT",
		"T", "add-tag", "D", "del-tag", "Delete", "lifecycle", "u", "restore", "r", "refresh",
		"esc", "back", "q", "quit",
	)
	compact := renderHelp(
		"x", "mask", "c", "copy", "C", "name", "e", "new", "tab", "version", "g", "current",
		"T", "add", "D", "del", "Delete", "life", "u", "restore", "r", "refresh", "esc", "back", "q", "quit",
	)
	narrow := renderHelp("x", "mask", "c", "copy", "e", "new-version", "tab", "version", "Delete", "lifecycle", "esc", "back", "q", "quit")
	minimum := renderHelp("tab", "version", "esc", "back")
	for _, help := range []string{full, compact, narrow, minimum} {
		if lipgloss.Width(help)+4 <= m.width {
			return help
		}
	}
	return ""
}

func (m SecretsManagerModel) renderSMVersionsPanel(width, height int) string {
	if height == 0 {
		return ""
	}
	lines := []string{panelHeaderStyle.Render("VERSION HISTORY"), ""}
	if len(m.versions) == 0 {
		lines = append(lines, dimStyle.Render(truncateString("No version metadata available", max(1, width))))
	} else {
		maxLines := max(1, height-4)
		linesUsed := 0
		for i, version := range m.versions {
			if linesUsed == maxLines {
				break
			}
			date := ""
			if version.CreatedDate != nil {
				date = version.CreatedDate.Format("2006-01-02 15:04")
			}
			line := truncateString(fmt.Sprintf("%s [%s] %s", version.VersionID, strings.Join(version.VersionStages, ","), date), max(1, width-2))
			if i == m.versionIndex {
				line = selectedStyle.Render("▸ " + line)
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
			linesUsed++
		}
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	if len(m.versions) > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%d/%d", m.versionIndex+1, len(m.versions))))
	} else {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().MarginLeft(2).Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m SecretsManagerModel) smTagsLine(tags map[string]string, width int) string {
	pairs := make([]string, 0, len(tags))
	for key, value := range tags {
		pairs = append(pairs, key+"="+util.SanitizeTerminal(value))
	}
	sort.Strings(pairs)
	return truncateString(strings.Join(pairs, ", "), width)
}

func (m SecretsManagerModel) renderSMValuePanel(width, height int) string {
	if height == 0 {
		return ""
	}
	contentWidth := max(1, width-4)
	header := panelHeaderStyle.Render("VALUE")
	if m.valueLoaded {
		kind := string(m.value.Kind)
		if m.value.Kind == aws.ValueBinary {
			kind = "binary, base64"
		}
		header += " " + dimStyle.Render("("+kind+")")
	}
	if m.masked {
		header += " " + dimStyle.Render("(masked)")
	}
	if lipgloss.Width(header) > width {
		header = panelHeaderStyle.Render(truncateString("VALUE", max(1, width)))
	}
	lines := []string{header, ""}
	if m.valueLoading {
		lines = append(lines, dimStyle.Render(truncateString("Loading selected value...", contentWidth)))
	} else if !m.valueLoaded {
		lines = append(lines, dimStyle.Render(truncateString("Press x to reveal or c to copy", contentWidth)))
	} else {
		value := displaySecretValue(m.value)
		if m.masked {
			value = util.MaskValue(value)
		} else {
			value = util.SanitizeTerminal(value)
		}
		for _, valueLine := range strings.Split(value, "\n") {
			if len(lines) >= height {
				break
			}
			lines = append(lines, truncateString(valueLine, contentWidth))
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().MarginRight(2).Width(width).Height(height).Render(strings.Join(lines[:height], "\n"))
}

func (m SecretsManagerModel) smStatus() string {
	if m.err != "" {
		return errorStyle.Render("  ✗ " + m.err)
	}
	if m.status != "" {
		return statusStyle.Render("  ✓ " + m.status)
	}
	return dimStyle.Render(fmt.Sprintf("  %d/%d Secrets Manager secrets", len(m.filtered), len(m.entries)))
}

func (m SecretsManagerModel) smDetailStatus() string {
	width := max(0, m.width)
	if m.err != "" {
		return errorStyle.Render(truncateString("  ✗ "+m.err, width))
	}
	if m.status != "" {
		return statusStyle.Render(truncateString("  ✓ "+m.status, width))
	}
	return dimStyle.Render(truncateString(fmt.Sprintf("  %d/%d Secrets Manager secrets", len(m.filtered), len(m.entries)), width))
}

func (m SecretsManagerModel) renderSMPrompt() string {
	var title, instruction string
	switch m.prompt.action {
	case "create":
		title, instruction = "CREATE SECRET", "Name (or name | fileb://path):"
	case "version":
		title, instruction = "CREATE NEW VERSION", "fileb://path, or blank to edit text:"
	case "tag-add":
		title, instruction = "ADD OR REPLACE TAG", "key=value:"
	case "tag-remove":
		title, instruction = "REMOVE TAG", "tag key:"
	case "delete-options":
		title, instruction = "DELETE LIFECYCLE", "Recovery days (7-30) or permanent:"
	case "delete-confirm":
		title, instruction = "CONFIRM DELETE", "Type delete "+m.activeName()+":"
	}
	content := warningStyle.Render(title) + "\n\n" + promptStyle.Render(instruction+" ") + inputStyle.Render(m.prompt.input) + "_"
	if m.prompt.action == "delete-confirm" {
		choice := fmt.Sprintf("Recovery window: %d days", m.prompt.recoveryDays)
		if m.prompt.permanent {
			choice = "Permanent deletion selected"
		}
		content = warningStyle.Render(title) + "\n\n" + warningStyle.Render(choice) + "\n\n" + promptStyle.Render(instruction+" ") + inputStyle.Render(m.prompt.input) + "_"
	}
	if m.prompt.error != "" {
		content += "\n\n" + errorStyle.Render(m.prompt.error)
	}
	content += "\n\n" + dimStyle.Render("Enter submit  Esc cancel")
	return dialogStyle.Render(content)
}
