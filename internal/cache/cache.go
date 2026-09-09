package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/parammatch"
)

const tagTTL = 15 * time.Minute

type Manager struct {
	cachePath string
	ttl       time.Duration
	data      *CacheData
	mu        sync.RWMutex
	refreshMu sync.Mutex
	lockFile  string
	scope     aws.ResourceIdentity
	changes   map[aws.ResourceIdentity]*CacheEntry
}

// NewManager preserves the original SSM-only API. New backend-aware callers
// should use NewManagerForBackend so the resolved AWS partition is retained.
func NewManager(cfg *config.Config, region, accountID string) (*Manager, error) {
	return NewManagerForBackend(cfg, "aws", region, accountID, aws.BackendSSM)
}

// NewManagerForBackend opens one backend-qualified cache snapshot.
func NewManagerForBackend(cfg *config.Config, partition, region, accountID string, backend aws.Backend) (*Manager, error) {
	if backend != aws.BackendSSM && backend != aws.BackendSecretsManager {
		return nil, fmt.Errorf("cache backend must identify one service, got %q", backend)
	}
	if partition == "" || region == "" || accountID == "" {
		return nil, fmt.Errorf("cache partition, account ID, and region are required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	scope := aws.ResourceIdentity{Partition: partition, AccountID: accountID, Region: region, Backend: backend}
	p := filepath.Join(home, ".clerk", "cache", "v2", partition, accountID, region, string(backend)+".json")
	m := &Manager{cachePath: p, ttl: cfg.CacheTTL, lockFile: p + ".lock", scope: scope, data: newCacheData(scope), changes: map[aws.ResourceIdentity]*CacheEntry{}}
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		// A malformed cache must not prevent an AWS-backed command, but retain a
		// diagnostic in memory so callers do not mistake it for a clean miss.
		m.data.LastRefreshError = "cache ignored: " + err.Error()
	} else if os.IsNotExist(err) && backend == aws.BackendSSM {
		legacyPath := filepath.Join(home, ".clerk", "cache", accountID, region+".json")
		if migrationErr := m.migrateLegacySSM(legacyPath); migrationErr != nil && !os.IsNotExist(migrationErr) {
			m.data.LastRefreshError = "legacy SSM cache ignored: " + migrationErr.Error()
		}
	}
	return m, nil
}

func newCacheData(scope aws.ResourceIdentity) *CacheData {
	return &CacheData{SchemaVersion: SchemaVersion, Partition: scope.Partition, AccountID: scope.AccountID, Region: scope.Region, Backend: scope.Backend, Entries: []CacheEntry{}, Incomplete: true}
}

func cloneEntry(e CacheEntry) CacheEntry {
	if e.Tags != nil {
		e.Tags = cloneTags(e.Tags)
	}
	if e.VersionHistory != nil {
		e.VersionHistory = append([]VersionHistoryEntry(nil), e.VersionHistory...)
	}
	return e
}
func cloneTags(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneData(in *CacheData) *CacheData {
	out := *in
	out.Entries = make([]CacheEntry, len(in.Entries))
	for i := range in.Entries {
		out.Entries[i] = cloneEntry(in.Entries[i])
	}
	return &out
}

func (m *Manager) load() error {
	b, err := os.ReadFile(m.cachePath)
	if err != nil {
		return err
	}
	var d CacheData
	if err := json.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("invalid cache file: %w", err)
	}
	if err := m.validateData(&d); err != nil {
		return err
	}
	for i := range d.Entries {
		entry, err := m.qualifyEntry(d.Entries[i])
		if err != nil {
			return fmt.Errorf("invalid cache entry: %w", err)
		}
		d.Entries[i] = entry
	}
	m.mu.Lock()
	m.data = cloneData(&d)
	m.mu.Unlock()
	return nil
}

func (m *Manager) validateData(d *CacheData) error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported cache schema version %d", d.SchemaVersion)
	}
	if d.Partition != m.scope.Partition || d.AccountID != m.scope.AccountID || d.Region != m.scope.Region || d.Backend != m.scope.Backend {
		return fmt.Errorf("cache scope does not match requested backend scope")
	}
	return nil
}

