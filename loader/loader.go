package loader

import (
	"encoding/json"
	"os"
	"strings"
)

type Site struct {
	Name string `json:"name"`
	URL string `json:"url"`
	ErrorType string `json:"errorType"`
	ErrorCode int `json:"errorCode"`
	ErrorMsg string `json:"errorMsg"`
	TitleContains string `json:"titleContains"`
	Weight int `json:"weight"`
}

func Load(path string) ([]Site, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return  nil, err
	}
	var sites []Site
	return sites, json.Unmarshal(data, &sites)
}

//Load usernames (one username per line) from a file
func LoadUsernames(path string) ([]string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var names []string
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if line != "" && !strings.HasPrefix(line, "#") {
            names = append(names, line)
        }
    }
    return names, nil
}

func BuildURL(template, username string) string {
	return strings.ReplaceAll(template, "{}", username)
}