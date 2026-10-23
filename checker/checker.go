package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ashendilantha/stalkie/loader"
	"github.com/ashendilantha/stalkie/matcher"
)

type Result struct {
	Username         string          `json:"username"`
	Site             string          `json:"site"`
	URL              string          `json:"url"`
	Found            bool            `json:"found"`
	Confidence       int             `json:"confidence"`
	Evidence         string          `json:"evidence"`
	Metadata         ProfileMetadata `json:"metadata,omitempty"`
	ImageMatched     bool            `json:"image_matched,omitempty"`
	VisualSimilarity int             `json:"visual_similarity,omitempty"`
	Error            string          `json:"error,omitempty"`
	Duration         time.Duration   `json:"duration"`
}

type CircuitRenewer interface {
	RenewCircuit() error
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
}

var uaCounter uint64

func nextUserAgent() string {
	idx := atomic.AddUint64(&uaCounter, 1)
	return userAgents[idx%uint64(len(userAgents))]
}

func Check(client *http.Client, site loader.Site, username string, maxRetries int, retryDelay time.Duration) Result {
	return CheckWithRenewerAndAvatar(client, site, username, maxRetries, retryDelay, nil, matcher.ImageFingerprint{}, 0)
}

func CheckWithRenewer(client *http.Client, site loader.Site, username string, maxRetries int, retryDelay time.Duration, renewer CircuitRenewer) Result {
	return CheckWithRenewerAndAvatar(client, site, username, maxRetries, retryDelay, renewer, matcher.ImageFingerprint{}, 0)
}

func CheckWithRenewerAndAvatar(client *http.Client, site loader.Site, username string, maxRetries int, retryDelay time.Duration, renewer CircuitRenewer, refAvatar matcher.ImageFingerprint, minSim int) Result {
	targetURL := loader.BuildURL(site.URL, username)
	result := Result{
		Username: username,
		Site:     site.Name,
		URL:      targetURL,
	}

	start := time.Now()
	var resp *http.Response
	var err error
	var cancel context.CancelFunc

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		var ctx context.Context
		timeout := client.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if reqErr != nil {
			cancel()
			result.Error = reqErr.Error()
			result.Duration = time.Since(start)
			return result
		}

		req.Header.Set("User-Agent", nextUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Sec-Ch-Ua", `"Chromium";v="126", "Not/A)Brand";v="8"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		resp, err = client.Do(req)
		if err == nil {
			break
		}
		cancel()
	}

	result.Duration = time.Since(start)

	if err != nil {
		if cancel != nil {
			cancel()
		}
		result.Error = err.Error()
		return result
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if cancel != nil {
			cancel()
		}
	}()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if readErr != nil && len(bodyBytes) == 0 {
		result.Error = readErr.Error()
		return result
	}
	body := string(bodyBytes)

	if isBlockedOrChallenge(resp.StatusCode, body) {
		if renewer != nil {
			_ = renewer.RenewCircuit()
		}
		result.Error = fmt.Sprintf("blocked by anti-bot protection (HTTP %d)", resp.StatusCode)
		return result
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if renewer != nil {
			_ = renewer.RenewCircuit()
		}
		result.Error = fmt.Sprintf("rate-limited by server (HTTP %d)", resp.StatusCode)
		return result
	}

	finalURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	if isRedirectToLoginOrHome(targetURL, finalURL, body) {
		result.Error = "redirected away from profile (authwall or missing account)"
		return result
	}

	result.Found, result.Confidence, result.Evidence = evaluateResponse(site, username, resp.StatusCode, body, finalURL)
	if result.Found {
		result.Metadata = ExtractProfileMetadata(body, finalURL)
		if !refAvatar.IsZero() && result.Metadata.AvatarURL != "" {
			sim, matched := refAvatar.FetchAndCompare(client, result.Metadata.AvatarURL, 5*time.Second, minSim)
			if sim > 0 {
				result.VisualSimilarity = sim
			}
			if matched {
				result.ImageMatched = true
				result.Confidence = 100
				result.Evidence += fmt.Sprintf(", visual avatar match: %d%%", sim)
			}
		}
	}
	return result
}

