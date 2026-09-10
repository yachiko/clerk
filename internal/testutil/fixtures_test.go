package testutil

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestFixtureSecretsAreUsefulAndDeterministic(t *testing.T) {
	first, second := fixtureSecrets(), fixtureSecrets()
	if len(first) != 4 || len(second) != len(first) {
		t.Fatalf("fixture secret count = %d, want 4", len(first))
	}
	for i := range first {
		if first[i].name != second[i].name || first[i].value != second[i].value || string(first[i].binary) != string(second[i].binary) {
			t.Fatalf("fixture %d is not deterministic", i)
		}
	}
	if first[0].name != "/dev/database/password" || !first[0].versioned {
		t.Fatalf("versioned duplicate fixture is not configured correctly: %#v", first[0])
	}
	if len(first[2].value) == 0 || first[3].binary == nil {
		t.Fatal("text/JSON and binary fixture values must be populated")
	}
}

func TestSecretTagsConvertsAllEntries(t *testing.T) {
	tags := secretTags(map[string]string{"env": "dev", "kind": "json"})
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	seen := make(map[string]string, len(tags))
	for _, tag := range tags {
		seen[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	if seen["env"] != "dev" || seen["kind"] != "json" {
		t.Fatalf("unexpected tags: %#v", seen)
	}
}
