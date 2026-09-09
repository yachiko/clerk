package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yachiko/clerk/internal/aws"
)

type refreshFake struct {
	entries []aws.ParameterMetadata
	result  aws.DescribeResult
	err     error
	tags    map[string]string
}

func (f refreshFake) DescribeParametersStream(_ context.Context, out chan<- aws.ParameterMetadata) (aws.DescribeResult, error) {
	defer close(out)
	for _, entry := range f.entries {
		out <- entry
	}
	return f.result, f.err
}

func (f refreshFake) GetParameterTags(context.Context, string) (map[string]string, error) {
	return f.tags, nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.json")
	scope := aws.ResourceIdentity{Partition: "aws", AccountID: "123456789012", Region: "us-east-1", Backend: aws.BackendSSM}
	return &Manager{
		cachePath: path, lockFile: path + ".lock", ttl: time.Hour,
		scope: scope, data: newCacheData(scope), changes: map[aws.ResourceIdentity]*CacheEntry{},
	}
}

func TestRefreshDoesNotReplayMutationsThatPredateItsSnapshot(t *testing.T) {
	m := newTestManager(t)
	if err := m.Update(CacheEntry{Name: "/api/key", Version: 1}); err != nil {
		t.Fatal(err)
	}

	client := refreshFake{
		entries: []aws.ParameterMetadata{{Name: "/api/key", Type: "String", Version: 2}},
		result:  aws.DescribeResult{Complete: true},
		tags:    map[string]string{"env": "test"},
	}
	if err := m.Refresh(context.Background(), client, "us-east-1", 1, nil); err != nil {
		t.Fatal(err)
	}
	entry, ok := m.Get("/api/key")
	if !ok || entry.Version != 2 || entry.Tags["env"] != "test" {
		t.Fatalf("refresh was overwritten by a stale mutation: %#v", entry)
	}
}

func TestRefreshWaitsForStreamResultBeforeDeclaringCoverage(t *testing.T) {
	m := newTestManager(t)
	client := refreshFake{
		entries: []aws.ParameterMetadata{{Name: "/one", Type: "String", Version: 1}},
		result:  aws.DescribeResult{Truncated: true},
	}
	if err := m.Refresh(context.Background(), client, "us-east-1", 1, nil); err != nil {
		t.Fatal(err)
	}
	if !m.GetStats().IsExpired {
		t.Fatal("a truncated snapshot must remain expired/incomplete")
	}
}
