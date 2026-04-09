package checker

import (
	"fmt"
	"strings"
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
) []Result {
	browser, err := launchBrowser(proxyType, proxyAddr)
	if err != nil {
		var failed []Result
		for _, site := range sites {
			failed = append(failed, Result{
				Username: username,
				Site:     site.Name,
				URL:      loader.BuildURL(site.URL, username),
					Found:    false,
					Error:    fmt.Sprintf("browser launch failed: %v", err),
			})
		}
		return failed
	}
	defer browser.MustClose()

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
				results <- checkWithBrowser(browser, j.site, username, maxRetries, timeout)
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

func checkWithBrowser(browser *rod.Browser, site loader.Site, username string, maxRetries int, timeout time.Duration) (result Result) {
	url := loader.BuildURL(site.URL, username)
	result = Result{
		Username: username,
		Site:     site.Name,
		URL:      url,
	}

	start := time.Now()
	var body string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		succeeded := false
		func() {
			defer func() {
				if recover() != nil {
					succeeded = false
				}
			}()

			page := browser.MustPage(url)
			defer page.MustClose()
			page.Timeout(timeout).MustWaitLoad()
			body = page.MustHTML()
			succeeded = true
		}()

		if succeeded {
			break
		}
	}

	result.Duration = time.Since(start)

	if body == "" {
		result.Error = "browser failed to render page"
		return result
	}

	
	if isLoginWall(body) {
		result.Error = "login wall detected in browser mode — site is blocking headless Chromium (try adding delays or using a real proxy)"
		return result
	}

	result.Found, result.Confidence, result.Evidence = scoreBrowser(site, username, body)
	return result
}

func scoreBrowser(site loader.Site, username string, body string) (found bool, confidence int, evidence string) {
	var signals []string

	
	score := 30
	signals = append(signals, "browser render")

	cleanBody := stripScripts(body)

	if hasNotFoundContent(site, cleanBody) {
		return false, 0, "profile not found (not-found markers detected)"
	}

	lowerClean := strings.ToLower(cleanBody)

	switch site.ErrorType {
		case "message":
			if site.ErrorMsg != "" && strings.Contains(normalizeForMatch(cleanBody), normalizeForMatch(site.ErrorMsg)) {
				return false, 0, "profile not found (error message detected)"
			}
			score += 25
			signals = append(signals, "no error msg")
		case "status_code":
			// In browser mode we don't have the raw HTTP status, so check
			// rendered content for clear 404 indicators in the visible page body
			if strings.Contains(lowerClean, "page not found") ||
				strings.Contains(lowerClean, "this page doesn't exist") ||
				strings.Contains(lowerClean, "account doesn't exist") {
					return false, 0, "profile not found (404/not found in rendered page)"
				}
				score += 25
				signals = append(signals, "no 404 marker")
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

func launchBrowser(proxyType string, proxyAddr string) (*rod.Browser, error) {
	l := launcher.New().Headless(true)

	switch proxyType {
		case "http":
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
		case "none", "":
			// direct
		default:
			return nil, fmt.Errorf("unsupported proxy type for browser mode: %s", proxyType)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, err
	}

	return browser, nil
}
