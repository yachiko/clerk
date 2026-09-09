package parammatch

import "testing"

func TestMatchContract(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"/", "/dev/api", true}, {"/dev", "/dev", true}, {"/dev", "/dev/api", false},
		{"/dev/*", "/dev/api", true}, {"/dev/api*", "/dev/api_key", true},
		{"*password*", "/prod/database/password", true}, {"/*/database/*", "/prod/database/password", true},
		{"/Dev/*", "/dev/api", false}, {"flat", "flat", true},
	}
	for _, tc := range cases {
		got, err := Match(tc.pattern, tc.name)
		if err != nil || got != tc.want {
			t.Fatalf("Match(%q,%q) = %v,%v; want %v", tc.pattern, tc.name, got, err, tc.want)
		}
	}
	if _, err := Match("/dev/[", "/dev/x"); err == nil {
		t.Fatal("invalid glob was accepted")
	}
}
