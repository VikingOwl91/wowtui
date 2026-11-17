# wowtui

[![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-pre--alpha-orange.svg)](#status)

A security-focused World of Warcraft addon manager for Linux, written in Go.
The interactive terminal UI is built with Bubble Tea. wowtui is an independent
implementation inspired by addon-manager workflows such as WowUp. It does not
copy WowUp code.

## Status

The project is in pre-alpha. The read-only scanner and the GitHub, GitLab,
generic Git, and direct-URL installation paths are functional. The TUI is
usable for scanning, listing, installing, checking, and updating addons, but
the interface and provider catalog are still evolving.

### Providers

| Provider | Status | Notes |
| --- | --- | --- |
| GitHub | Available | Releases, tags, and default-branch fallback |
| GitLab | Available | Releases, tags, and branch archives |
| Generic Git | Available | HTTPS/SSH repositories via tags or shallow branch clones |
| Direct URL | Available | ZIP URLs with ETag, Last-Modified, or SHA-256 identity |
| Wago | Pending approval | Kept as a planned integration; no stable public read/download API was verified |
| CurseForge | Key-gated | Enabled only when a local API key is configured; it is never bundled with wowtui |
| WowUpHub | Pending upstream approval | Adapter implemented, activation paused until WowUp confirms third-party use |

## Features

- Discover WoW installations in Wine and Lutris layouts
- Scan all twelve current client directories, each as its own flavor (see
  [Flavors](#flavors))
- Parse addon `.toc` metadata and source hints
- Install addons from GitHub releases, tags, branches, GitLab releases, or ZIP URLs
- Pick release assets by the game type of the target client, from the BigWigs
  packager's `release.json` when a GitHub release ships one
- Cache GitHub API responses on disk and revalidate them with conditional
  requests
- Check and update individual or all managed addons
- Backup existing addon folders before replacement
- Transactional installation with rollback on failure
- Refuse to overwrite folders owned by another managed addon
- Remove folders that a new addon version no longer ships
- Warn when a tag or branch source archive needs the BigWigs packager
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

Install for another client, or into one specific installation:

```bash
./wowtui add github:Owner/Repo --flavor classic-era
./wowtui add github:Owner/Repo --install "/path/to/_retail_"
```

`--flavor` defaults to `retail` and accepts every flavor ID listed under
[Flavors](#flavors). When more than one installation has that flavor, choose
one with `--install`.

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

`update --all` keeps going when one addon fails and ends with a summary line
such as `summary: 3 updated, 5 current, 1 failed` (with `--dry-run`:
`summary: 3 to update, 5 current, 1 failed (dry run)`). It exits with status 1
when any addon failed.

Map an unmanaged installed addon to an active provider without replacing its
files:

```bash
./wowtui map
./wowtui adopt "/path/to/_retail_" WeakAuras github:Owner/WeakAuras
```

`adopt` refuses a folder that another managed addon in the same installation
already owns.

## Flavors

wowtui treats every client directory as its own flavor. Each flavor maps to
the game type its client runs. PTR and beta clients share the game type of the
client whose content they test.

| Directory | Flavor ID | Game type |
| --- | --- | --- |
| `_retail_` | `retail` | mainline |
| `_ptr_` | `ptr` | mainline |
| `_xptr_` | `xptr` | mainline |
| `_beta_` | `beta` | mainline |
| `_classic_` | `classic` | mists |
| `_classic_ptr_` | `classic-ptr` | mists |
| `_classic_beta_` | `classic-beta` | forever |
| `_classic_era_` | `classic-era` | vanilla |
| `_classic_era_ptr_` | `classic-era-ptr` | tbc |
| `_anniversary_` | `anniversary` | tbc |
| `_classic_titan_` | `classic-titan` | titan |
| `_forever_` | `forever` | forever |

The game types of `_classic_`, `_classic_ptr_`, `_classic_beta_`,
`_classic_era_ptr_`, and `_anniversary_` follow what Blizzard ships in those
directories today. They change when those clients move to another expansion.

Discovery probes all twelve directories below a WoW root, both for paths in
`[wow] paths` and for the default Wine and Lutris prefixes
(`~/.wine/drive_c/Program Files/World of Warcraft`,
`~/Games/*/drive_c/Program Files/World of Warcraft`, and the
`Program Files (x86)` variants). A directory found in a default prefix only
counts when it contains `Interface/`. A configured path that is missing or
holds no installation is an error. When a configured path is an installation
directory with an unknown name, `scan` infers the flavor from the addons'
`## Interface` numbers: an addon whose numbers all map to one game type votes
for it, and the vote only counts when exactly one non-test client runs that
game type. A tie leaves the flavor empty.

Providers select builds by game type:

- GitHub first reads the release's `release.json` asset, if it has one (the
  BigWigs packager writes it). The build listed for the client's game type
  wins, and a full build beats a `nolib` build. The packager's `classic` means
  vanilla and `bcc` means tbc. If `release.json` lists builds but none for
  that game type, the install fails with an error that names the game types
  it supports. A `release.json` that cannot be downloaded, is larger than
  1 MiB, is malformed, has no known packager flavor, or names a file that is
  not a ZIP asset of the release is ignored. wowtui downloads it without the
  GitHub token.
- Without a usable `release.json`, GitHub uses a release's only ZIP asset as
  is. With several ZIP assets it picks the one whose name carries a game type
  token from the BigWigs packager names (`vanilla`/`era`/`classicera`,
  `bcc`/`tbc`, `wrath`/`wotlkc`, `titan`, `cata`/`cataclysm`, `mists`,
  `forever`/`camelot`, `retail`/`mainline`). A
  bare `-classic` asset counts as vanilla, as in the packager convention, and
  ranks below an explicit vanilla token. If no asset names the game type, an
  asset without any game type token is used. If there is none, wowtui installs
  the source archive of the release tag.
- CurseForge matches files by the fixed version type ID of the game type.
- WowUpHub matches releases and searches by the hub's game type (`retail`,
  `classic`, `burningCrusade`, `wotlk`, `cata`, `mists`, `forever`). The hub has
  no Titan game type, so `classic-titan` finds nothing there.
- Tag and branch archives, GitLab releases, generic Git, and direct URLs are
  not selected by flavor. wowtui installs the addon folders the archive
  contains.

## Terminal UI

The TUI has three tabs: Addons, Explore (provider search), and Settings. `tab`
or `1`, `2`, `3` switch between them. On the Addons tab, `s` rescans, `r`
checks for updates, `u` updates the selected addon, `U` updates all managed
addons, `a` opens the add dialog, `f` searches, `i` and `I` cycle the game
version and installation filters, and `d` shows details.

Warnings and errors from an operation stay in a notice bar above the key help
until you press `ctrl+x` or start the next operation. The bar shows up to five
lines, fewer on short terminals, and counts the rest as `+N more`.

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
api_key = ""
```

CurseForge activates automatically when `api_key` is non-empty, or when
`WOWTUI_CURSEFORGE_API_KEY` is present in the process environment. The normal
binary also reads a `.env` file in its current directory, but only for
`WOWTUI_GITHUB_TOKEN`, `WOWTUI_WAGO_API_KEY`, and `WOWTUI_CURSEFORGE_API_KEY`.
Every other variable in that file is ignored, and a variable that is already
set in the process environment wins over the file. Values are taken
literally, `$` included; one pair of matching outer quotes is stripped. Do not
put a CurseForge key in a distributed build or commit it to version control.

Provider settings can also be managed from the CLI:

```bash
./wowtui providers
./wowtui providers enable gitlab
./wowtui providers order github gitlab direct
```

Provider credentials can be supplied through environment variables where
supported. They are never stored in the addon registry. An environment value
overrides the matching `config.toml` value for the running process only: when
`providers enable`, `disable`, or `order` rewrite `config.toml`, they keep the
credential the file had and never write the one from the environment.

## GitHub API Cache

wowtui keeps GitHub API responses in `$XDG_CACHE_HOME/wowtui/http` when
`XDG_CACHE_HOME` is an absolute path, otherwise in `~/.cache/wowtui/http`.
Each cached API GET URL gets one file, named after the SHA-256 of the URL, that
holds the URL, the `ETag` and `Last-Modified` values, and the response body.
Only `200` responses with `ETag` or `Last-Modified` and a body of at most
4 MiB are stored. Request headers, and with them the token, are never written. The
directory is created with mode 0700, the files get 0600.

The next request for a cached URL sends the stored values as `If-None-Match`
and `If-Modified-Since`, and a `304 Not Modified` is answered from the cache. An
entry that is unreadable, damaged, or belongs to another URL is ignored and the
URL is fetched in full again. If the directory cannot be written, wowtui works
without the cache. Downloads (release assets, `release.json`, source archives)
are never cached. The cache has no size limit yet; deleting the directory is
safe.

The cache saves rate limit only with a token. According to the GitHub REST API
docs, a `304` leaves the primary rate limit untouched only when the request
carries a valid `Authorization` header. Without a token, the cache
saves bandwidth, but every request still counts against the 60 requests per
hour for unauthenticated clients; with a token the limit is 5,000 per hour.
When GitHub answers `403` or `429` with `X-RateLimit-Remaining: 0`, wowtui
reports that the rate limit is exhausted, adds the reset time when GitHub sends
one, and points to `WOWTUI_GITHUB_TOKEN` or `providers.github.token` when no
token is configured.

## Security Model

Downloaded archives are never extracted directly into `AddOns`. wowtui writes
the download to a hidden work directory `<install>/Interface/.wowtui-work-*`
and extracts it there, so the final moves into `AddOns` are renames on the same
filesystem. Downloads are capped at 512 MiB. Extraction rejects absolute and
Zip Slip paths, symlinks and other non-regular entries, more than 20,000 files,
more than 512 MiB in total, and single files over 256 MiB.

Before anything in `AddOns` changes, the installer:

- refuses the install when the archive contains a folder that another managed
  addon in the same installation owns, and names the folder and its owner
- refuses addon folder names that are empty, start with a dot, or contain `/`,
  `\`, or NUL, and targets that exist as a file
- copies every folder it will replace or remove into a timestamped backup next
  to the registry (by default `~/.local/share/wowtui/backups/`)

It then moves the old folders into a hidden rollback directory
`<install>/Interface/.wowtui-rollback-*` and moves the new ones into `AddOns`.
If a step fails, the old folders return from the rollback directory. Work and
rollback directories are removed afterwards, on success and on failure.

Replacing a folder that no addon manages still works, but wowtui shows a
warning with the backup path. On update, folders the previous version had and
the new one no longer ships are removed; they are in the backup and come back
on rollback. A dropped folder that another managed addon still claims stays.

Generic Git sources are shallow-cloned and zipped. A checkout that contains a
symlink or another non-regular file is rejected. Tag and branch source
archives get a warning when `.pkgmeta` or `pkgmeta.yaml` lists `externals`, or
when a `.toc` still contains `@project-version@`. Both mean the archive was not
built by the BigWigs packager; the warning suggests `--source release`.

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
