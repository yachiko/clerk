package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
)

func TestBackendFlagDefaultsToAll(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	globalOpts = GlobalOptions{}
	root := NewRootCommand("test", "", "")
	flag := root.PersistentFlags().Lookup("backend")
	if flag == nil || flag.DefValue != "all" || globalOpts.Backend != "all" {
		t.Fatalf("backend flag = %#v, value %q", flag, globalOpts.Backend)
	}
}

func TestRefreshKeepsSSMCompatibilityDefault(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	globalOpts = GlobalOptions{}
	root := NewRootCommand("test", "", "")
	var refreshCmd *cobra.Command
	for _, command := range root.Commands() {
		if command.Name() == "refresh" {
			refreshCmd = command
			break
		}
	}
	if refreshCmd == nil {
		t.Fatal("refresh command not found")
	}
	backend, err := refreshBackend(refreshCmd)
	if err != nil || backend != aws.BackendSSM {
		t.Fatalf("refresh backend = %q, err=%v", backend, err)
	}
}

func TestDirectAndMutationBackendValidationRunsBeforeAWS(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"get", "/x"}, "get requires an explicit --backend"},
		{[]string{"get", "/x", "--backend", "all"}, "get requires one concrete backend"},
		{[]string{"put", "/x", "value"}, "put requires explicit --backend ssm"},
		{[]string{"delete", "/x", "--force", "--backend", "secretsmanager"}, "delete is supported only with --backend ssm"},
		{[]string{"cp", "/x", "/y", "--backend", "all"}, "cp is supported only with --backend ssm"},
		{[]string{"mv", "/x", "/y", "--force", "--backend", "secretsmanager"}, "mv is supported only with --backend ssm"},
		{[]string{"refresh", "--backend", "all"}, "refresh requires one concrete backend"},
	}
	for _, test := range tests {
		globalOpts = GlobalOptions{}
		root := NewRootCommand("test", "", "")
		root.SetArgs(test.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%v: error %q, want substring %q", test.args, err, test.want)
		}
	}
}

func TestGetSelectorValidation(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"get", "secret", "--backend", "secretsmanager", "--stage", "AWSCURRENT", "--version-id", "opaque"}, "mutually exclusive"},
		{[]string{"get", "secret", "--backend", "secretsmanager", "--raw"}, "valid only with --value"},
		{[]string{"get", "/x", "--backend", "ssm", "--stage", "prod"}, "supported only with --backend secretsmanager"},
	}
	for _, test := range tests {
		globalOpts = GlobalOptions{}
		root := NewRootCommand("test", "", "")
		root.SetArgs(test.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%v: error %q, want substring %q", test.args, err, test.want)
		}
	}
}

func TestAggregateMetadataKeepsEqualNamesAndPartialScope(t *testing.T) {
	now := time.Now()
	providers := []metadataProvider{
		{scope: discoveryScope{Backend: aws.BackendSSM}, list: func(context.Context, string, bool) ([]cache.CacheEntry, error) {
			return []cache.CacheEntry{{Identity: aws.ResourceIdentity{Backend: aws.BackendSSM}, Name: "shared", LastModifiedDate: now}}, nil
		}},
		{scope: discoveryScope{Backend: aws.BackendSecretsManager}, list: func(context.Context, string, bool) ([]cache.CacheEntry, error) {
			return nil, errors.New("access denied")
		}},
	}
	items, scopes, err := aggregateMetadata(context.Background(), providers, "*", false)
	if err == nil || len(items) != 1 || len(scopes) != 2 || scopes[0].Status != "succeeded" || scopes[1].Status != "failed" || discoveryCompleteness(scopes) != "partial" {
		t.Fatalf("items=%#v scopes=%#v err=%v", items, scopes, err)
	}

	providers[1].list = func(context.Context, string, bool) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{{Identity: aws.ResourceIdentity{Backend: aws.BackendSecretsManager}, Name: "shared", LastModifiedDate: now}}, nil
	}
	items, scopes, err = aggregateMetadata(context.Background(), providers, "*", false)
	if err != nil || len(items) != 2 || items[0].Identity.Backend == items[1].Identity.Backend || discoveryCompleteness(scopes) != "complete" {
		t.Fatalf("equal names were conflated: items=%#v scopes=%#v err=%v", items, scopes, err)
	}
}

