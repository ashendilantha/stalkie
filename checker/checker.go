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
			break
		}
	}

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	body = string(bodyBytes)

	
	if isLoginWall(body) {
		result.Error = "login wall detected — site blocked the request (try -browser or wait before retrying)"
		return result
	}

	result.Found, result.Confidence, result.Evidence = score(site, username, resp, body)
	return result
}

func score(site loader.Site, username string, resp *http.Response, body string) (found bool, confidence int, evidence string) {
	var signals []string
	score := 0

	
	cleanBody := stripScripts(body)

	if hasNotFoundContent(site, cleanBody) {
		return false, 0, "profile not found (not-found markers detected)"
	}

	switch site.ErrorType {
		case "status_code":
			if resp.StatusCode == 200 {
				score += 40
				signals = append(signals, "HTTP 200")
			} else if resp.StatusCode == site.ErrorCode {
				return false, 0, "profile not found (status code)"
			}
		case "message":
			if strings.Contains(normalizeForMatch(cleanBody), normalizeForMatch(site.ErrorMsg)) {
				return false, 0, "profile not found (error message detected)"
			}
			score += 40
			signals = append(signals, "no error msg")
	}

	if site.TitleContains != "" {
		expected := strings.ReplaceAll(site.TitleContains, "{}", username)
		if titleContains(body, expected) {
			score += 35
			signals = append(signals, "title match")
		}
	}

	
	if countUsernameMentions(body, username) >= 2 {
		score += 15
		signals = append(signals, "username in body")
	}

	if site.Weight > 0 {
		score += site.Weight
	}

	if score > 100 {
		score = 100
	}

	found = score >= 50
	return found, score, strings.Join(signals, ", ")
}


func stripScripts(body string) string {
	result := body
	lower := strings.ToLower(result)
	for {
		start := strings.Index(lower, "<script")
		if start == -1 {
			break
		}
		end := strings.Index(lower[start:], "</script>")
		if end == -1 {
			break
		}
		end = start + end + 9 // len("</script>") == 9
		result = result[:start] + result[end:]
		lower = strings.ToLower(result)
	}
	return result
}


func hasNotFoundContent(site loader.Site, cleanBody string) bool {
	normalizedBody := normalizeForMatch(cleanBody)

	// Site-specific error message is the most reliable signal
	if site.ErrorMsg != "" {
		if strings.Contains(normalizedBody, normalizeForMatch(site.ErrorMsg)) {
			return true
		}

		return false
	}


	conservativeMarkers := []string{
		"page not found",
		"profile not found",
		"this page doesn't exist",
		"this page does not exist",
		"account doesn't exist",
		"couldn't find the page",
		"we couldn't find that page",
	}

	for _, marker := range conservativeMarkers {
		if strings.Contains(normalizedBody, normalizeForMatch(marker)) {
			return true
		}
	}

	return false
}


func isLoginWall(body string) bool {
	lower := strings.ToLower(body)
	loginMarkers := []string{
		"log in to instagram",
		"log into instagram",
		"log in to facebook",
		"log into facebook",
		"sign in to tiktok",
		"join tiktok",
		"join linkedin",
		"sign in to linkedin",
		"authwall",
		"you must be logged in",
		"please log in to continue",
	}
	for _, marker := range loginMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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

func countUsernameMentions(body, username string) int {
	if username == "" {
		return 0
	}
	return strings.Count(strings.ToLower(body), strings.ToLower(username))
}

func normalizeForMatch(text string) string {
	normalized := strings.ToLower(text)
	replacer := strings.NewReplacer(
		"\u2019", "'",
		"\u2018", "'",
		"`",      "'",
		"\u201c", "\"",
		"\u201d", "\"",
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
