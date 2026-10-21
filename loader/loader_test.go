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

func TestFilterByCategory(t *testing.T) {
	sites := []Site{
		{Name: "GitHub", Category: "coding"},
		{Name: "Twitter", Category: "social"},
		{Name: "Behance", Category: "design"},
		{Name: "Chess", Category: "gaming"},
		{Name: "PayPal", Category: "finance"},
	}

	all := FilterByCategory(sites, "")
	if len(all) != 5 {
		t.Errorf("expected 5 sites for empty filter, got %d", len(all))
	}

	codingAndDesign := FilterByCategory(sites, "coding, design")
	if len(codingAndDesign) != 2 {
		t.Fatalf("expected 2 sites for coding,design, got %d", len(codingAndDesign))
	}
	if codingAndDesign[0].Name != "GitHub" || codingAndDesign[1].Name != "Behance" {
		t.Errorf("unexpected filtered sites: %v", codingAndDesign)
	}

	none := FilterByCategory(sites, "nonexistent")
	if len(none) != 0 {
		t.Errorf("expected 0 sites, got %d", len(none))
	}
}

func TestGeneratePermutations(t *testing.T) {
	base := "ashendilantha"
	perms := GeneratePermutations(base)
	if len(perms) == 0 {
		t.Fatalf("expected permutations, got empty")
	}
	if perms[0] != base {
		t.Errorf("expected first element to be %s, got %s", base, perms[0])
	}

	hasSuffix := false
	hasPrefix := false
	for _, p := range perms {
		if p == "ashendilantha_" || p == "ashendilantha-dev" {
			hasSuffix = true
		}
		if p == "realashendilantha" {
			hasPrefix = true
		}
	}
	if !hasSuffix {
		t.Errorf("expected suffix permutation not found")
	}
	if !hasPrefix {
		t.Errorf("expected prefix permutation not found")
	}

	withSeparator := GeneratePermutations("ashen-dev")
	hasUnder := false
	for _, p := range withSeparator {
		if p == "ashen_dev" {
			hasUnder = true
		}
	}
	if !hasUnder {
		t.Errorf("expected separator conversion to _ not found")
	}

	deduped := DedupeUsernames([]string{"user", "User", "other", "user"})
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique names, got %d", len(deduped))
	}
}
