package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildURL(t *testing.T) {
	cases := []struct {
		template string
		username string
		expected string
	}{
		{"https://github.com/{}", "ashendilantha", "https://github.com/ashendilantha"},
		{"https://twitter.com/{}", "testuser", "https://twitter.com/testuser"},
		{"https://example.com/u/{}/profile", "user123", "https://example.com/u/user123/profile"},
	}

	for _, tc := range cases {
		got := BuildURL(tc.template, tc.username)
		if got != tc.expected {
			t.Errorf("BuildURL(%q, %q) = %q; want %q", tc.template, tc.username, got, tc.expected)
		}
	}
}

func TestLoadUsernames(t *testing.T) {
	content := "user1\n# comment line\n\nuser2\n   user3   \n"
	tmpFile := filepath.Join(t.TempDir(), "users.txt")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed creating test usernames file: %v", err)
	}

	names, err := LoadUsernames(tmpFile)
	if err != nil {
		t.Fatalf("LoadUsernames failed: %v", err)
	}

	expected := []string{"user1", "user2", "user3"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("name[%d] = %q; want %q", i, name, expected[i])
		}
	}
}