func TestListOutputContracts(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	entry := cache.CacheEntry{Identity: aws.ResourceIdentity{Backend: aws.BackendSecretsManager}, Name: "same", Type: "Secret", LastModifiedDate: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)}
	scope := discoveryScope{Backend: aws.BackendSecretsManager, Status: "succeeded", Complete: true}
	globalOpts.Output = "plain"
	var stdout, stderr bytes.Buffer
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, []discoveryScope{scope}, aws.BackendSecretsManager, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "BACKEND  NAME  TYPE  VERSION  MODIFIED\n") || !strings.Contains(stdout.String(), "secretsmanager  same") {
		t.Fatalf("plain output lacks textual backend column:\n%s", stdout.String())
	}

	globalOpts.Output = "json"
	stdout.Reset()
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, []discoveryScope{scope}, aws.BackendSecretsManager, false); err != nil {
		t.Fatal(err)
	}
	var envelope discoveryEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.SchemaVersion != 1 || envelope.Completeness != "complete" || len(envelope.Items) != 1 || len(envelope.Scopes) != 1 {
		t.Fatalf("invalid envelope: %#v, %v", envelope, err)
	}

	stdout.Reset()
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, []discoveryScope{scope}, aws.BackendSSM, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), `"items"`) || !strings.HasPrefix(strings.TrimSpace(stdout.String()), "[") {
		t.Fatalf("explicit SSM JSON is not legacy array: %s", stdout.String())
	}
}

func TestSecretsManagerValueOutput(t *testing.T) {
	t.Cleanup(func() {
		globalOpts = GlobalOptions{}
		getValueOnly, getRaw, getMask = false, false, false
	})
	id := aws.ResourceIdentity{Backend: aws.BackendSecretsManager}
	binary := &aws.SecretDetail{Name: "binary", ARN: "arn", Value: aws.NewBinaryValue(id, []byte{0x00, 0xff, 'A'}), VersionID: "v1"}
	globalOpts.Output, getValueOnly, getRaw, getMask = "plain", true, false, false
	var output bytes.Buffer
	if err := outputSecretTo(&output, binary); err != nil || output.String() != "AP9B" {
		t.Fatalf("base64 output = %q, err=%v", output.String(), err)
	}
	output.Reset()
	getRaw = true
	if err := outputSecretTo(&output, binary); err != nil || !bytes.Equal(output.Bytes(), []byte{0x00, 0xff, 'A'}) {
		t.Fatalf("raw output = %v, err=%v", output.Bytes(), err)
	}

	text := &aws.SecretDetail{Name: "text", ARN: "arn", Value: aws.NewTextValue(id, " exact\nvalue\x00")}
	output.Reset()
	getRaw = false
	if err := outputSecretTo(&output, text); err != nil || output.String() != " exact\nvalue\x00" {
		t.Fatalf("text output = %q, err=%v", output.String(), err)
	}

	globalOpts.Output, getValueOnly = "json", false
	output.Reset()
	if err := outputSecretTo(&output, binary); err != nil || !strings.Contains(output.String(), `"value_encoding": "base64"`) || !strings.Contains(output.String(), `"value": "AP9B"`) {
		t.Fatalf("binary JSON = %s, err=%v", output.String(), err)
	}
	getMask = true
	if err := outputSecretTo(&output, binary); err == nil || !strings.Contains(err.Error(), "not valid for binary") {
		t.Fatalf("binary mask error = %v", err)
	}
}
