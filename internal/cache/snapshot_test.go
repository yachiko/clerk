package cache

import (
	"testing"

	"github.com/yachiko/clerk/internal/aws"
)

func TestReplaceSnapshotQualifiesAndMarksComplete(t *testing.T) {
	manager, _ := qualifiedManager(t, aws.BackendSecretsManager)
	if err := manager.ReplaceSnapshot([]CacheEntry{{Identity: aws.ResourceIdentity{Partition: "aws", AccountID: "123456789012", Region: "us-east-1", Backend: aws.BackendSecretsManager, CanonicalID: "arn:secret"}, Name: "shared", Type: "Secret"}}); err != nil {
		t.Fatal(err)
	}
	stats := manager.GetStats()
	entries := manager.GetAll()
	if !stats.Complete || stats.IsExpired || len(entries) != 1 || entries[0].Identity.Backend != aws.BackendSecretsManager {
		t.Fatalf("stats=%#v entries=%#v", stats, entries)
	}
}
