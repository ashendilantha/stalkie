package checker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ashendilantha/stalkie/loader"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func CheckAllWithBrowser(
	sites []loader.Site,
	username string,
	workers int,
	ratelimit time.Duration,
	maxRetries int,
	timeout time.Duration,
	proxyType string,
	proxyAddr string,
	onResult func(Result),
) []Result {
	if workers <= 0 {
		workers = 5
	}

	browser, err := launchBrowser(proxyType, proxyAddr)
	if err != nil {
		failed := make([]Result, len(sites))
		for i, site := range sites {
			failed[i] = Result{
				Username: username,
				Site:     site.Name,
				URL:      loader.BuildURL(site.URL, username),
				Found:    false,
				Error:    fmt.Sprintf("browser initialization failed: %v", err),
			}
			if onResult != nil {
				onResult(failed[i])
			}
		}
		return failed
	}
	defer browser.MustClose()

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
				res := checkWithBrowser(browser, job.site, username, maxRetries, timeout)
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

func checkWithBrowser(browser *rod.Browser, site loader.Site, username string, maxRetries int, timeout time.Duration) Result {
	targetURL := loader.BuildURL(site.URL, username)
	result := Result{
		Username: username,
		Site:     site.Name,
		URL:      targetURL,
	}

	start := time.Now()
	var body string
	var finalURL string
	var succeeded bool

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					succeeded = false
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			page := browser.Context(ctx).MustPage(targetURL)
			defer page.MustClose()

			page.MustWaitLoad()
			time.Sleep(500 * time.Millisecond)

			info := page.MustInfo()
			finalURL = info.URL
			body = page.MustHTML()
			succeeded = true
		}()

		if succeeded && body != "" {
			break
		}
	}

	result.Duration = time.Since(start)

	if !succeeded || body == "" {
		result.Error = "browser failed to render target page"
		return result
	}

	if isBlockedOrChallenge(200, body) {
		result.Error = "browser detected anti-bot challenge or block"
		return result
	}

	if isRedirectToLoginOrHome(targetURL, finalURL, body) {
		result.Error = "redirected to login or authwall in browser mode"
		return result
	}

	result.Found, result.Confidence, result.Evidence = evaluateBrowserRender(site, username, body)
	return result
}

func evaluateBrowserRender(site loader.Site, username string, body string) (bool, int, string) {
	cleanText := extractVisibleText(body)

	if hasUniversalNotFoundMarkers(cleanText) {
		return false, 0, "not found markers in rendered body"
	}

	if site.ErrorType == "message" && site.ErrorMsg != "" {
		if strings.Contains(normalizeText(cleanText), normalizeText(site.ErrorMsg)) {
			return false, 0, "profile not found (error message detected)"
		}
	}

	var signals []string
	score := 25
	signals = append(signals, "browser rendered")

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
			score += 25
			hasPositiveIdentifier = true
			signals = append(signals, "username in title")
		}
	}

	if containsWord(cleanText, username) {
		score += 20
		hasPositiveIdentifier = true
		signals = append(signals, "username in body text")
	}

	if !hasPositiveIdentifier {
		return false, score, strings.Join(signals, ", ")
	}

	if site.Weight > 0 && score >= 40 {
		score += site.Weight
	}

	if score > 100 {
		score = 100
	}

	found := score >= 50
	return found, score, strings.Join(signals, ", ")
}

func launchBrowser(proxyType string, proxyAddr string) (*rod.Browser, error) {
	l := launcher.New().
		Headless(true).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage")

	normProxy := strings.ToLower(strings.TrimSpace(proxyType))
	switch normProxy {
	case "http", "https":
		if proxyAddr != "" {
			l = l.Proxy(proxyAddr)
		}
	case "tor", "socks", "socks5":
		addr := proxyAddr
		if addr == "" {
			addr = "127.0.0.1:9050"
		}
		if !strings.HasPrefix(addr, "socks5://") {
			addr = "socks5://" + addr
		}
		l = l.Proxy(addr)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, err
	}

	return rod.New().ControlURL(controlURL).MustConnect(), nil
}