func (m *Manager) migrateLegacySSM(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var legacy CacheData
	if err := json.Unmarshal(b, &legacy); err != nil {
		return fmt.Errorf("invalid legacy cache file: %w", err)
	}
	migrated := newCacheData(m.scope)
	migrated.LastRefresh = legacy.LastRefresh
	migrated.Incomplete = legacy.Incomplete
	migrated.Complete = !legacy.LastRefresh.IsZero() && !legacy.Incomplete && legacy.LastRefreshError == ""
	migrated.LastRefreshError = legacy.LastRefreshError
	for _, entry := range legacy.Entries {
		// Legacy files predate multiple backends and are SSM-only by definition.
		entry.Identity = aws.ResourceIdentity{}
		qualified, qualifyErr := m.qualifyEntry(entry)
		if qualifyErr != nil {
			return qualifyErr
		}
		migrated.Entries = append(migrated.Entries, qualified)
	}
	return m.withDiskLock(func(d *CacheData) error {
		*d = *migrated
		return nil
	})
}

// withDiskLock serializes a whole read/merge/write transaction. os.Rename
// provides readers with either the old complete JSON document or the new one.
func (m *Manager) withDiskLock(fn func(*CacheData) error) error {
	if err := os.MkdirAll(filepath.Dir(m.cachePath), 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	if err := m.acquireLock(); err != nil {
		return fmt.Errorf("failed to acquire cache lock: %w", err)
	}
	defer m.releaseLock()
	d := newCacheData(m.scope)
	if b, err := os.ReadFile(m.cachePath); err == nil {
		if json.Unmarshal(b, d) != nil || m.validateData(d) != nil {
			d = newCacheData(m.scope)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.cachePath), ".cache-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}
	if err = os.Rename(tmp, m.cachePath); err != nil {
		return fmt.Errorf("failed to replace cache: %w", err)
	}
	m.mu.Lock()
	m.data = cloneData(d)
	m.mu.Unlock()
	return nil
}
func (m *Manager) acquireLock() error {
	for i := 0; i < 50; i++ {
		f, err := os.OpenFile(m.lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_ = f.Close()
			return nil
		}
		if os.IsExist(err) {
			if info, se := os.Stat(m.lockFile); se == nil && time.Since(info.ModTime()) > 5*time.Minute {
				_ = os.Remove(m.lockFile)
				continue
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}
		return err
	}
	return fmt.Errorf("failed to acquire lock after retries")
}
func (m *Manager) releaseLock() { _ = os.Remove(m.lockFile) }

func (m *Manager) IsExpired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data.LastRefresh.IsZero() || !m.data.Complete || m.data.Incomplete || time.Since(m.data.LastRefresh) > m.ttl
}
func (m *Manager) GetAge() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data.LastRefresh.IsZero() {
		return 0
	}
	return time.Since(m.data.LastRefresh)
}
func (m *Manager) GetStats() CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d := m.data
	return CacheStats{TotalEntries: len(d.Entries), LastRefresh: d.LastRefresh, IsExpired: d.LastRefresh.IsZero() || !d.Complete || d.Incomplete || time.Since(d.LastRefresh) > m.ttl, Partition: d.Partition, AccountID: d.AccountID, Region: d.Region, Backend: d.Backend, Complete: d.Complete && !d.Incomplete, LastRefreshError: d.LastRefreshError}
}
func (m *Manager) GetAll() []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CacheEntry, len(m.data.Entries))
	for i, e := range m.data.Entries {
		out[i] = cloneEntry(e)
	}
	return out
}
func (m *Manager) Search(pattern string) []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []CacheEntry
	for _, e := range m.data.Entries {
		ok, err := parammatch.Match(pattern, e.Name)
		if err == nil && ok {
			out = append(out, cloneEntry(e))
		}
	}
	return out
}
func (m *Manager) SearchByTag(key, value string) []CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []CacheEntry
	for _, e := range m.data.Entries {
		if v, ok := e.Tags[key]; ok && (value == "" || v == value) {
			out = append(out, cloneEntry(e))
		}
	}
	return out
}
func (m *Manager) Get(name string) (*CacheEntry, bool) {
	return m.GetByIdentity(m.identity(name))
}

