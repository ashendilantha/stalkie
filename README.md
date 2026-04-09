# Stalkie — Social Media OSINT Tool

> Username intelligence across 13+ platforms with confidence scoring, proxy support, and headless browser rendering.

```
  ██████ ████████  █████  ██      ██   ██ ██ ███████
 ██         ██    ██   ██ ██      ██  ██  ██ ██
  █████     ██    ███████ ██      █████   ██ █████
      ██    ██    ██   ██ ██      ██  ██  ██ ██
 ██████     ██    ██   ██ ███████ ██   ██ ██ ███████
```

---

## Features

- **Confidence scoring** — every hit is scored 0–100 based on multiple signals (HTTP status, title match, body mentions), not just found/not-found
- **Script-tag stripping** — strips `<script>` blocks before analysis, eliminating false negatives caused by JS bundles on platforms like YouTube and TikTok
- **Login wall detection** — detects when sites (Instagram, LinkedIn) serve a login redirect instead of a real profile and flags it instead of reporting a false positive
- **Headless browser mode** — renders JS-heavy pages with real Chromium via `go-rod` for sites that don't work with plain HTTP
- **Proxy & Tor support** — route traffic through HTTP proxy, SOCKS5, or Tor (defaults to `127.0.0.1:9050`)
- **Bulk input** — search multiple usernames from a file in one run
- **Multi-format export** — save results as TXT, CSV, or JSON with full metadata
- **Concurrent workers** — configurable worker pool with rate limiting for speed vs. stealth tradeoffs

---

## Installation

```bash
git clone https://github.com/ashendilantha/stalkie
cd stalkie
go build -o stalkie .
```

Requires Go 1.21+. Browser mode auto-downloads Chromium on first use.

---

## Usage

```bash
# Single username
./stalkie -u torvalds

# Bulk usernames from file
./stalkie -f usernames.txt

# Headless browser mode (for JS-heavy sites)
./stalkie -u torvalds -browser

# Route through Tor
./stalkie -u torvalds -proxy tor

# Export results
./stalkie -u torvalds -csv results.csv -json results.json
```

### All Flags

| Flag | Default | Description |
|---|---|---|
| `-u` | — | Single username to search |
| `-f` | — | Text file with one username per line |
| `-sites` | `sites.json` | Sites database file |
| `-w` | `20` | Concurrent workers |
| `-rate` | `50` | Delay between requests (ms) |
| `-retries` | `2` | Max retries on network failure |
| `-timeout` | `10` | Request timeout (seconds) |
| `-proxy` | `none` | Proxy type: `none`, `http`, `socks5`, `tor` |
| `-proxy-addr` | — | Proxy address (e.g. `127.0.0.1:9050`) |
| `-min-score` | `50` | Minimum confidence score to report as found |
| `-browser` | `false` | Use headless browser for JS-heavy sites |
| `-o` | — | Save results as `.txt` |
| `-csv` | — | Save results as `.csv` |
| `-json` | — | Save results as `.json` |

---

## Confidence Scoring

Each result is scored based on four signals:

| Signal | Points |
|---|---|
| HTTP 200 / no error message detected | +40 |
| Page `<title>` contains the username | +35 |
| Username appears ≥ 2 times in page body | +15 |
| Site-specific weight bonus | up to +10 |

Results below `--min-score` (default 50) are suppressed. Raise it to reduce false positives; lower it to catch weak matches.

---

## Adding Sites

Edit `sites.json` to add platforms without touching source code:

```json
{
  "name": "Example",
  "url": "https://example.com/{}",
  "errorType": "message",
  "errorMsg": "User not found",
  "titleContains": "{} - Example",
  "weight": 8
}
```

`errorType` is either `"status_code"` (checks HTTP response code) or `"message"` (looks for an error string in the page body).

---

## How It Differs from Sherlock

| Feature | Sherlock | Stalkie |
|---|---|---|
| Language | Python | Go (native binary, no runtime needed) |
| Result type | Found / Not Found | Confidence score with evidence |
| Script-tag stripping | ❌ | ✅ eliminates JS false negatives |
| Login wall detection | ❌ | ✅ |
| Headless browser mode | ❌ | ✅ via go-rod |
| Tor / SOCKS5 support | partial | ✅ native |
| Export formats | TXT | TXT, CSV, JSON |
| Sites database | Hardcoded Python | External `sites.json` |

---

## Disclaimer

Stalkie is intended for lawful OSINT investigations and educational use only. Always ensure you have appropriate authorization before investigating any individual. The author assumes no liability for misuse.

---

<p align="center">
  <img src="https://komarev.com/ghpvc/?username=ashendilantha&label=Profile%20views&color=0e75b6&style=flat" alt="profile views" />
</p>

**Built by [Ashen Dilantha](https://github.com/ashendilantha)**
