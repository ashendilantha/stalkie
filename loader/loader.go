package loader

import (
	"encoding/json"
	"os"
	"strings"
)

type Site struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	ErrorType     string `json:"errorType"`
	ErrorCode     int    `json:"errorCode"`
	ErrorMsg      string `json:"errorMsg"`
	TitleContains string `json:"titleContains"`
	Weight        int    `json:"weight"`
	Category      string `json:"category,omitempty"`
}

func Load(path string) ([]Site, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sites []Site
	if err := json.Unmarshal(data, &sites); err != nil {
		return nil, err
	}
	return sites, nil
}

func LoadUsernames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			names = append(names, trimmed)
		}
	}
	return names, nil
}

func BuildURL(template, username string) string {
	return strings.ReplaceAll(template, "{}", username)
}

func FilterByCategory(sites []Site, categoryQuery string) []Site {
	trimmedQuery := strings.TrimSpace(categoryQuery)
	if trimmedQuery == "" {
		return sites
	}

	rawCats := strings.Split(trimmedQuery, ",")
	allowed := make(map[string]bool)
	for _, c := range rawCats {
		norm := strings.ToLower(strings.TrimSpace(c))
		if norm != "" {
			allowed[norm] = true
		}
	}

	if len(allowed) == 0 {
		return sites
	}

	var filtered []Site
	for _, s := range sites {
		cat := strings.ToLower(strings.TrimSpace(s.Category))
		if allowed[cat] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
