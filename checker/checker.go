package checker

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ashendilantha/stalkie/loader"
)

type Result struct {
	Username   string
	Site       string
	URL        string
	Found      bool
	Confidence int
	Evidence   string
	Error      string
	Duration   time.Duration
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/121.0",
}

var uaIndex int

func nextUserAgent() string {
	ua := userAgents[uaIndex%len(userAgents)]
	uaIndex++
	return ua
}

func Check(client *http.Client, site loader.Site, username string, maxRetries int, retryDelay time.Duration) Result {
	url := loader.BuildURL(site.URL, username)
	result := Result{
		Username: username,
		Site:     site.Name,
		URL:      url,
	}

	var resp *http.Response
	var body string
	var err error
	start := time.Now()

	//retry logic
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		req, reqErr := http.NewRequest("GET", url, nil)
		if reqErr != nil {
			result.Error = reqErr.Error()
			continue
		}
		req.Header.Set("User-Agent", nextUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err = client.Do(req)
		if err == nil {
			break // successfull req
		}
	}

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	defer resp.Body.Close()

	//read body with limit to 64kb
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	body = string(bodyBytes)

	result.Found, result.Confidence, result.Evidence = score(site, username, resp, body)
	return result
}

// score confidence level
func score(site loader.Site, username string, resp *http.Response, body string) (found bool, confidence int, evidence string) {
	var signals []string
	score := 0

	if hasNotFoundContent(site, body) {
		return false, 0, "profile not found (not-found markers detected)"
	}

	//check status code
	switch site.ErrorType {
	case "status_code":
		if resp.StatusCode == 200 {
			score += 40
			signals = append(signals, "HTTP 200")
		} else if resp.StatusCode == site.ErrorCode {
			return false, 0, "profile not found (status code)"
		}
	case "message":
		if strings.Contains(normalizeForMatch(body), normalizeForMatch(site.ErrorMsg)) {
			return false, 0, "profile not found (error message detected)"
		}
		score += 40
		signals = append(signals, "No error msg")
	}

	//page title signal
	if site.TitleContains != "" {
		expected := strings.ReplaceAll(site.TitleContains, "{}", username)
		if titleContains(body, expected) {
			score += 35
			signals = append(signals, "title match")
		}
	}

	//username should appear multiple times in real profile pages; single mention is often just URL echo
	if countUsernameMentions(body, username) >= 2 {
		score += 15
		signals = append(signals, "username in body")
	}

	//apply site weight as a small confidence boost (weights in sites.json are 1-10)
	if site.Weight > 0 {
		score += site.Weight
	}

	if score > 100 {
		score = 100
	}

	found = score >= 50
	return found, score, strings.Join(signals, ", ")

}

func titleContains(body, expected string) bool {
	start := strings.Index(body, "<title>")
	end := strings.Index(body, "</title>")
	if start == -1 || end == -1 {
		return false
	}
	title := strings.ToLower(body[start+7 : end])
	return strings.Contains(title, strings.ToLower(expected))
}

func hasNotFoundContent(site loader.Site, body string) bool {
	normalizedBody := normalizeForMatch(body)

	if site.ErrorMsg != "" && strings.Contains(normalizedBody, normalizeForMatch(site.ErrorMsg)) {
		return true
	}

	commonNotFoundMarkers := []string{
		"page not found",
		"404",
		"not found",
		"couldn't find the page",
		"we couldn't find that page",
		"profile not found",
		"this page isn't available",
		"this page doesn't exist",
		"this page does not exist",
		"account doesn't exist",
	}

	for _, marker := range commonNotFoundMarkers {
		if strings.Contains(normalizedBody, normalizeForMatch(marker)) {
			return true
		}
	}

	return false
}

func countUsernameMentions(body, username string) int {
	if username == "" {
		return 0
	}
	return strings.Count(strings.ToLower(body), strings.ToLower(username))
}

func normalizeForMatch(text string) string {
	normalized := strings.ToLower(text)
	replacer := strings.NewReplacer(
		"’", "'",
		"‘", "'",
		"`", "'",
		"“", "\"",
		"”", "\"",
		"\u00a0", " ",
	)
	normalized = replacer.Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	return normalized
}

func CheckAll(client *http.Client, sites []loader.Site, username string, workers int, ratelimit time.Duration, maxRetries int) []Result {
	type job struct {
		site loader.Site
	}

	jobs := make(chan job, len(sites))
	results := make(chan Result, len(sites))

	limiter := time.NewTicker(ratelimit)
	defer limiter.Stop()

	for i := 0; i < workers; i++ {
		go func() {
			for j := range jobs {
				<-limiter.C
				results <- Check(client, j.site, username, maxRetries, 500*time.Millisecond)
			}
		}()
	}

	for _, s := range sites {
		jobs <- job{site: s}
	}
	close(jobs)

	var all []Result
	for range sites {
		all = append(all, <-results)
	}
	return all
}
