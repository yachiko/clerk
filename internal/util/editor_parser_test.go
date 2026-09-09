package util

import "testing"

func TestParseEditorCommandPreservesQuotedArgumentsWithoutShell(t *testing.T) {
	args, err := parseEditorCommand(`"/Applications/My Editor" --wait 'two words'`)
	if err != nil { t.Fatal(err) }
	want := []string{"/Applications/My Editor", "--wait", "two words"}
	if len(args) != len(want) { t.Fatalf("got %#v", args) }
	for i := range want { if args[i] != want[i] { t.Fatalf("argument %d = %q, want %q", i, args[i], want[i]) } }
	if _, err := parseEditorCommand(`editor "unterminated`); err == nil { t.Fatal("accepted unmatched quote") }
}
