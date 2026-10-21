# Stalkie - OSINT Username Enumeration Engine

> Fast, concurrent username intelligence across 650+ platforms with confidence scoring, automated WhatsMyName schema synchronization, anti-bot challenge isolation, proxy routing, and headless browser rendering.

```
  ██████ ████████  █████  ██      ██   ██ ██ ███████ 
 ██         ██    ██   ██ ██      ██  ██  ██ ██      
  █████     ██    ███████ ██      █████   ██ █████   
      ██    ██    ██   ██ ██      ██  ██  ██ ██      
 ██████     ██    ██   ██ ███████ ██   ██ ██ ███████ 
```

---

## Features

- **Automated Upstream Sync** - Synchronize and normalize 650+ platform signatures directly from the WhatsMyName project via `-update-db`.
- **Target Categorization & Selective Enumeration** - Platforms tagged with categories (`coding`, `social`, `design`, `gaming`, `finance`). Filter scans with `-category coding,design` to reduce scan times and minimize footprint.
- **Structured JSON-LD & Profile Data Extraction** - Automatically extracts display names, avatar URLs, bios, follower counts, creation dates, and locations from `<script type="application/ld+json">` and OpenGraph meta tags into console, CSV, and JSON outputs.
- **Smart Username Mutations** - Generate naming variations (`_`, `-dev`, `real<user>`, `<user>01`) with `-permute` to discover alternative handles.
- **TLS Fingerprint Spoofing (uTLS)** - Emulates genuine Chrome and Firefox ClientHello TLS handshakes (JA3/JA4) with HTTP/2 multiplexing to evade bot protections without headless Chromium.
- **Tor Circuit Auto-Renewal** - Connects to the Tor control port (`9051`) and triggers `SIGNAL NEWNYM` automatically when rate limits (HTTP 429) or Cloudflare challenges are detected.
- **False-Positive Elimination** - Requires verified positive indicators (username in `<title>`, `og:title`, or visible user text) before reporting an account as found.
- **Confidence Scoring** - Multi-signal evaluation engine scoring each match from 0 to 100 with clear evidence attribution.
- **Anti-Bot & Rate-Limit Isolation** - Detects Cloudflare challenges, Datadome interstitials, HTTP 403, 429, 503, and LinkedIn 999 responses, categorizing them as blocked rather than false positives or silent misses.
- **Login Wall & Authwall Detection** - Flags redirect traps (e.g. Instagram login, Facebook sign-in) instead of misclassifying them as active profiles.
- **Headless Browser Mode** - Optional Rod-based Chromium engine for executing client-side JavaScript on single-page applications.
- **Real-Time Live Progress** - Live progress tracking with immediate worker-level feedback.
- **Proxy & Tor Routing** - Native support for HTTP, HTTPS, SOCKS5, and Tor circuits (`127.0.0.1:9050`).
- **Bulk Processing & Export** - Enumerate single handles or batch files; export results to TXT, CSV, or structured JSON.

---

## Installation

```bash
git clone https://github.com/ashendilantha/stalkie
cd stalkie
go build -o stalkie .
```

Requires Go 1.21+. Headless browser mode automatically manages Chromium instances on first run when `-browser` is supplied.

---

## Usage

### 1. Database Synchronization

Pull and normalize the latest platform database from WhatsMyName:

```bash
./stalkie -update-db
```

### 2. Search Commands

```bash
# Single username scan
./stalkie -u ashendilantha

# Scan across specific sites database
./stalkie -u ashendilantha -sites sites.json

# Bulk scan from a text file
./stalkie -f usernames.txt

# Adjust concurrency and rate limiting
./stalkie -u ashendilantha -w 40 -rate 10

# Route traffic through Tor with automatic circuit rotation
./stalkie -u ashendilantha -proxy tor -tor-control 127.0.0.1:9051

# Filter by platform category
./stalkie -u ashendilantha -category coding,design

# Test username permutations and naming variations
./stalkie -u ashendilantha -permute

# Route traffic through SOCKS5 proxy
./stalkie -u ashendilantha -proxy socks5 -proxy-addr 127.0.0.1:1080

# Run in headless browser mode for JS-rendered applications
./stalkie -u ashendilantha -browser

# Export to TXT, CSV, and JSON with extracted metadata
./stalkie -u ashendilantha -o results.txt -csv results.csv -json results.json
```

