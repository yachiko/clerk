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
	cachePath         string
	ttl               time.Duration
	data              *CacheData
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	lockFile          string
	region, accountID string
	changes           map[string]*CacheEntry
}

func NewManager(cfg *config.Config, region, accountID string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	p := filepath.Join(home, ".clerk", "cache", accountID, region+".json")
	m := &Manager{cachePath: p, ttl: cfg.CacheTTL, lockFile: p + ".lock", region: region, accountID: accountID, data: &CacheData{Entries: []CacheEntry{}}, changes: map[string]*CacheEntry{}}
	_ = m.load() // corrupt caches are cache misses
	return m, nil
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
	m.mu.Lock()
	m.data = cloneData(&d)
	m.mu.Unlock()
	return nil
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
	d := &CacheData{Entries: []CacheEntry{}}
	if b, err := os.ReadFile(m.cachePath); err == nil {
		if json.Unmarshal(b, d) != nil {
			d = &CacheData{Entries: []CacheEntry{}}
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
	defer os.Remove(tmp)
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
	return m.data.LastRefresh.IsZero() || m.data.Incomplete || time.Since(m.data.LastRefresh) > m.ttl
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
	return CacheStats{TotalEntries: len(d.Entries), LastRefresh: d.LastRefresh, IsExpired: d.LastRefresh.IsZero() || d.Incomplete || time.Since(d.LastRefresh) > m.ttl, Region: d.Region, Complete: !d.Incomplete}
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.data.Entries {
		if e.Name == name {
			x := cloneEntry(e)
			return &x, true
		}
	}
	return nil, false
}
func upsert(es []CacheEntry, e CacheEntry) []CacheEntry {
	for i := range es {
		if es[i].Name == e.Name {
			es[i] = cloneEntry(e)
			return es
		}
	}
	return append(es, cloneEntry(e))
}
func remove(es []CacheEntry, name string) []CacheEntry {
	for i := range es {
		if es[i].Name == name {
			return append(es[:i], es[i+1:]...)
		}
	}
	return es
}
func (m *Manager) Update(e CacheEntry) error {
	e = cloneEntry(e)
	m.mu.Lock()
	m.changes[e.Name] = &e
	m.mu.Unlock()
	return m.withDiskLock(func(d *CacheData) error { d.Entries = upsert(d.Entries, e); return nil })
}
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	m.changes[name] = nil
	m.mu.Unlock()
	return m.withDiskLock(func(d *CacheData) error { d.Entries = remove(d.Entries, name); return nil })
}

type RefreshProgressCallback func(current, total int)

func (m *Manager) Refresh(ctx context.Context, client *aws.Client, region string, parallel int, cb RefreshProgressCallback) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if parallel < 1 {
		parallel = 1
	}
	m.mu.RLock()
	old := cloneData(m.data)
	m.mu.RUnlock()
	prior := map[string]CacheEntry{}
	for _, e := range old.Entries {
		prior[e.Name] = e
	}
	ch := make(chan aws.ParameterMetadata, 100)
	var result aws.DescribeResult
	var streamErr error
	go func() { result, streamErr = client.DescribeParametersStream(ctx, ch) }()
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
			e := CacheEntry{Name: p.Name, Type: p.Type, Version: p.Version, LastModifiedDate: p.LastModifiedDate}
			if old, ok := prior[p.Name]; ok {
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
	if streamErr != nil {
		return streamErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return m.withDiskLock(func(d *CacheData) error {
		next := &CacheData{LastRefresh: time.Now(), Region: region, Incomplete: !result.Complete, Entries: es}
		if !result.Complete {
			for _, e := range d.Entries {
				found := false
				for _, seen := range es {
					if e.Name == seen.Name {
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
		start := map[string]CacheEntry{}
		for _, e := range old.Entries {
			start[e.Name] = e
		}
		current := map[string]CacheEntry{}
		for _, e := range d.Entries {
			current[e.Name] = e
		}
		for name, before := range start {
			if _, stillPresent := current[name]; !stillPresent {
				next.Entries = remove(next.Entries, name)
			} else if !reflect.DeepEqual(before, current[name]) {
				next.Entries = upsert(next.Entries, current[name])
			}
		}
		for name, now := range current {
			if _, existed := start[name]; !existed {
				next.Entries = upsert(next.Entries, now)
			}
		}
		m.mu.Lock()
		changes := m.changes
		m.changes = map[string]*CacheEntry{}
		m.mu.Unlock()
		for name, e := range changes {
			if e == nil {
				next.Entries = remove(next.Entries, name)
			} else {
				next.Entries = upsert(next.Entries, *e)
			}
		}
		*d = *next
		return nil
	})
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