// GetByIdentity retrieves one resource without conflating equal display names
// in different AWS scopes or backends.
func (m *Manager) GetByIdentity(identity aws.ResourceIdentity) (*CacheEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.data.Entries {
		if m.entryIdentity(e) == identity {
			x := cloneEntry(e)
			return &x, true
		}
	}
	return nil, false
}
func entryIdentity(e CacheEntry, scope aws.ResourceIdentity) aws.ResourceIdentity {
	id := e.Identity
	if id.Partition == "" {
		id.Partition = scope.Partition
	}
	if id.AccountID == "" {
		id.AccountID = scope.AccountID
	}
	if id.Region == "" {
		id.Region = scope.Region
	}
	if id.Backend == "" {
		id.Backend = scope.Backend
	}
	if id.CanonicalID == "" {
		id.CanonicalID = e.Name
	}
	return id
}
func upsert(es []CacheEntry, e CacheEntry, scope aws.ResourceIdentity) []CacheEntry {
	for i := range es {
		if entryIdentity(es[i], scope) == e.Identity {
			es[i] = cloneEntry(e)
			return es
		}
	}
	return append(es, cloneEntry(e))
}
func remove(es []CacheEntry, identity aws.ResourceIdentity, scope aws.ResourceIdentity) []CacheEntry {
	for i := range es {
		if entryIdentity(es[i], scope) == identity {
			return append(es[:i], es[i+1:]...)
		}
	}
	return es
}
func (m *Manager) Update(e CacheEntry) error {
	var err error
	e, err = m.qualifyEntry(e)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.changes[e.Identity] = &e
	m.mu.Unlock()
	return m.withDiskLock(func(d *CacheData) error { d.Entries = upsert(d.Entries, e, m.scope); return nil })
}
func (m *Manager) Delete(name string) error {
	return m.DeleteByIdentity(m.identity(name))
}

// DeleteByIdentity removes only the qualified resource.
func (m *Manager) DeleteByIdentity(identity aws.ResourceIdentity) error {
	if err := m.validateIdentity(identity); err != nil {
		return err
	}
	m.mu.Lock()
	m.changes[identity] = nil
	m.mu.Unlock()
	return m.withDiskLock(func(d *CacheData) error { d.Entries = remove(d.Entries, identity, m.scope); return nil })
}

func (m *Manager) identity(canonicalID string) aws.ResourceIdentity {
	id := m.scope
	id.CanonicalID = canonicalID
	return id
}

func (m *Manager) entryIdentity(entry CacheEntry) aws.ResourceIdentity {
	return entryIdentity(entry, m.scope)
}

func (m *Manager) qualifyEntry(entry CacheEntry) (CacheEntry, error) {
	entry = cloneEntry(entry)
	entry.Identity = entryIdentity(entry, m.scope)
	if err := m.validateIdentity(entry.Identity); err != nil {
		return CacheEntry{}, err
	}
	return entry, nil
}

func (m *Manager) validateIdentity(identity aws.ResourceIdentity) error {
	if identity.CanonicalID == "" {
		return fmt.Errorf("cache resource canonical ID is required")
	}
	if identity.Partition != m.scope.Partition || identity.AccountID != m.scope.AccountID || identity.Region != m.scope.Region || identity.Backend != m.scope.Backend {
		return fmt.Errorf("resource identity does not match cache scope")
	}
	return nil
}

type RefreshProgressCallback func(current, total int)

type RefreshClient interface {
	DescribeParametersStream(context.Context, chan<- aws.ParameterMetadata) (aws.DescribeResult, error)
	GetParameterTags(context.Context, string) (map[string]string, error)
}

