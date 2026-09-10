package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
)

func TestBackendFlagDefaultsToSSM(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	globalOpts = GlobalOptions{}
	root := NewRootCommand("test", "", "")
	flag := root.PersistentFlags().Lookup("backend")
	if flag == nil || flag.DefValue != "ssm" || globalOpts.Backend != "ssm" {
		t.Fatalf("backend flag = %#v, value %q", flag, globalOpts.Backend)
	}
}

func TestBackendAwareCommandsDefaultToSSM(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	globalOpts = GlobalOptions{}
	root := NewRootCommand("test", "", "")
	for _, name := range []string{"browse", "cp", "delete", "get", "list", "mv", "put", "refresh", "tag", "untag"} {
		var command *cobra.Command
		for _, candidate := range root.Commands() {
			if candidate.Name() == name {
				command = candidate
				break
			}
		}
		if command == nil {
			t.Fatalf("%s command not found", name)
		}
		backend, err := selectedBackend(command)
		if err != nil || backend != aws.BackendSSM {
			t.Fatalf("%s backend = %q, err=%v", name, backend, err)
		}
	}
}

func TestBackendSelectionValidationRunsBeforeAWS(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"get", "/x", "--backend", "all"}, `invalid backend "all"`},
		{[]string{"delete", "/x", "--force", "--backend", "secretsmanager"}, "requires --recovery-window"},
		{[]string{"cp", "/x", "/y", "--backend", "secretsmanager"}, "cp does not support --backend secretsmanager"},
		{[]string{"mv", "/x", "/y", "--force", "--backend", "secretsmanager"}, "mv does not support --backend secretsmanager"},
		{[]string{"put", "/x", "value", "--backend", "secretsmanager", "--type", "SecureString"}, "--type is supported only"},
		{[]string{"restore", "/x"}, "restore is supported only"},
		{[]string{"refresh", "--backend", "all"}, `invalid backend "all"`},
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

func TestSSMOnlyCommandsAcceptDefaultBackend(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	globalOpts = GlobalOptions{}
	root := NewRootCommand("test", "", "")
	for _, name := range []string{"cp", "delete", "mv", "put"} {
		for _, command := range root.Commands() {
			if command.Name() == name {
				if err := command.PreRunE(command, nil); err != nil {
					t.Errorf("%s rejected default backend: %v", name, err)
				}
			}
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

func TestListOutputContracts(t *testing.T) {
	t.Cleanup(func() { globalOpts = GlobalOptions{} })
	entry := cache.CacheEntry{Identity: aws.ResourceIdentity{Backend: aws.BackendSecretsManager}, Name: "same", Type: "Secret", LastModifiedDate: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)}
	globalOpts.Output = "plain"
	var stdout, stderr bytes.Buffer
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, aws.BackendSecretsManager, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "NAME  TYPE  VERSION  MODIFIED\n") || strings.Contains(stdout.String(), "BACKEND") {
		t.Fatalf("plain output is not single-provider output:\n%s", stdout.String())
	}

	globalOpts.Output = "json"
	stdout.Reset()
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, aws.BackendSecretsManager, false); err != nil {
		t.Fatal(err)
	}
	var items []cache.CacheEntry
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil || len(items) != 1 || strings.Contains(stdout.String(), `"scopes"`) {
		t.Fatalf("invalid single-provider array: %#v, %v", items, err)
	}

	stdout.Reset()
	if err := outputListTo(&stdout, &stderr, []cache.CacheEntry{entry}, aws.BackendSSM, false); err != nil {
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
