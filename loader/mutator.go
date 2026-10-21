package loader

import (
	"strconv"
	"strings"
	"unicode"
)

func GeneratePermutations(base string) []string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	add := func(candidate string) {
		c := strings.TrimSpace(candidate)
		if c == "" {
			return
		}
		low := strings.ToLower(c)
		if !seen[low] {
			seen[low] = true
			result = append(result, c)
		}
	}

	add(trimmed)

	suffixes := []string{
		"_",
		"-dev",
		"_dev",
		"01",
		"1",
		"_official",
		".official",
		"_real",
		"hq",
	}
	for _, sfx := range suffixes {
		add(trimmed + sfx)
	}

	prefixes := []string{
		"real",
		"the",
		"iam",
		"i_am_",
		"official",
	}
	for _, pfx := range prefixes {
		add(pfx + trimmed)
	}

	if strings.Contains(trimmed, "_") {
		add(strings.ReplaceAll(trimmed, "_", "-"))
		add(strings.ReplaceAll(trimmed, "_", "."))
	}
	if strings.Contains(trimmed, "-") {
		add(strings.ReplaceAll(trimmed, "-", "_"))
		add(strings.ReplaceAll(trimmed, "-", "."))
	}
	if strings.Contains(trimmed, ".") {
		add(strings.ReplaceAll(trimmed, ".", "_"))
		add(strings.ReplaceAll(trimmed, ".", "-"))
	}

	idx := len(trimmed) - 1
	for idx >= 0 && unicode.IsDigit(rune(trimmed[idx])) {
		idx--
	}
	if idx < len(trimmed)-1 {
		prefix := trimmed[:idx+1]
		numStr := trimmed[idx+1:]
		if n, err := strconv.Atoi(numStr); err == nil {
			add(prefix)
			add(prefix + strconv.Itoa(n+1))
			if n > 1 {
				add(prefix + strconv.Itoa(n-1))
			}
			if len(numStr) == 1 {
				add(prefix + "0" + numStr)
			}
		}
	}

	return result
}

func DedupeUsernames(names []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, n := range names {
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			continue
		}
		low := strings.ToLower(trimmed)
		if !seen[low] {
			seen[low] = true
			unique = append(unique, trimmed)
		}
	}
	return unique
}

