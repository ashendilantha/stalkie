package checker

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ashendilantha/stalkie/loader"
)

func TestEvaluateResponse(t *testing.T) {
	site := loader.Site{
		Name:          "TestGitHub",
		URL:           "https://example.com/{}",
		ErrorType:     "status_code",
		ErrorCode:     404,
		TitleContains: "GitHub",
		Weight:        10,
	}

	found, score, _ := evaluateResponse(site, "ashendilantha", 404, "Not Found", "https://example.com/ashendilantha")
	if found || score != 0 {
		t.Errorf("expected 404 to return false and 0 score, got found=%v score=%d", found, score)
	}

	bodyReal := "<html><head><title>ashendilantha - GitHub</title></head><body>ashendilantha profile content</body></html>"
	foundReal, scoreReal, _ := evaluateResponse(site, "ashendilantha", 200, bodyReal, "https://example.com/ashendilantha")
	if !foundReal || scoreReal < 80 {
		t.Errorf("expected valid profile to be found with high confidence, got found=%v score=%d", foundReal, scoreReal)
	}

	bodySoft404 := "<html><head><title>Page Not Found</title></head><body>This page does not exist</body></html>"
	foundSoft, scoreSoft, _ := evaluateResponse(site, "nonexistent", 200, bodySoft404, "https://example.com/nonexistent")
	if foundSoft || scoreSoft != 0 {
		t.Errorf("expected soft 404 to be rejected, got found=%v score=%d", foundSoft, scoreSoft)
	}
}

func TestCheckLiveMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ashendilantha" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><head><title>ashendilantha Profile</title></head><body>ashendilantha info</body></html>"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	site := loader.Site{
		Name:      "MockService",
		URL:       server.URL + "/{}",
		ErrorType: "status_code",
		ErrorCode: 404,
		Weight:    8,
	}

	client := server.Client()
	client.Timeout = 2 * time.Second

	resValid := Check(client, site, "ashendilantha", 0, 0)
	if !resValid.Found {
		t.Errorf("expected found for valid user, got false")
	}

	resFake := Check(client, site, "nonexistent_user", 0, 0)
	if resFake.Found {
		t.Errorf("expected false for fake user, got true")
	}
}