func evaluateResponse(site loader.Site, username string, statusCode int, body string, finalURL string) (bool, int, string) {
	if statusCode == 404 || statusCode == 410 {
		return false, 0, fmt.Sprintf("HTTP %d", statusCode)
	}

	if site.ErrorType == "status_code" {
		expectedErrCode := site.ErrorCode
		if expectedErrCode == 0 {
			expectedErrCode = 404
		}
		if statusCode == expectedErrCode {
			return false, 0, fmt.Sprintf("HTTP %d error code", statusCode)
		}
		if statusCode != 200 {
			return false, 0, fmt.Sprintf("non-200 status code: %d", statusCode)
		}
	} else if site.ErrorType == "message" {
		if statusCode != 200 {
			return false, 0, fmt.Sprintf("non-200 status code: %d", statusCode)
		}
	}

	normalizedBody := normalizeText(body)
	if site.ErrorMsg != "" && strings.Contains(normalizedBody, normalizeText(site.ErrorMsg)) {
		return false, 0, "profile not found (error message detected)"
	}

	cleanText := extractVisibleText(body)

	if hasUniversalNotFoundMarkers(cleanText) || hasUniversalNotFoundMarkers(normalizedBody) {
		return false, 0, "not found markers in body"
	}

	var signals []string
	score := 0

	if statusCode == 200 {
		score += 30
		signals = append(signals, "HTTP 200")
	}

	pageTitle := extractTagContent(body, "title")
	ogTitle := extractMetaProperty(body, "og:title")
	hasPositiveIdentifier := false

	if site.TitleContains != "" {
		expectedTitle := strings.ReplaceAll(site.TitleContains, "{}", username)
		expectedNormalized := normalizeText(expectedTitle)
		if strings.Contains(normalizeText(pageTitle), expectedNormalized) || strings.Contains(normalizeText(ogTitle), expectedNormalized) {
			score += 35
			hasPositiveIdentifier = true
			signals = append(signals, "title match")
		}
	} else {
		uLower := strings.ToLower(username)
		if strings.Contains(strings.ToLower(pageTitle), uLower) || strings.Contains(strings.ToLower(ogTitle), uLower) {
			score += 30
			hasPositiveIdentifier = true
			signals = append(signals, "username in title")
		}
	}

	if containsWord(cleanText, username) {
		score += 25
		hasPositiveIdentifier = true
		signals = append(signals, "username in body text")
	}

	if !hasPositiveIdentifier {
		return false, score, strings.Join(signals, ", ")
	}

	if site.Weight > 0 {
		score += site.Weight
	}

	if score > 100 {
		score = 100
	}

	found := score >= 50
	return found, score, strings.Join(signals, ", ")
}

