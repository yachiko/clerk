package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/testutil"
)

func qualifiedManager(t *testing.T, backend aws.Backend) (*Manager, string) {
	t.Helper()
	home := testutil.IsolateHome(t)
	m, err := NewManagerForBackend(&config.Config{CacheTTL: time.Hour}, "aws", "us-east-1", "123456789012", backend)
	if err != nil {
		t.Fatal(err)
	}
	return m, home
}

func TestBackendQualifiedCachesAllowEqualDisplayNames(t *testing.T) {
	home := testutil.IsolateHome(t)
	cfg := &config.Config{CacheTTL: time.Hour}
	ssm, err := NewManagerForBackend(cfg, "aws", "us-east-1", "123456789012", aws.BackendSSM)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := NewManagerForBackend(cfg, "aws", "us-east-1", "123456789012", aws.BackendSecretsManager)
	if err != nil {
		t.Fatal(err)
	}
	if err := ssm.Update(CacheEntry{Name: "shared", Type: "SecureString"}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Update(CacheEntry{Name: "shared", Type: "Secret"}); err != nil {
		t.Fatal(err)
	}

	ssmEntry, ssmOK := ssm.Get("shared")
	secretEntry, secretOK := secrets.Get("shared")
	if !ssmOK || !secretOK || ssmEntry.Identity == secretEntry.Identity {
		t.Fatalf("equal names were not independently qualified: %#v %#v", ssmEntry, secretEntry)
	}
	if ssm.cachePath == secrets.cachePath || ssm.cachePath != filepath.Join(home, ".clerk", "cache", "v2", "aws", "123456789012", "us-east-1", "ssm.json") || secrets.cachePath != filepath.Join(home, ".clerk", "cache", "v2", "aws", "123456789012", "us-east-1", "secretsmanager.json") {
		t.Fatalf("backend paths are not isolated: %q %q", ssm.cachePath, secrets.cachePath)
	}
}

func TestQualifiedUpdateAndDeleteRejectOtherBackends(t *testing.T) {
	m, _ := qualifiedManager(t, aws.BackendSSM)
	ssmID := aws.ResourceIdentity{Partition: "aws", AccountID: "123456789012", Region: "us-east-1", Backend: aws.BackendSSM, CanonicalID: "shared"}
	secretID := ssmID
	secretID.Backend = aws.BackendSecretsManager
	if err := m.Update(CacheEntry{Identity: ssmID, Name: "shared", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(CacheEntry{Identity: secretID, Name: "shared", Version: 2}); err == nil {
		t.Fatal("cross-backend update unexpectedly succeeded")
	}
	if err := m.DeleteByIdentity(secretID); err == nil {
		t.Fatal("cross-backend delete unexpectedly succeeded")
	}
	if entry, ok := m.GetByIdentity(ssmID); !ok || entry.Version != 1 {
		t.Fatalf("qualified SSM entry was changed: %#v", entry)
	}
	if err := m.DeleteByIdentity(ssmID); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.GetByIdentity(ssmID); ok {
		t.Fatal("qualified delete did not remove the SSM entry")
	}
}

func TestLegacyCacheMigratesOnlyAsSSMMetadata(t *testing.T) {
	home := testutil.IsolateHome(t)
	legacyPath := filepath.Join(home, ".clerk", "cache", "123456789012", "us-east-1.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"last_refresh":"2026-01-02T03:04:05Z","region":"us-east-1","entries":[{"name":"shared","type":"SecureString","version":3,"value":"plaintext","binary":"c2VjcmV0","synthetic_secret":true}]}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{CacheTTL: time.Hour}
	secrets, err := NewManagerForBackend(cfg, "aws", "us-east-1", "123456789012", aws.BackendSecretsManager)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets.Get("shared"); ok {
		t.Fatal("legacy SSM entry was interpreted as a Secrets Manager resource")
	}
	ssm, err := NewManagerForBackend(cfg, "aws", "us-east-1", "123456789012", aws.BackendSSM)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := ssm.Get("shared")
	if !ok || entry.Identity.Backend != aws.BackendSSM || entry.Identity.CanonicalID != "shared" {
		t.Fatalf("legacy entry was not qualified as SSM: %#v", entry)
	}
	b, err := os.ReadFile(ssm.cachePath)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(b)
	for _, forbidden := range []string{`"value"`, `"binary"`, `"synthetic_secret"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("migrated cache retained secret-bearing field %s: %s", forbidden, serialized)
		}
	}
	var data CacheData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if data.SchemaVersion != SchemaVersion || data.Partition != "aws" || data.AccountID != "123456789012" || data.Region != "us-east-1" || data.Backend != aws.BackendSSM {
		t.Fatalf("migrated scope metadata is incomplete: %#v", data)
	}
	if !strings.Contains(serialized, `"complete": true`) {
		t.Fatalf("migrated cache lacks explicit completeness metadata: %s", serialized)
	}
}

func TestRefreshPersistsIncompleteErrorSnapshot(t *testing.T) {
	m, _ := qualifiedManager(t, aws.BackendSSM)
	if err := m.Update(CacheEntry{Name: "known", Version: 1}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("discovery failed")
	err := m.Refresh(context.Background(), refreshFake{entries: []aws.ParameterMetadata{{Name: "partial", Version: 2}}, result: aws.DescribeResult{Complete: false}, err: wantErr}, "us-east-1", 1, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("refresh error = %v, want %v", err, wantErr)
	}
	stats := m.GetStats()
	if stats.Complete || !stats.IsExpired || stats.LastRefreshError != wantErr.Error() {
		t.Fatalf("failed refresh status was not retained: %#v", stats)
	}
	b, readErr := os.ReadFile(m.cachePath)
	if readErr != nil || !strings.Contains(string(b), `"complete": false`) || !strings.Contains(string(b), `"last_refresh_error": "discovery failed"`) {
		t.Fatalf("failed snapshot metadata was not serialized: %v %s", readErr, b)
	}
	if _, ok := m.Get("known"); !ok {
		t.Fatal("incomplete snapshot incorrectly removed a previously known entry")
	}
	if _, ok := m.Get("partial"); !ok {
		t.Fatal("partial metadata was not retained")
	}
}
