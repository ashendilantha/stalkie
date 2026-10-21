package syncer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ashendilantha/stalkie/loader"
)

func TestNormalizeWMNSite(t *testing.T) {
	cases := []struct {
		name        string
		input       wmnSite
		expectValid bool
		expectURL   string
		expectType  string
		expectCode  int
		expectMsg   string
		expectTitle string
		expectW     int
	}{
		{
			name: "Standard site with account placeholder",
			input: wmnSite{
				Name:     "TestPlatform",
				URICheck: "https://example.com/user/{account}/",
				MCode:    404,
				MString:  "User not found",
				EString:  "User profile",
				Cat:      "coding",
			},
			expectValid: true,
			expectURL:   "https://example.com/user/{}/",
			expectType:  "status_code",
			expectCode:  404,
			expectMsg:   "User not found",
			expectTitle: "User profile",
			expectW:     8,
		},
		{
			name: "Site with message error type",
			input: wmnSite{
				Name:     "SocialApp",
				URICheck: "https://social.example.com/{}",
				MCode:    200,
				MString:  "No such account exists",
				Cat:      "social",
			},
			expectValid: true,
			expectURL:   "https://social.example.com/{}",
			expectType:  "message",
			expectCode:  0,
			expectMsg:   "No such account exists",
			expectTitle: "",
			expectW:     7,
		},
		{
			name: "Invalid missing placeholder",
			input: wmnSite{
				Name:     "BrokenSite",
				URICheck: "https://example.com/static",
			},
			expectValid: false,
		},
		{
			name: "Invalid empty name",
			input: wmnSite{
				Name:     "",
				URICheck: "https://example.com/{account}",
			},
			expectValid: false,
		},
		{
			name: "Invalid non-http scheme",
			input: wmnSite{
				Name:     "FTPTarget",
				URICheck: "ftp://example.com/{account}",
			},
			expectValid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site, ok := normalizeWMNSite(tc.input)
			if ok != tc.expectValid {
				t.Fatalf("expected valid=%v, got %v", tc.expectValid, ok)
			}
			if !tc.expectValid {
				return
			}
			if site.URL != tc.expectURL {
				t.Errorf("expected URL %q, got %q", tc.expectURL, site.URL)
			}
			if site.ErrorType != tc.expectType {
				t.Errorf("expected ErrorType %q, got %q", tc.expectType, site.ErrorType)
			}
			if site.ErrorCode != tc.expectCode {
				t.Errorf("expected ErrorCode %d, got %d", tc.expectCode, site.ErrorCode)
			}
			if site.ErrorMsg != tc.expectMsg {
				t.Errorf("expected ErrorMsg %q, got %q", tc.expectMsg, site.ErrorMsg)
			}
			if site.TitleContains != tc.expectTitle {
				t.Errorf("expected TitleContains %q, got %q", tc.expectTitle, site.TitleContains)
			}
			if site.Weight != tc.expectW {
				t.Errorf("expected Weight %d, got %d", tc.expectW, site.Weight)
			}
		})
	}
}

func TestSyncPipeline(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "test_sites.json")

	initialCurated := []loader.Site{
		{
			Name:          "GitHub",
			URL:           "https://github.com/{}",
			ErrorType:     "status_code",
			ErrorCode:     404,
			TitleContains: "GitHub",
			Weight:        10,
		},
	}
	curatedData, err := json.Marshal(initialCurated)
	if err != nil {
		t.Fatalf("failed preparing curated fixture: %v", err)
	}
	if err := os.WriteFile(targetFile, curatedData, 0644); err != nil {
		t.Fatalf("failed writing curated fixture: %v", err)
	}

	mockPayload := wmnPayload{
		Sites: []wmnSite{
			{
				Name:     "github",
				URICheck: "https://github.com/{account}",
				MCode:    404,
				MString:  "404 Not Found",
				Cat:      "coding",
			},
			{
				Name:     "DockerHub",
				URICheck: "https://hub.docker.com/u/{account}/",
				MCode:    404,
				MString:  "User not found",
				Cat:      "coding",
			},
			{
				Name:     "BrokenSite",
				URICheck: "invalid-url",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockPayload)
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 5 * time.Second

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed creating test request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed mock request: %v", err)
	}
	defer resp.Body.Close()

	var payload wmnPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed decoding mock payload: %v", err)
	}

	merged := make(map[string]loader.Site)
	for _, ws := range payload.Sites {
		if s, ok := normalizeWMNSite(ws); ok {
			merged[strings.ToLower(s.Name)] = s
		}
	}

	existingSites, err := loader.Load(targetFile)
	if err != nil {
		t.Fatalf("failed loading target fixture: %v", err)
	}
	for _, cur := range existingSites {
		if cur.TitleContains != "" || cur.ErrorType == "status_code" {
			merged[strings.ToLower(cur.Name)] = cur
		}
	}

	if len(merged) != 2 {
		t.Fatalf("expected 2 valid sites after merge, got %d", len(merged))
	}

	gh := merged["github"]
	if gh.TitleContains != "GitHub" || gh.Weight != 10 {
		t.Errorf("curated override for GitHub was lost: %+v", gh)
	}

	docker := merged["dockerhub"]
	if docker.URL != "https://hub.docker.com/u/{}/" {
		t.Errorf("expected normalized DockerHub URL, got %q", docker.URL)
	}
}