func isBlockedOrChallenge(statusCode int, body string) bool {
	if statusCode == 403 || statusCode == 429 || statusCode == 503 || statusCode == 999 {
		return true
	}
	lower := strings.ToLower(body)
	blockMarkers := []string{
		"just a moment...",
		"attention required! | cloudflare",
		"cloudflare ray id:",
		"cf-error-details",
		"/cdn-cgi/styles/cf.errors.css",
		"please enable cookies",
		"access denied",
		"please verify you are a human",
		"you've been blocked by network security",
		"captcha-delivery",
		"interstitial-challenge",
	}
	for _, marker := range blockMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isRedirectToLoginOrHome(targetURL, finalURL, body string) bool {
	finalLower := strings.ToLower(finalURL)
	targetLower := strings.ToLower(targetURL)

	if finalLower != targetLower {
		redirectIndicators := []string{
			"/login",
			"/signin",
			"/accounts/login",
			"/authwall",
			"login.facebook.com",
			"facebook.com/login",
		}
		for _, ind := range redirectIndicators {
			if strings.Contains(finalLower, ind) {
				return true
			}
		}
	}

	bodyLower := strings.ToLower(body)
	loginMarkers := []string{
		"log in to instagram",
		"log into instagram",
		"log in to facebook",
		"log into facebook",
		"sign in to tiktok",
		"join tiktok",
		"sign in to linkedin",
		"join linkedin",
	}
	for _, marker := range loginMarkers {
		if strings.Contains(bodyLower, marker) {
			return true
		}
	}
	return false
}

func hasUniversalNotFoundMarkers(cleanText string) bool {
	normalized := normalizeText(cleanText)
	markers := []string{
		"page not found",
		"profile not found",
		"user not found",
		"account not found",
		"this account doesn't exist",
		"this account does not exist",
		"this page doesn't exist",
		"this page does not exist",
		"couldn't find this account",
		"couldn't find that page",
		"sorry, this page isn't available",
		"nobody on reddit goes by that name",
		"no such user",
		"content is unavailable",
		"an error occured",
		"an error occurred",
		"invalid username",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

var (
	scriptRegex  = regexp.MustCompile(`(?si)<(script|style|head|noscript)[^>]*>.*?</(script|style|head|noscript)>`)
	tagRegex     = regexp.MustCompile(`(?s)<[^>]+>`)
	metaOgRegex  = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["']og:title["'][^>]*content=["']([^"']+)["'][^>]*>`)
	metaAltRegex = regexp.MustCompile(`(?i)<meta\s+[^>]*content=["']([^"']+)["'][^>]*property=["']og:title["'][^>]*>`)
)

func extractVisibleText(html string) string {
	noScripts := scriptRegex.ReplaceAllString(html, " ")
	plain := tagRegex.ReplaceAllString(noScripts, " ")
	return strings.Join(strings.Fields(plain), " ")
}

func extractTagContent(html, tagName string) string {
	startTag := "<" + tagName
	start := strings.Index(strings.ToLower(html), startTag)
	if start == -1 {
		return ""
	}
	tagClose := strings.Index(html[start:], ">")
	if tagClose == -1 {
		return ""
	}
	startContent := start + tagClose + 1
	endTag := "</" + tagName + ">"
	end := strings.Index(strings.ToLower(html[startContent:]), endTag)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(html[startContent : startContent+end])
}

func extractMetaProperty(html, property string) string {
	if matches := metaOgRegex.FindStringSubmatch(html); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	if matches := metaAltRegex.FindStringSubmatch(html); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	tLower := strings.ToLower(text)
	wLower := strings.ToLower(word)
	idx := strings.Index(tLower, wLower)
	if idx == -1 {
		return false
	}
	beforeOk := idx == 0 || !isAlphaNumeric(tLower[idx-1])
	afterOk := idx+len(wLower) == len(tLower) || !isAlphaNumeric(tLower[idx+len(wLower)])
	return beforeOk && afterOk
}

func isAlphaNumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z')
}

func normalizeText(text string) string {
	normalized := strings.ToLower(text)
	replacer := strings.NewReplacer(
		"\u2019", "'",
		"\u2018", "'",
		"`", "'",
		"\u201c", `"`,
		"\u201d", `"`,
		"\u00a0", " ",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	)
	normalized = replacer.Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
}

func CheckAll(client *http.Client, sites []loader.Site, username string, workers int, ratelimit time.Duration, maxRetries int, onResult func(Result)) []Result {
	return CheckAllWithRenewer(client, sites, username, workers, ratelimit, maxRetries, nil, onResult)
}

func CheckAllWithRenewer(client *http.Client, sites []loader.Site, username string, workers int, ratelimit time.Duration, maxRetries int, renewer CircuitRenewer, onResult func(Result)) []Result {
	return CheckAllWithOptions(client, sites, username, workers, ratelimit, maxRetries, renewer, matcher.ImageFingerprint{}, 0, onResult)
}

func CheckAllWithOptions(client *http.Client, sites []loader.Site, username string, workers int, ratelimit time.Duration, maxRetries int, renewer CircuitRenewer, refAvatar matcher.ImageFingerprint, minSim int, onResult func(Result)) []Result {
	if workers <= 0 {
		workers = 10
	}

	siteCount := len(sites)
	results := make([]Result, siteCount)
	jobs := make(chan struct {
		index int
		site  loader.Site
	}, siteCount)

	var wg sync.WaitGroup
	var ticker *time.Ticker
	var limiterChan <-chan time.Time

	if ratelimit > 0 {
		ticker = time.NewTicker(ratelimit)
		defer ticker.Stop()
		limiterChan = ticker.C
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if limiterChan != nil {
					<-limiterChan
				}
				res := CheckWithRenewerAndAvatar(client, job.site, username, maxRetries, 200*time.Millisecond, renewer, refAvatar, minSim)
				results[job.index] = res
				if onResult != nil {
					onResult(res)
				}
			}
		}()
	}

	for i, s := range sites {
		jobs <- struct {
			index int
			site  loader.Site
		}{index: i, site: s}
	}
	close(jobs)

	wg.Wait()
	return results
}
