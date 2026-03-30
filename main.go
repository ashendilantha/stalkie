package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ashendilantha/stalkie/checker"
	"github.com/ashendilantha/stalkie/loader"
	"github.com/ashendilantha/stalkie/output"
	"github.com/ashendilantha/stalkie/proxy"
	"github.com/schollz/progressbar/v3"
)

func main() {
	//flags
	username := flag.String("u", "", "Single username to search")
	bulkFile := flag.String("f", "", "Text file with one username per line")
	sitesFile := flag.String("sites", "sites.json", "Sites database file")
	workers := flag.Int("w", 20, "Concurrent workers")
	rateMs := flag.Int("rate", 50, "Delay between requests in ms (rate limiting)")
	retries := flag.Int("retries", 2, "Max retries on network failure")
	timeout := flag.Int("timeout", 10, "HTTP request timeout in second")
	proxyType := flag.String("proxy", "none", "Proxy type: none, http, socks5, tor")
	proxyAddr := flag.String("proxy-addr", "", "Proxy address (e.g. 127.0.0.1:9050)")
	outTXT := flag.String("o", "", "Save results as .txt")
	outCSV := flag.String("csv", "", "Save results as .csv")
	outJSON := flag.String("json", "", "Save results as .json")
	minScore := flag.Int("min-score", 50, "Minimum confidence score to report as found")
	useBrowser := flag.Bool("browser", false, "Use headless browser mode for JS-heavy sites")
	flag.Parse()

	output.PrintBanner()

	//build usernames list
	var usernames []string
	if *username != "" {
		usernames = append(usernames, *username)
	}
	if *bulkFile != "" {
		names, err := loader.LoadUsernames(*bulkFile)
		if err != nil {
			fmt.Printf("Error loading usernames file: %v\n", err)
			os.Exit(1)
		}
		usernames = append(usernames, names...)
	}

	if len(usernames) == 0 {
		fmt.Println("Usage: stalkie -u <username> OR -f <usernames.txt>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	//load sites
	sites, err := loader.Load(*sitesFile)
	if err != nil {
		fmt.Printf("Error loading sites: %v\n", err)
		os.Exit(1)
	}

	//build HTTP client with proxy or Tor
	httpClient, err := proxy.BuildClient(proxy.Config{
		Type:    *proxyType,
		Address: *proxyAddr,
	}, time.Duration(*timeout)*time.Second)
	if err != nil {
		fmt.Printf("Proxy error: %v\n", err)
		os.Exit(1)
	}

	rateLimit := time.Duration(*rateMs) * time.Millisecond
	var allResults []checker.Result

	//search each username
	for _, uname := range usernames {
		fmt.Printf("\n[*] Searching: %s%s%s across %d sites\n\n",
			"\033[1m", uname, "\033[0m", len(sites))

		bar := progressbar.NewOptions(len(sites),
			progressbar.OptionSetDescription("  scanning"),
			progressbar.OptionSetWidth(30),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
		)

		start := time.Now()

		//run concurrent search with rate limiting
		var results []checker.Result
		if *useBrowser {
			results = checker.CheckAllWithBrowser(
				sites,
				uname,
				*workers,
				rateLimit,
				*retries,
				time.Duration(*timeout)*time.Second,
				*proxyType,
				*proxyAddr,
			)
		} else {
			results = checker.CheckAll(httpClient, sites, uname, *workers, rateLimit, *retries)
		}

		bar.Finish()

		for _, r := range results {
			if r.Confidence < *minScore && r.Found {
				r.Found = false // downgrade low-confidence hits
			}
			output.PrintResult(r)
			allResults = append(allResults, r)
		}

		output.PrintSummary(results, uname, time.Since(start))
	}

	//save results
	if *outTXT != "" {
		for _, uname := range usernames {
			var sub []checker.Result
			for _, r := range allResults {
				if r.Username == uname {
					sub = append(sub, r)
				}
			}
			output.SaveTXT(sub, uname, *outTXT)
		}
		fmt.Printf("[*] Saved TXT → %s\n", *outTXT)
	}

	if *outCSV != "" {
		output.SaveCSV(allResults, *outCSV)
		fmt.Printf("[*] Saved CSV → %s\n", *outCSV)
	}

	if *outJSON != "" {
		output.SaveJSON(allResults, *outJSON)
		fmt.Printf("[*] Saved JSON → %s\n", *outJSON)
	}
}
