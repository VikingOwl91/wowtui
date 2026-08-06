# wowtui

[![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-pre--alpha-orange.svg)](#status)

A security-focused World of Warcraft addon manager for Linux, written in Go.
The interactive terminal UI is built with Bubble Tea. wowtui is an independent
implementation inspired by addon-manager workflows such as WowUp. It does not
copy WowUp code.

## Status

The project is in pre-alpha. The read-only scanner and the GitHub, GitLab, and
direct-URL installation paths are functional. The TUI is usable for scanning,
listing, installing, checking, and updating addons, but the interface and
provider catalog are still evolving.

### Providers

| Provider | Status | Notes |
| --- | --- | --- |
| GitHub | Available | Releases, tags, and default-branch fallback |
| GitLab | Available | Releases, tags, and branch archives |
| Generic Git | Available | HTTPS/SSH repositories via tags or shallow branch clones |
| Direct URL | Available | ZIP URLs with ETag, Last-Modified, or SHA-256 identity |
| Wago | Pending approval | Kept as a planned integration; no stable public read/download API was verified |
| CurseForge | Pending approval | Adapter ready; activation requires approved third-party API access |
| WowUpHub | Pending upstream approval | Adapter implemented, activation paused until WowUp confirms third-party use |

## Features

- Discover WoW installations in Wine and Lutris layouts
- Scan Retail, Classic, Classic Era, Beta, and PTR installations
- Parse addon `.toc` metadata and source hints
- Install addons from GitHub releases, tags, branches, GitLab releases, or ZIP URLs
- Check and update individual or all managed addons
- Backup existing addon folders before replacement
- Transactional installation with rollback on failure
- Safe ZIP extraction with Zip Slip, symlink, file-count, and size checks
- SHA-256 archive hashes
- Provider enable/disable state and configurable priority order
- Installation-specific display titles and multi-install support
- Non-blocking TUI operations with structured progress events

## Quick Start

Build the binary:

```bash
go build -o wowtui .
```

Scan installations and addons:

```bash
./wowtui scan
./wowtui list
./wowtui installs
./wowtui map
```

Start the TUI:

```bash
./wowtui
```

Install an addon from GitHub:

```bash
./wowtui add github:VikingOwl91/StreamerPrivacy
```

Pin a source explicitly:

```bash
./wowtui add github:Owner/Repo --source tag
```

Install from a direct ZIP URL:

```bash
./wowtui add direct:https://example.com/addon.zip
```

Check and update managed addons:

```bash
./wowtui check
./wowtui update AddonName
./wowtui update --all
./wowtui update --all --dry-run
```

Map an unmanaged installed addon to an active provider without replacing its
files:

```bash
./wowtui map
./wowtui adopt "/path/to/_retail_" WeakAuras github:Owner/WeakAuras
```

## Configuration

The default configuration path is `~/.config/wowtui/config.toml`. XDG paths
are supported.

```toml
[wow]
paths = [
  "/home/user/Games/battlenet-new/drive_c/Program Files (x86)/World of Warcraft",
]

[providers]
order = ["wowuphub", "curseforge", "github", "gitlab", "direct"]

[providers.github]
enabled = true
token = ""

[providers.gitlab]
enabled = true
api_key = ""

[providers.curseforge]
enabled = false
api_key = ""
```

Provider settings can also be managed from the CLI:

```bash
./wowtui providers
./wowtui providers enable gitlab
./wowtui providers disable curseforge
./wowtui providers order github gitlab direct
```

Provider credentials can be supplied through environment variables where
supported. They are never stored in the addon registry.

## Security Model

Downloaded archives are never extracted directly into the WoW directory.
The installer validates and stages an archive first, then creates a backup and
performs a transactional replacement. Unsafe paths, symlinks, unsupported file
types, excessive file counts, and oversized files are rejected.

Bubblewrap sandboxing is planned as an optional isolation layer around the
archive worker on Linux. The internal extractor remains the mandatory baseline
so the application also works without Bubblewrap.

## Development

The project uses test-driven development. Run the local gates with:

```bash
gofmt -l .
go vet ./...
go test ./...
govulncheck ./...
```

The code is organized so the service and provider layers can be tested without
a terminal. Provider HTTP tests use local test servers rather than live APIs.

## License

MIT. See [LICENSE](LICENSE).
