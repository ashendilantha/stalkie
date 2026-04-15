# Security Policy

## Supported Versions

Stalkie is an actively maintained open-source OSINT tool. Only the latest release is supported with security updates.

| Version | Supported |
| ------- | --------- |
| latest  | ✅        |
| older   | ❌        |

## Scope

Stalkie is an **educational** username-checking tool intended for lawful OSINT research and cybersecurity awareness. The following are considered in-scope for vulnerability reports:

- Logic flaws that produce misleading results (false positives/negatives at scale)
- Issues in the HTTP client or proxy handling that could expose the user's identity unintentionally
- Dependency vulnerabilities in Go modules
- Insecure default configurations

The following are **out of scope**:

- Misuse of the tool against third-party platforms (this is a user responsibility)
- Rate limiting or blocking by target sites (expected behavior)

## Reporting a Vulnerability

If you discover a security issue in Stalkie, please report it responsibly.

- **For critical or sensitive vulnerabilities** — use [GitHub's private vulnerability reporting](../../security/advisories/new) to avoid public disclosure before a fix is available.
- **For low-severity issues** — opening a public GitHub issue is acceptable.

Please include:
1. A clear description of the vulnerability
2. Steps to reproduce
3. Potential impact
4. Suggested fix (if any)

I aim to respond within **72 hours** and will credit reporters in the release notes (unless you prefer to stay anonymous).

## Responsible Use

Stalkie is built for **educational and research purposes only**. Users are responsible for ensuring their use complies with applicable laws and the terms of service of any platform queried. The maintainer does not condone or support misuse of this tool.

## Contact

Maintainer: **Ashen Dilantha**
GitHub: [@ashendilantha](https://github.com/ashendilantha)
