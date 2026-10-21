package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ashendilantha/stalkie/checker"
	"github.com/ashendilantha/stalkie/loader"
	"github.com/ashendilantha/stalkie/output"
	"github.com/ashendilantha/stalkie/proxy"
	"github.com/ashendilantha/stalkie/syncer"
	"github.com/schollz/progressbar/v3"
)

func main() {
	username := flag.String("u", "", "Single username to search")
	bulkFile := flag.String("f", "", "Text file with one username per line")
	sitesFile := flag.String("sites", "sites.json", "Sites database file")
	category := flag.String("category", "", "Filter platforms by comma-separated categories (e.g. coding,social,design,gaming,finance)")
	permute := flag.Bool("permute", false, "Check common username permutations and mutations")
	updateDB := flag.Bool("update-db", false, "Pull and normalize upstream database from WhatsMyName")
	workers := flag.Int("w", 20, "Concurrent workers")
	rateMs := flag.Int("rate", 50, "Delay between requests in ms")
	retries := flag.Int("retries", 2, "Max retries on network failure")
	timeout := flag.Int("timeout", 10, "HTTP request timeout in seconds")
	proxyType := flag.String("proxy", "none", "Proxy type: none, http, socks5, tor")
	proxyAddr := flag.String("proxy-addr", "", "Proxy address")
	torControl := flag.String("tor-control", "127.0.0.1:9051", "Tor control port address for circuit renewal")
	torPassword := flag.String("tor-password", "", "Tor control port password")
	outTXT := flag.String("o", "", "Save results as .txt")
	outCSV := flag.String("csv", "", "Save results as .csv")
	outJSON := flag.String("json", "", "Save results as .json")
	outHTML := flag.String("html", "", "Save interactive relationship graph as .html")
	minScore := flag.Int("min-score", 50, "Minimum confidence score to report as found")
	useBrowser := flag.Bool("browser", false, "Use headless browser mode for JS-heavy sites")
	noUTLS := flag.Bool("no-utls", false, "Disable uTLS fingerprint spoofing")
	flag.Parse()

	if *updateDB {
		output.PrintBanner()
		fmt.Printf("[*] Syncing database with WhatsMyName upstream...\n")
		syncTimeout := time.Duration(*timeout) * time.Second
		if syncTimeout < 30*time.Second {
			syncTimeout = 30 * time.Second
		}
		total, err := syncer.Sync(*sitesFile, syncTimeout)
		if err != nil {
			fmt.Printf("[!] Database sync failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[*] Successfully synced and normalized %d sites into %s\n", total, *sitesFile)
		return
	}

	output.PrintBanner()

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
		fmt.Println("Usage: stalkie -u <username> OR -f <usernames.txt> OR -update-db")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *permute {
		var expanded []string
		for _, u := range usernames {
			expanded = append(expanded, loader.GeneratePermutations(u)...)
		}
		usernames = loader.DedupeUsernames(expanded)
		fmt.Printf("[*] Permutations enabled: scanning %d username variations\n", len(usernames))
	}

	sites, err := loader.Load(*sitesFile)
	if err != nil {
		fmt.Printf("Error loading sites: %v\n", err)
		os.Exit(1)
	}

	if *category != "" {
		initialCount := len(sites)
		sites = loader.FilterByCategory(sites, *category)
		if len(sites) == 0 {
			fmt.Printf("[!] No sites matched category filter: %s (available: coding, social, design, gaming, finance)\n", *category)
			os.Exit(1)
		}
		fmt.Printf("[*] Category filter '%s': %d of %d platforms selected\n", *category, len(sites), initialCount)
	}

	httpClient, err := proxy.BuildClient(proxy.Config{
		Type:        *proxyType,
		Address:     *proxyAddr,
		DisableUTLS: *noUTLS,
		TorControl:  *torControl,
		TorPassword: *torPassword,
	}, time.Duration(*timeout)*time.Second)
	if err != nil {
		fmt.Printf("Proxy error: %v\n", err)
		os.Exit(1)
	}

	var torRenewer checker.CircuitRenewer
	if strings.ToLower(strings.TrimSpace(*proxyType)) == "tor" {
		torRenewer = proxy.NewTorRenewer(*torControl, *torPassword)
	}

	var rateLimit time.Duration
	if *rateMs > 0 {
		rateLimit = time.Duration(*rateMs) * time.Millisecond
	}

	var allResults []checker.Result

	for _, uname := range usernames {
		fmt.Printf("\n[*] Searching: %s%s%s across %d sites\n\n",
			"\033[1m", uname, "\033[0m", len(sites))

		bar := progressbar.NewOptions(len(sites),
			progressbar.OptionSetDescription("  scanning"),
			progressbar.OptionSetWidth(30),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
		)

		var barMu sync.Mutex
		onResult := func(r checker.Result) {
			barMu.Lock()
			bar.Add(1)
			barMu.Unlock()
		}

		start := time.Now()

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
				onResult,
			)
		} else {
			results = checker.CheckAllWithRenewer(httpClient, sites, uname, *workers, rateLimit, *retries, torRenewer, onResult)
		}

		bar.Finish()

		for _, r := range results {
			if r.Confidence < *minScore && r.Found {
				r.Found = false
			}
			output.PrintResult(r)
			allResults = append(allResults, r)
		}

		output.PrintSummary(results, uname, time.Since(start))
	}

	if *outTXT != "" {
		for _, uname := range usernames {
			var sub []checker.Result
			for _, r := range allResults {
				if r.Username == uname {
					sub = append(sub, r)
				}
			}
			if err := output.SaveTXT(sub, uname, *outTXT); err != nil {
				fmt.Printf("Error saving TXT: %v\n", err)
			}
		}
		fmt.Printf("[*] Saved TXT -> %s\n", *outTXT)
	}

	if *outCSV != "" {
		if err := output.SaveCSV(allResults, *outCSV); err != nil {
			fmt.Printf("Error saving CSV: %v\n", err)
		} else {
			fmt.Printf("[*] Saved CSV -> %s\n", *outCSV)
		}
	}

	if *outJSON != "" {
		if err := output.SaveJSON(allResults, *outJSON); err != nil {
			fmt.Printf("Error saving JSON: %v\n", err)
		} else {
			fmt.Printf("[*] Saved JSON -> %s\n", *outJSON)
		}
	}

	if *outHTML != "" {
		primaryTarget := *username
		if primaryTarget == "" && len(usernames) > 0 {
			primaryTarget = usernames[0]
		}
		if err := output.SaveHTML(allResults, primaryTarget, *outHTML); err != nil {
			fmt.Printf("Error saving HTML: %v\n", err)
		} else {
			fmt.Printf("[*] Saved HTML graph -> %s\n", *outHTML)
		}
	}
}
