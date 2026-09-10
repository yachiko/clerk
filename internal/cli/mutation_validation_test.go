package cli

import (
	"strings"
	"testing"
)

func TestMutationValidationBeforeAWS(t *testing.T) {
	t.Cleanup(func() {
		globalOpts = GlobalOptions{}
		deleteRecoveryWindow = 0
		deleteForceWithoutRecovery = false
		putDescription = ""
		putType = ""
	})
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"delete", "secret", "--backend", "secretsmanager", "--recovery-window", "6", "--force"}, "between 7 and 30"},
		{[]string{"delete", "secret", "--backend", "secretsmanager", "--recovery-window", "7", "--force-delete-without-recovery", "--force"}, "mutually exclusive"},
		{[]string{"delete", "/parameter", "--recovery-window", "7", "--force"}, "supported only with --backend secretsmanager"},
		{[]string{"put", "secret", "value", "--backend", "secretsmanager", "--type", "String"}, "--type is supported only"},
		{[]string{"put", "/parameter", "value", "--description", "text"}, "--description is supported only"},
		{[]string{"put", "secret", "file://somewhere", "--backend", "secretsmanager", "--stdin"}, "literal value cannot be combined"},
		{[]string{"tag", "secret", "--backend", "secretsmanager"}, "at least one tag is required"},
		{[]string{"untag", "/parameter", "--keys", "a,"}, "tag keys cannot be empty"},
	}
	for _, test := range tests {
		globalOpts = GlobalOptions{}
		deleteRecoveryWindow = 0
		deleteForceWithoutRecovery = false
		putDescription = ""
		putType = ""
		root := NewRootCommand("test", "", "")
		root.SetArgs(test.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%v: error %q, want substring %q", test.args, err, test.want)
		}
	}
}