func (m *Manager) Refresh(ctx context.Context, client RefreshClient, region string, parallel int, cb RefreshProgressCallback) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if parallel < 1 {
		parallel = 1
	}
	m.mu.Lock()
	old := cloneData(m.data)
	// Changes made before a refresh are already represented in old. Only
	// changes after this point need rebasing over the incoming snapshot.
	m.changes = map[aws.ResourceIdentity]*CacheEntry{}
	m.mu.Unlock()
	prior := map[aws.ResourceIdentity]CacheEntry{}
	for _, e := range old.Entries {
		prior[m.entryIdentity(e)] = e
	}
	ch := make(chan aws.ParameterMetadata, 100)
	type streamResult struct {
		result aws.DescribeResult
		err    error
	}
	streamDone := make(chan streamResult, 1)
	go func() {
		result, err := client.DescribeParametersStream(ctx, ch)
		streamDone <- streamResult{result: result, err: err}
	}()
	var es []CacheEntry
	var esMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)
	done := 0
	for p := range ch {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			e := CacheEntry{Identity: m.identity(p.Name), Name: p.Name, Type: p.Type, Version: p.Version, LastModifiedDate: p.LastModifiedDate}
			if old, ok := prior[e.Identity]; ok {
				e.Tags = old.Tags
				e.TagsFetchedAt = old.TagsFetchedAt
				e.TagsComplete = old.TagsComplete
				e.TagsError = old.TagsError
				e.VersionHistory = old.VersionHistory
			}
			if e.TagsFetchedAt.IsZero() || time.Since(e.TagsFetchedAt) >= tagTTL || !e.TagsComplete {
				tags, err := client.GetParameterTags(ctx, p.Name)
				if err != nil {
					e.TagsError = err.Error()
					e.TagsComplete = false
				} else {
					e.Tags = tags
					e.TagsFetchedAt = time.Now()
					e.TagsComplete = true
					e.TagsError = ""
				}
			}
			esMu.Lock()
			es = append(es, e)
			done++
			if cb != nil {
				cb(done, 0)
			}
			esMu.Unlock()
		}()
	}
	wg.Wait()
	stream := <-streamDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	persistErr := m.withDiskLock(func(d *CacheData) error {
		next := newCacheData(m.scope)
		next.LastRefresh = time.Now()
		next.Incomplete = !stream.result.Complete || stream.err != nil
		next.Complete = !next.Incomplete
		next.Entries = es
		if stream.err != nil {
			next.LastRefreshError = stream.err.Error()
		}
		if next.Incomplete {
			for _, e := range d.Entries {
				found := false
				for _, seen := range es {
					if m.entryIdentity(e) == m.entryIdentity(seen) {
						found = true
						break
					}
				}
				if !found {
					next.Entries = append(next.Entries, cloneEntry(e))
				}
			}
		}
		// Another process can have changed the disk snapshot while this refresh
		// was in flight. Rebase those changes when they differ from our starting
		// snapshot, rather than overwriting a successful local mutation.
		start := map[aws.ResourceIdentity]CacheEntry{}
		for _, e := range old.Entries {
			start[m.entryIdentity(e)] = e
		}
		current := map[aws.ResourceIdentity]CacheEntry{}
		for _, e := range d.Entries {
			current[m.entryIdentity(e)] = e
		}
		for identity, before := range start {
			if _, stillPresent := current[identity]; !stillPresent {
				next.Entries = remove(next.Entries, identity, m.scope)
			} else if !reflect.DeepEqual(before, current[identity]) {
				next.Entries = upsert(next.Entries, current[identity], m.scope)
			}
		}
		for identity, now := range current {
			if _, existed := start[identity]; !existed {
				next.Entries = upsert(next.Entries, now, m.scope)
			}
		}
		m.mu.Lock()
		changes := m.changes
		m.changes = map[aws.ResourceIdentity]*CacheEntry{}
		m.mu.Unlock()
		for identity, e := range changes {
			if e == nil {
				next.Entries = remove(next.Entries, identity, m.scope)
			} else {
				next.Entries = upsert(next.Entries, *e, m.scope)
			}
		}
		*d = *next
		return nil
	})
	if persistErr != nil {
		return persistErr
	}
	return stream.err
}

func (m *Manager) Sort(es []CacheEntry, by string) []CacheEntry {
	out := make([]CacheEntry, len(es))
	for i, e := range es {
		out[i] = cloneEntry(e)
	}
	switch by {
	case "name", "n":
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	case "created", "c":
		sort.Slice(out, func(i, j int) bool { return out[i].LastModifiedDate.Before(out[j].LastModifiedDate) })
	case "modified", "m":
		sort.Slice(out, func(i, j int) bool { return out[i].LastModifiedDate.After(out[j].LastModifiedDate) })
	}
	return out
}
func matchGlob(pattern, name string) bool {
	ok, err := parammatch.Match(pattern, name)
	return err == nil && ok
}