---

## Command Flags

| Flag | Default | Description |
|---|---|---|
| `-u` | - | Single username to enumerate |
| `-f` | - | Text file containing usernames (one per line) |
| `-update-db` | `false` | Pull and normalize upstream database from WhatsMyName |
| `-sites` | `sites.json` | Path to platform database JSON |
| `-category` | - | Filter platforms by comma-separated categories (`coding`, `social`, `design`, `gaming`, `finance`) |
| `-permute` | `false` | Generate and check common naming variations and permutations |
| `-w` | `20` | Concurrent worker pool size |
| `-rate` | `50` | Request interval delay in milliseconds |
| `-retries` | `2` | Maximum retry attempts per endpoint |
| `-timeout` | `10` | HTTP and browser timeout in seconds |
| `-proxy` | `none` | Proxy protocol: `none`, `http`, `socks5`, `tor` |
| `-proxy-addr` | - | Proxy server host and port |
| `-tor-control` | `127.0.0.1:9051` | Tor control port address for circuit auto-renewal |
| `-tor-password` | - | Tor control port authentication password |
| `-min-score` | `50` | Minimum confidence score threshold for reporting |
| `-browser` | `false` | Enable headless Chromium rendering |
| `-no-utls` | `false` | Disable uTLS fingerprint spoofing |
| `-o` | - | Output path for TXT export |
| `-csv` | - | Output path for CSV export |
| `-json` | - | Output path for JSON export |

---

## Verification & Scoring Model

Stalkie enforces positive corroboration before classifying a target as found. Accounts must meet or exceed the `--min-score` threshold (default 50) and contain at least one positive identifier:

| Evaluation Signal | Points | Condition |
|---|---|---|
| HTTP 200 Status | +30 | Valid HTTP response without error triggers |
| Explicit Title Match | +35 | Matched against configured `titleContains` pattern |
| Inferred Title Match | +30 | Target username located in `<title>` or `og:title` |
| Visible Text Mention | +25 | Target username present in stripped visible body text |
| Category Weight | +0 to +8 | Bonus applied for verified matches in key categories |

If negative markers (e.g. "page not found", "user not found", universal error patterns, or soft-404 redirects) are detected, confidence is reset to 0 regardless of status code.

---

## Testing

Run the automated test suite across all packages:

```bash
go test -v ./...
```

To run individual package test suites:

```bash
go test -v ./syncer
go test -v ./checker
go test -v ./loader
```

---

## Comparison with Sherlock

| Capability | Sherlock | Stalkie |
|---|---|---|
| Runtime | Python (interpreter required) | Go (single compiled static binary) |
| Output Data | Found / Not Found | Confidence score (0-100%) with evidence string |
| Database Sync | Manual / Script | Built-in via `-update-db` (WhatsMyName 650+ sites) |
| Headless Browser Rendering | No | Yes (native Rod Chromium integration) |
| Anti-Bot Challenge Handling | Limited | Yes (Cloudflare, Datadome, 403/429/999 isolated) |
| Script & Head Stripping | No | Yes (prevents metadata false positives) |
| Tor / SOCKS5 Integration | External dependencies | Native standard library & context proxying |
| Live Real-Time Progress | Basic | Live progress bar with worker streaming |
| Export Formats | TXT, CSV, XLSX | TXT, CSV, JSON |

---

## Disclaimer

Stalkie is designed for defensive security analysis, authorized open-source intelligence collection, and educational verification. Ensure you have proper authorization before conducting reconnaissance against any identifier. The authors assume no responsibility for unauthorized activity.

---

**Maintained by [Ashen Dilantha](https://github.com/ashendilantha)**
