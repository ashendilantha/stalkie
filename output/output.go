package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ashendilantha/stalkie/checker"
)

const (
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

func PrintBanner() {
	fmt.Print(bold + cyan + `
  ██████ ████████  █████  ██      ██   ██ ██ ███████ 
 ██         ██    ██   ██ ██      ██  ██  ██ ██      
  █████     ██    ███████ ██      █████   ██ █████   
      ██    ██    ██   ██ ██      ██  ██  ██ ██      
 ██████     ██    ██   ██ ███████ ██   ██ ██ ███████ 
` + reset)
	fmt.Println(bold + cyan + "\nStalkie - OSINT Username Enumeration Engine" + reset)
	fmt.Println()
}

func confidenceBar(score int) string {
	filled := score / 10
	if filled > 10 {
		filled = 10
	}
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 10; i++ {
		if i < filled {
			b.WriteString("█")
		} else {
			b.WriteString("░")
		}
	}
	b.WriteString(fmt.Sprintf("] %d%%", score))
	return b.String()
}

func PrintResult(r checker.Result) {
	if !r.Found {
		return
	}
	fmt.Printf("  %s[+]%s %-20s %s\n", green, reset, r.Site, r.URL)
	fmt.Printf("      %sconfidence:%s %s  %s(%s)%s\n",
		cyan, reset, confidenceBar(r.Confidence),
		yellow, r.Evidence, reset)
}

func PrintSummary(results []checker.Result, username string, elapsed time.Duration) {
	found := 0
	blocked := 0
	for _, r := range results {
		if r.Found {
			found++
		}
		if r.Error != "" {
			blocked++
		}
	}
	fmt.Printf("\n%s[*]%s Username  : %s%s%s\n", cyan, reset, bold, username, reset)
	fmt.Printf("%s[*]%s Found     : %s%d%s / %d sites\n", cyan, reset, green, found, reset, len(results))
	if blocked > 0 {
		fmt.Printf("%s[*]%s Blocked   : %s%d%s (anti-bot / rate-limit / network)\n", cyan, reset, yellow, blocked, reset)
	}
	fmt.Printf("%s[*]%s Duration  : %s\n\n", cyan, reset, elapsed.Round(time.Millisecond))
}

func SaveTXT(results []checker.Result, username, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "Stalkie results for: %s\n\n", username)
	for _, r := range results {
		if r.Found {
			fmt.Fprintf(f, "[+] %-20s %s (confidence: %d%%)\n", r.Site, r.URL, r.Confidence)
		}
	}
	return nil
}

func SaveCSV(results []checker.Result, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"username", "site", "url", "found", "confidence", "evidence", "error", "duration_ms"}); err != nil {
		return err
	}
	for _, r := range results {
		if err := w.Write([]string{
			r.Username, r.Site, r.URL,
			strconv.FormatBool(r.Found),
			strconv.Itoa(r.Confidence),
			r.Evidence, r.Error,
			strconv.FormatInt(r.Duration.Milliseconds(), 10),
		}); err != nil {
			return err
		}
	}
	return nil
}

func SaveJSON(results []checker.Result, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
