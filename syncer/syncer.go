package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ashendilantha/stalkie/loader"
)

const (
	wmnURL      = "https://raw.githubusercontent.com/WebBreacher/WhatsMyName/main/wmn-data.json"
	sherlockURL = "https://raw.githubusercontent.com/sherlock-project/sherlock/master/sherlock_project/resources/data.json"
)

type wmnPayload struct {
	Sites []wmnSite `json:"sites"`
}

type wmnSite struct {
	Name     string   `json:"name"`
	URICheck string   `json:"uri_check"`
	ECode    int      `json:"e_code"`
	EString  string   `json:"e_string"`
	MCode    int      `json:"m_code"`
	MString  string   `json:"m_string"`
	Known    []string `json:"known"`
	Cat      string   `json:"cat"`
}

type sherlockEntry struct {
	ErrorType string          `json:"errorType"`
	ErrorCode int             `json:"errorCode"`
	ErrorMsg  json.RawMessage `json:"errorMsg"`
	URL       string          `json:"url"`
}

func Sync(targetPath string, timeout time.Duration) (int, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
	}

	existingMap := make(map[string]loader.Site)
	if existingSites, err := loader.Load(targetPath); err == nil {
		for _, s := range existingSites {
			key := strings.ToLower(strings.TrimSpace(s.Name))
			existingMap[key] = s
		}
	}

	wmnSites, wmnErr := fetchWhatsMyName(client)
	if wmnErr != nil {
		return 0, fmt.Errorf("failed fetching WhatsMyName: %w", wmnErr)
	}

	mergedMap := make(map[string]loader.Site)

	for _, ws := range wmnSites {
		site, ok := normalizeWMNSite(ws)
		if !ok {
			continue
		}
		key := strings.ToLower(site.Name)
		mergedMap[key] = site
	}

	for key, curated := range existingMap {
		if curated.TitleContains != "" || curated.ErrorType == "status_code" {
			mergedMap[key] = curated
		}
	}

	var finalList []loader.Site
	for _, s := range mergedMap {
		finalList = append(finalList, s)
	}

	sort.Slice(finalList, func(i, j int) bool {
		return strings.ToLower(finalList[i].Name) < strings.ToLower(finalList[j].Name)
	})

	data, err := json.MarshalIndent(finalList, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("failed formatting sites json: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return 0, fmt.Errorf("failed writing to %s: %w", targetPath, err)
	}

	return len(finalList), nil
}

func fetchWhatsMyName(client *http.Client) ([]wmnSite, error) {
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wmnURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "stalkie-db-syncer/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}

	var payload wmnPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json schema: %w", err)
	}

	return payload.Sites, nil
}

func normalizeWMNSite(s wmnSite) (loader.Site, bool) {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return loader.Site{}, false
	}

	targetURL := strings.TrimSpace(s.URICheck)
	if strings.Contains(targetURL, "{account}") {
		targetURL = strings.ReplaceAll(targetURL, "{account}", "{}")
	}

	if !strings.Contains(targetURL, "{}") {
		return loader.Site{}, false
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return loader.Site{}, false
	}

	errorType := "message"
	errorCode := 0
	if s.MCode != 0 && s.MCode != 200 {
		errorType = "status_code"
		errorCode = s.MCode
	}

	errorMsg := strings.TrimSpace(s.MString)
	titleContains := ""
	cleanE := strings.TrimSpace(s.EString)
	if cleanE != "" && len(cleanE) <= 60 && !strings.ContainsAny(cleanE, "<>{}\n\r\t") {
		titleContains = cleanE
	}

	weight := 6
	switch strings.ToLower(s.Cat) {
	case "coding", "tech":
		weight = 8
	case "social":
		weight = 7
	case "gaming", "art":
		weight = 6
	}

	return loader.Site{
		Name:          name,
		URL:           targetURL,
		ErrorType:     errorType,
		ErrorCode:     errorCode,
		ErrorMsg:      errorMsg,
		TitleContains: titleContains,
		Weight:        weight,
	}, true
}

