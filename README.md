# wowtui

[![Go](https://img.shields.io/badge/Go-1.26.11-00ADD8?logo=go&logoColor=white)](https://go.dev/)
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
- Map unmanaged addons to providers from TOC hints and an optional signed
  addon index, and adopt them without replacing files
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
./wowtui map --adoptable
./wowtui map --changes
./wowtui adopt "/path/to/_retail_" WeakAuras github:WeakAuras/WeakAuras2
```

`adopt` refuses a folder that another managed addon in the same installation
already owns. See [Mapping Unmanaged Addons](#mapping-unmanaged-addons).

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

## Mapping Unmanaged Addons

`wowtui map` lists provider candidates for installed addon folders that no
managed addon owns. It prints one row per candidate with the columns
`FOLDER`, `FLAVOR`, `PROVIDER`, `AVAILABILITY`, `REFERENCE`, `REASON`, and
`INSTALL`, sorted by installation path, folder, provider, and reference, so
repeated runs print the same order. `--adoptable` keeps only candidates whose
provider is active.

wowtui ships file hashes for the five folders in the official WeakAuras
5.21.1 and 5.22.1 releases. Matching works locally without a WowUp profile,
provider credentials, or a configured index. A signed index can supply hashes
for other releases. A matching TOC version identifies which release to check.
wowtui reads the files in all declared folders and offers the package when a
majority of its files match and at least half match in each folder.
`release:fingerprint` in the reason column identifies these candidates.
Use `map --changes` to list added, modified and missing paths once per release
and installation.

Other candidates come from TOC hints, the release changelog, local WowUp
installation records and the optional [Addon Index](#addon-index).

- `X-Source-Repo` and `X-Website` yield a GitHub, GitLab, or WowUp reference,
  or a CurseForge reference with a numeric project ID. `X-Source-Repo` also
  yields a `git:` reference when the value is an https, ssh, or git URL that
  names a repository below an owner. A plain web page never becomes a `direct`
  or `git` candidate.
- `X-Curse-Project-ID` yields `curseforge:<id>`. `X-Wago-ID` and `X-WoWI-ID`
  yield `wago:<id>` and `wowinterface:<id>`, which are informational: wowtui
  has no active provider for them.
- Several hints that point to the same provider and reference become one row,
  and `REASON` lists the hint keys, comma separated.
- A packaged `CHANGELOG.md` can identify a GitHub repository when its title
  matches the folder or TOC title and both its release and comparison links
  name the same repository and installed TOC version. The reason is
  `packager:changelog`.

On Linux, a scan looks for `preferences.json` and `addons.json` in the
`WowUpCf` and `WowUp` directories under `$XDG_CONFIG_HOME` (by default
`~/.config`). The installation ID links each package's folder list to its
WoW client. A scanned folder with the same name in that client can match a
recorded CurseForge project ID, GitHub owner/repository or WowUpHub ID,
including a project's alternate provider IDs. These matches show
`wowup:installed-folder` as their reason. Scans skip records marked ignored.

The `REFERENCE` column holds the prefixed form that `adopt` and `add` accept:
`github:`, `gitlab:`, `wowup:`, `curseforge:`, or `git:`. A legacy
`https://wow.curseforge.com/projects/<slug>` URL is read as a CurseForge
reference, never as a direct download. CurseForge resolves numeric IDs, so
`map` does not suggest a slug from such a URL.

`AVAILABILITY` tells whether `adopt` can use the candidate right now:

| Value | Meaning |
| --- | --- |
| `active` | The provider is enabled locally. Import checks access to the project when registering a new package. |
| `paused` | WowUpHub and Wago, paused pending upstream approval |
| `needs-api-key` | CurseForge without a configured API key |
| `disabled` | The provider was turned off with `providers disable` |
| `unsupported` | wowtui has no provider for this reference, such as `wowinterface:` |

## Addon Index

The addon index is an optional, signed list of addon packages. It helps
`map` recognize installed folders, and it gives addons short names:
`wowtui add index:<id>` and `wowtui adopt <install-path> <folder> index:<id>`.

An index is a JSON file plus a detached signature in `<file>.sig`. wowtui
verifies the signature with the public key from the config before it parses
anything. The format is described in [SPEC.md](SPEC.md). The repository ships
no index, only a test fixture: build one with the
[maintainer tool](#maintainers-building-the-index) or get one from someone you
trust.

```bash
./wowtui index use --path /path/to/index.json --public-key <base64-key>
./wowtui index status
./wowtui index search weakauras
./wowtui add index:weakauras
./wowtui adopt "/path/to/_retail_" WeakAuras index:weakauras
```

- `index use` loads and verifies the file with the given key, and only then
  saves `[index] path` and `public_key` in the config. The signature file is
  always `<path>.sig`.
- `index status` prints the path, the key fingerprint (`sha256:` and the first
  16 hex digits of the SHA-256 of the key), whether the index loads, the entry
  count, and the build time. It exits with an error when the index does not
  load.
- `index search <query>` matches the entry ID or name, ignoring case, and
  prints ID, name, source, folders, game types, whether a person reviewed the
  entry, and funding links.
- `add index:<id>` resolves the entry to its source and installs from there.
  It prints `index:<id> is github:Owner/Repo`, with `(unreviewed)` appended
  when the entry is not reviewed.
- `adopt ... index:<id>` registers every folder of the entry that the
  installation has, under the entry's source. The folder you name must be one
  of the entry's folders. It refuses when another managed addon owns any of
  those folders.
- `map` adds index candidates. `REASON` is `index:folder` when an entry lists
  the folder (compared ignoring case), `index:curseforge`, `index:wago`, or
  `index:wowinterface` when the folder's TOC ID matches the entry's ID, and
  `unreviewed` is added when the entry is not reviewed. Index candidates come
  first within a folder.

If a configured index fails to load (missing file, bad signature, invalid
content), `map`, `add`, and `adopt` print one line
`warning: index not loaded, continuing without it: <reason>` to stderr and go
on without it. An `index:<id>` reference is the exception: it fails with
`index not loaded` or `no index configured`, because there is nothing to
resolve it with. Older registry entries can lack file hashes. For those
entries, `check` and `update` consult the bundled manifests and any configured
signed index. If the configured index cannot be verified, the lookup reports
an error.

Trust model: the index holds metadata and source references only, never a
download URL. Entries may point at GitHub or GitLab repositories only. wowtui
always resolves releases from the source repository through the normal
provider, with the same release selection, archive checks, backup, and
rollback as for any other reference. A valid signature shows that the file
comes from the holder of the key you configured. It says nothing about the
addon code. An entry that is not reviewed was generated, not checked by a
person.

Open points:

- The index is read from a local file only. There is no download or update
  mechanism yet.
- There is no rollback protection: wowtui does not remember the build time of
  an earlier index, so it accepts an older, validly signed file.
- Index candidates in `map` and `add index:<id>` do not check the entry's game
  types against the installation's flavor.

## Terminal UI

The TUI has three tabs: Addons, Explore (provider search), and Settings. On
the Addons tab, `tab` or `2` opens Explore and `3` opens Settings. On the
Settings tab, `tab`, `esc`, or `1` go back to Addons and `2` opens Explore.
On the Addons tab, `s` rescans, `r` checks for updates, `u` updates the
selected addon, `U` updates all managed addons, `a` opens the add dialog, `f`
searches, `i` and `I` cycle the game version and installation filters, and
`d` shows details.

Keys you type stay in the focused input. Explore opens with its search field
focused, so `q`, `a`, `d`, `/`, `1`, and `3` are text there. Only `enter`
(search), `esc` (clear the field and leave it), `tab`, `shift+tab`, and
`ctrl+c` act. After `esc` or a search, the table keys work: `a` installs the
selected result, `d` shows its details, `/` focuses the field again, `tab`
opens Settings, and `shift+tab` or `1` returns to Addons. `a` does nothing
while another operation runs. In the add dialog, the reference field takes
every key, `q` included; `q` quits only from the other fields. The `f` filter
on the Addons tab takes keys the same way until `enter` or `esc`.

### Import installed addons

Press `s` on the Addons tab to refresh the scan, then `m` to import existing
folders. The import screen shows source matches for the selected game and
installation filters.

Use the arrow keys to browse and Space to select a folder and its source.
Press `a` to select all folders with a single match. Where a folder has
several candidates, select the source you want with Space. Press
Enter to review the batch and `y` to confirm. Import verifies the source and
records the installed folders in the registry. It leaves their files in place.
Folders selected for the same source in one installation are managed together.
For a fingerprint match, selecting one row selects the whole release. Press
`d` to inspect its folders and any file differences. Import rechecks the
selection before registering it and records the original release hashes.

When updating an addon with known file hashes, `u` shows local differences.
Enter or Esc keeps the local copy. Press `y` to replace the reviewed files
with the selected upstream release, even if the version is already current.
A further edit requires a fresh review. Bulk updates skip affected packages
and continue with the others. In the CLI, a single-addon `update` prints the
file list and a `--replace-local=<token>` argument for that review. If you
manage the same addon in multiple clients, use its full reference and choose
the installation, e.g. `wowtui update WeakAuras/WeakAuras2 --install "/path/to/_retail_"`.
New installs
and successful updates save file hashes for future comparisons.

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

[index]
path = "/home/user/.local/share/wowtui/index.json"
public_key = "<base64 ed25519 public key>"
```

The `[index]` section is optional and is normally written by
`wowtui index use`. See [Addon Index](#addon-index).

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

### Archive sandbox

`add` and `update` start a private extraction worker from the installed
`wowtui` binary. The parent checks the finished staging tree before reading
addon metadata or replacing installed files. On Linux, Bubblewrap isolates the
worker from the host network, home directory, registry, and live addon files.
It mounts the archive read-only and gives the worker a writable staging area.
Linux amd64 and arm64 have seccomp profiles for the worker.

Both settings default to `auto`. To require the strongest backend, set:

```toml
[security]
sandbox = "required"
seccomp = "required"
```

| Setting | Behavior |
| --- | --- |
| Both `auto` | On Linux, use Bubblewrap with seccomp where available. Warn and use Bubblewrap alone if filtering is unavailable, or run the worker directly if Bubblewrap is unavailable. Other platforms run the worker directly. |
| `sandbox = "required"` | Fail extraction if Bubblewrap is unavailable. With seccomp on `auto`, Bubblewrap alone is sufficient. |
| `seccomp = "required"` | Require both Bubblewrap and the seccomp filter. |

The CLI and TUI report the selected backend. The filter terminates a worker
that attempts an unexpected syscall. An extraction error stops the
install or update. Fallback takes place only during capability selection,
before the worker processes the archive. The direct worker uses the same ZIP
validation but has the user's filesystem permissions.

## Maintainers: Building the Index

`tools/indexbuild` builds and signs an index. It is a maintainer tool, not part
of the `wowtui` binary, and runs from a checkout:

```bash
go run ./tools/indexbuild keygen   --out DIR
go run ./tools/indexbuild discover --registry registry.json --out candidates.json --cache DIR
go run ./tools/indexbuild fingerprint --zip Addon-1.0.zip --version 1.0 --out manifest.json
go run ./tools/indexbuild build    --candidates candidates.json --key signing.key --out index.json
```

- `keygen` writes `signing.key` (mode 0600, the base64 ed25519 seed) and
  `signing.pub` (the base64 public key to give to users) into `--out`. It
  refuses to overwrite existing files. Keep the private key out of version
  control.
- `discover` reads the folder names and TOC hints of a wowtui `registry.json`
  (not the installation paths) and looks up each addon's GitHub repository.
  It writes `candidates.json` and a review file, by default `--out` with
  `.json` replaced by `.review.json` (`--review` overrides it). The review
  file lists what needs a person: ambiguous, filtered, unresolved, and
  conflicting lookups, hints that could not be confirmed or that disagree, and
  API errors. `--mirror-owners` takes a comma separated list of GitHub
  accounts whose repositories are copies and never count as originals
  (default `hippuli`, empty for none).
- `fingerprint` produces a manifest for a packaged release ZIP. Add it to the
  package's `fingerprints` array in `candidates.json` before signing.
  The builder selects format 2 when a package has manifests. The bundled
  releases have [source checksums and reproduction instructions](internal/fingerprint/data/README.md).
- `build` turns `candidates.json` into `index.json` and `index.json.sig`. It
  skips packages the index cannot hold and prints each as a warning. It
  verifies the pair with the same loader wowtui uses before it writes
  anything, and it refuses a private key file that group or others can
  access. Every entry it writes has `reviewed` set to false, and every source
  is a GitHub repository.

All GitHub access goes through `gh api` with GET requests only: code search,
repository, contents, and release reads. `gh` keeps the credentials; the tool
never reads, stores, or passes a token. Every response, misses and 404s
included, is cached in the `--cache` directory, so a rerun makes no repeated
calls. Delete the directory to fetch fresh data. Code search requests are
spaced at least seven seconds apart. On a rate limit answer (`403` or `429`
with `Retry-After` or `X-RateLimit-Remaining: 0`) the tool waits for the
reset, at most 15 minutes per wait and six waits per request. Server errors
are retried up to four attempts, ten seconds apart.

Output is deterministic: packages, folders, and IDs are sorted, entry IDs
depend only on the set of repositories, `generated_at` is the newest fetch
time of any answer used, and ed25519 signatures are deterministic. The same
candidates and key give byte-identical `index.json` and `index.json.sig`.

## Development

The project uses test-driven development and requires Go 1.26.11 or later.
The `govulncheck` version is pinned in `go.mod`. Run these checks from the
repository root.

```bash
gofmt -l .
go vet ./...
go test ./...
go tool govulncheck ./...
```

On pull requests and pushes to `main`, CI requires the vulnerability scan to
pass before either native acceptance job starts. A reachable advisory or a
scanner error fails the check.

The M3 native acceptance job runs the following command independently on Linux
amd64 and arm64, on hosts with working Bubblewrap, user namespaces, and seccomp:

```bash
M3_NATIVE=1 WOWTUI_TEST_NATIVE_SANDBOX=1 go test -tags=m3native -count=1 -v ./internal/sandbox/... ./acceptance
```

The acceptance test builds the production binary, installs and updates a
fixture from a local HTTP server with both protections required, and compares
the registry and installed tree after a rejected update. The native job also
checks filesystem and network isolation and exercises a filtered syscall.
A passing run on each native architecture qualifies that architecture.

The code is organized so the service and provider layers can be tested without
a terminal. Provider HTTP tests use local test servers rather than live APIs.

## License

MIT. See [LICENSE](LICENSE).
