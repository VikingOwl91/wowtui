# wowtui Spec Sheet

Stand: 2026-11-18

## 1. Ziel

Ein schlanker, sicherer WoW-Addon-Updater als TUI. Kein WowUp-Klon.
Der Scope ist bewusst eng: GitHub Releases und direkte ZIP-URLs,
saubere Installation mit Backup und Rollback, mehrere WoW-Versionen.

Das Projekt ist gleichzeitig Portfolio-Stück: Dateisystemarbeit,
HTTP-APIs, Archive, parallele Downloads, persistenter Zustand,
Updateplanung, TUI, Sandboxing.

WowUp dient als konzeptionelle Inspiration für Provider-Logik,
Matching und Multi-Flavor-Handling. wowtui ist eine eigenständige
Go-Implementierung; es wird kein WowUp-Code kopiert.

## 2. Scope

### Version 0.1 (MVP)

- Befehle: `scan`, `list`, `add`, `check`, `update <addon>`,
  `update --all`, plus TUI-Modus ohne Argumente
- `add` akzeptiert `github:Owner/Repo` oder eine GitHub-URL. Addons
  lassen sich manuell per Link installieren oder als GitHub-Repo in
  die Registry aufnehmen
- Provider: GitHub Releases, Direct-URL (ZIP)
- Registry: lokale `registry.json`
- SHA-256-Hashes über heruntergeladene Archive und entpackte Ordner
- Backup vor jedem Update, Rollback bei Fehlern
- Dry-Run für `update`
- Flavors: Retail und Classic

Nicht in 0.1:

- Suche über mehrere Plattformen
- Automatische Migration fremd installierter Addons
- Abhängigkeiten zwischen Addons
- Wago

### Danach

- v0.2: Wago als zweiter Provider (dokumentierte API, API-Key nötig)
- v0.3+: SQLite statt JSON, sobald mehrere Installationen,
  Releasehistorien oder umfangreiche Metadaten dazukommen
- CurseForge nur mit eigenem API-Key des Users, wowtui liefert nie einen
  Key mit; die Einhaltung der CurseForge-ToS (Nutzungsbedingungen,
  Downloadfreigaben) bleibt Sache des Users und des Projekts

## 3. Tech Stack

| Baustein | Wahl | Anmerkung |
|---|---|---|
| Sprache | Go | |
| TUI | Bubble Tea | Model-Update-View |
| TUI-Komponenten | Bubbles | Tabellen, Spinner, Texteingaben |
| Styling | Lip Gloss | Rahmen, Layout |
| CLI | Cobra | Subbefehle neben der TUI |
| Konfiguration | TOML, XDG-Pfade | kein Viper, eigener Loader reicht |
| Registry | JSON (0.1), später SQLite | |
| Sandbox | Bubblewrap, optional, nur Linux | |

## 4. Architektur

Vier Schichten, strikt getrennt:

```
TUI (Bubble Tea) / CLI (Cobra)
 │
Application Service
 │
├── Installation Scanner   (WoW-Installationen finden)
├── Addon Registry         (Zustand, registry.json)
├── Update Planner         (Versionsabgleich, Auswahl nach Spieltyp)
└── Package Installer      (transaktionale Installation)
     │
     ├── GitHub Provider
     ├── GitLab Provider
     ├── Generic-Git Provider
     ├── Direct-URL Provider
     ├── CurseForge Provider   (nur mit eigenem API-Key)
     ├── WowUpHub Provider     (pausiert)
     └── (später) Wago Provider
```

Bubble Tea ist reine Präsentationsschicht. Der Kern funktioniert ohne
Terminal, damit CLI, Tests und spätere Headless-Nutzung denselben Code
verwenden.

### Repository-Layout

Ohne Testdateien:

```
main.go                 (.env laden, cmd.Execute)

cmd/
├── root.go             (Flags, Provider-Aufbau, Service)
├── add.go
├── check.go
├── installs.go
├── list.go
├── map.go              (map und adopt)
├── providers.go
├── rename.go
├── scan.go
├── tui.go
└── update.go

internal/
├── addon/
│   ├── addon.go
│   ├── scanner.go
│   └── toc.go
├── config/
│   ├── config.go
│   └── dotenv.go
├── installer/
│   ├── archive.go
│   ├── backup.go
│   ├── download.go
│   ├── fs.go
│   ├── installer.go
│   ├── naming.go
│   └── transaction.go
├── provider/
│   ├── provider.go
│   ├── reference.go
│   ├── manager.go
│   ├── assets.go
│   ├── releasejson.go
│   ├── httpcache.go
│   ├── tags.go
│   ├── github.go
│   ├── gitlab.go
│   ├── git.go
│   ├── direct.go
│   ├── curseforge.go
│   └── wowuphub.go
├── registry/
│   └── registry.go
├── sandbox/
│   └── sandbox.go      (noch leer, siehe M3)
├── service/
│   ├── service.go
│   ├── event.go
│   └── packager.go
├── tui/
│   ├── tui.go
│   ├── model.go
│   ├── addons.go
│   ├── explore.go
│   ├── details.go
│   ├── add.go
│   ├── settings.go
│   ├── fuzzy.go
│   └── styles.go
└── wow/
    ├── wow.go
    ├── flavor.go
    └── discovery.go
```

### Kern-Interfaces

```go
// internal/provider
type Provider interface {
    Resolve(ctx context.Context, ref Reference) (RemoteAddon, error)
    Latest(ctx context.Context, addon RemoteAddon, flavor wow.GameFlavor, source string) (Release, error)
    Download(ctx context.Context, release Release) (io.ReadCloser, error)
}

type SearchProvider interface {
    Provider
    Search(ctx context.Context, query string, flavor wow.GameFlavor) ([]RemoteAddon, error)
}

// internal/service: Struct ohne Interface, CLI und TUI nutzen dieselben Methoden
func (s *Service) Scan(ctx context.Context, configPaths []string) ([]registry.Installation, error)
func (s *Service) Add(ctx context.Context, ref provider.Reference, flavor wow.GameFlavor, install wow.Installation, source string) error
func (s *Service) Check(ctx context.Context) ([]UpdateStatus, error)
func (s *Service) Update(ctx context.Context, refOrName string, dryRun bool) error
func (s *Service) UpdateAll(ctx context.Context, dryRun bool) error
func (s *Service) MapUnmanaged() ([]MappingCandidate, error)
func (s *Service) Adopt(ctx context.Context, installPath, folderName string, ref provider.Reference) error

// geplant (M3), internal/sandbox ist noch leer
type Sandbox interface {
    Run(ctx context.Context, operation Operation) (Result, error)
}
```

### Provider und API-Keys

Alle Provider sind über User-konfigurierbare API-Keys in der
TOML-Konfiguration nutzbar:

```toml
[providers.github]
token = ""          # optional, hebt GitHub-Rate-Limits an

[providers.wago]
api_key = ""        # erforderlich für den Wago-Provider

[providers.curseforge]
api_key = ""        # erforderlich, eigener Key des Users, nie mitgeliefert
```

Regeln:

- Keys liegen nur in der Konfigurationsdatei (XDG-Pfad, Dateirechte
  0600) oder in Umgebungsvariablen, nie in der Registry oder im Repo
- Umgebungsvariablen: `WOWTUI_GITHUB_TOKEN`, `WOWTUI_WAGO_API_KEY`,
  `WOWTUI_CURSEFORGE_API_KEY`. Sie überschreiben die Werte aus
  `config.toml` nur im laufenden Prozess. Schreibt `wowtui providers`
  die Konfiguration neu, bleibt der Wert aus der Datei stehen; ein Key
  aus der Umgebung landet nie in `config.toml`
- Eine `.env` im Arbeitsverzeichnis wird gelesen, aber nur für diese
  drei Variablen. Andere Einträge werden ignoriert, schon gesetzte
  Umgebungsvariablen gewinnen. Werte werden wörtlich übernommen, ein
  Paar passender äußerer Anführungszeichen wird entfernt
- Ein Provider ohne gültigen Key wird übersprungen und meldet einen
  klaren Fehler, statt still zu scheitern
- GitHub funktioniert ohne Token, mit Token nur für höhere
  Rate-Limits (laut GitHub-Doku 60 Anfragen pro Stunde ohne, 5000 mit
  Token). Antwortet die API mit `403` oder `429` und
  `X-RateLimit-Remaining: 0`, meldet wowtui ein erschöpftes Rate-Limit
  mit Reset-Zeit, sofern GitHub sie mitschickt, und ohne Token mit dem
  Hinweis auf `WOWTUI_GITHUB_TOKEN` oder `providers.github.token`
- Direct-URL braucht keinen Key

Provider können zusätzlich aktiviert/deaktiviert und priorisiert werden:

```toml
[providers]
order = ["wowuphub", "wago", "curseforge", "github", "gitlab", "git", "direct"]

[providers.git]
enabled = false
```

Die Reihenfolge oben ist der Default; nicht genannte Provider hängt
wowtui in dieser Reihenfolge an. `enabled` gilt für GitHub, GitLab,
Generic Git und Direct. CurseForge hängt nur am API-Key, WowUpHub und
Wago bleiben unabhängig von `enabled` pausiert.

Die CLI stellt dafür `wowtui providers`,
`wowtui providers enable <name>`,
`wowtui providers disable <name>`,
`wowtui providers order <name>...` und
`wowtui providers test <name>` (Authentifizierung prüfen, ohne
Credentials auszugeben; derzeit nur CurseForge) bereit. Die TUI zeigt dieselbe
Reihenfolge und den Aktivierungsstatus in den Settings.

GitHub, GitLab, Generic Git, Direct, WowUpHub und CurseForge sind implementiert.
GitHub, GitLab, Generic Git und Direct sind standardmäßig aktiv. CurseForge wird
aktiv, sobald ein lokaler API-Key konfiguriert ist. WowUpHub bleibt bis zur Antwort
der WowUp-Entwickler auf eine Anfrage zur Drittanbieter-Nutzung
pausiert. Wago bleibt bis zur Klärung der offiziellen Client-API auf
pending-approval. CurseForge benötigt einen API-Key für Drittanbieter-Clients.

### GitHub ohne Releases (Fallback-Kette)

Viele Repos haben keine Releases oder keine ZIP-Artefakte. Der
GitHub-Provider nutzt deshalb eine Fallback-Kette:

1. Neuestes stabiles Release mit passendem ZIP-Asset für den Spieltyp
   des Flavors. Hat das Release ein `release.json` (BigWigs-Packager),
   entscheidet dessen Eintrag für den Spieltyp; listet es Builds, aber
   keinen für diesen Spieltyp, bricht wowtui mit einem Fehler ab, der
   die unterstützten Spieltypen nennt, ohne zu Schritt 2 zu gehen.
   Sonst Auswahl über die Asset-Namen (beides siehe Abschnitt 5)
2. Release vorhanden, aber ohne passendes ZIP-Asset: Quellarchiv des
   Release-Tags
3. Kein Release: Quellarchiv des neuesten Tags (semver-Sortierung,
   bei nicht parsebaren Tags API-Reihenfolge)
4. Auch kein Tag: Quellarchiv des Default-Branch, Version ist der
   Commit-SHA

GitHub liefert diese Archive automatisch unter
`github.com/Owner/Repo/archive/refs/tags/<Tag>.zip` beziehungsweise
`.../refs/heads/<Branch>.zip`. Der Installer entpackt den
Wrapper-Ordner (`Repo-Tag/`), den diese Archive mitbringen. Bleibt
genau ein Addon-Ordner übrig, bekommt er den Namen, den seine
`.toc`-Dateien gemeinsam ergeben; Client-Suffixe wie `_Mainline` oder
`-Vanilla` fallen dabei weg. Ergibt das keinen sicheren Ordnernamen
(leer, mit Punkt am Anfang, mit `/`, `\` oder NUL), bleibt der
bisherige Name.

Versionsidentität je Quelle: Release = Release-ID, Tag = Tag-Name,
Branch = Commit-SHA. Die Registry speichert die Quelle im
`source`-Feld. Damit funktionieren auch Übergänge: per Tag
installiert, das Repo bekommt später ein Release, der Check meldet
das als Update.

Unmanaged installierte Addons können über einen Mapping-Workflow einem
Provider zugeordnet werden. Der Workflow nutzt TOC-Hints zuerst und
verlangt eine explizite Bestätigung über `adopt`, bevor ein Addon als
verwaltet gespeichert wird. Die vorhandenen Dateien werden dabei nicht
ersetzt; das passiert erst beim folgenden Update. `adopt` verweigert
einen Ordner, den in derselben Installation schon ein anderes
verwaltetes Addon besitzt.

`add --source release|tag|branch` pinnt die Quelle. Updates folgen
dann dem Pin statt der Kette.

Einschränkung: Tag- und Branch-Archive kennen kein Flavor-Filtering
und keine Asset-Auswahl. Installiert werden die Ordner mit
`.toc`-Dateien auf oberster Ebene des Repos, bei einem flachen Repo
das Repo selbst. Weil solche Archive nicht vom BigWigs-Packager gebaut
sind, warnt wowtui bei Tag- und Branch-Archiven, wenn `.pkgmeta` oder
`pkgmeta.yaml` `externals` listet (diese Bibliotheken können dann
fehlen) oder eine `.toc` noch `@project-version@` enthält. Die Warnung
empfiehlt `--source release`.

### HTTP-Cache für die GitHub-API

GitHub-API-Antworten liegen in `$XDG_CACHE_HOME/wowtui/http`, wenn
`XDG_CACHE_HOME` ein absoluter Pfad ist, sonst in
`~/.cache/wowtui/http` (`cmd/root.go` `httpCacheDir`,
`internal/provider/httpcache.go`):

- Je gecachter API-GET-URL eine Datei `<SHA-256 der URL>.json` mit
  URL, `ETag`, `Last-Modified` und Body
- Gespeichert werden nur `200`-Antworten mit `ETag` oder
  `Last-Modified` und höchstens 4 MiB Body. Request-Header und damit
  der Token landen nie im Cache. Verzeichnis 0700, Dateien 0600
- Folgeanfragen schicken die gespeicherten Werte als `If-None-Match`
  und `If-Modified-Since`; ein `304` wird aus dem Cache beantwortet
- Unlesbare, beschädigte oder zu einer anderen URL gehörende Einträge
  werden ignoriert und neu geholt. Ist das Verzeichnis nicht
  schreibbar, läuft wowtui ohne Cache
- Downloads (Release-Assets, `release.json`, Quellarchive) werden nie
  gecacht
- Noch keine Größenbegrenzung; das Verzeichnis darf jederzeit gelöscht
  werden

Laut GitHub-Doku zählt ein `304` nur dann nicht gegen das primäre
Rate-Limit, wenn der Request korrekt mit `Authorization`-Header
autorisiert ist. Ohne Token spart der Cache also Bandbreite, aber kein
Kontingent.

### Datenmodell

Ein Ordner ist nicht immer ein Addon. Ein Paket kann mehrere Ordner
enthalten (Details/, Details_Streamer/, Details_TinyThreat/). Deshalb
zwei Ebenen:

```go
// internal/registry, gekürzt
type Folder struct {           // gescannter Ordner einer Installation
    Name       string
    Version    string
    Interfaces []int
    TocFiles   []string
    Hints      map[string]string
}

type Addon struct {            // verwaltetes Paket
    Provider    string
    Ref         string
    Flavor      string
    InstallPath string
    Folders     []string       // Ordnernamen in AddOns
    Version     string
    Source      string         // auto, release, tag, branch
    ReleaseID   int64
    AssetName   string
    SHA256      string
}

// internal/wow
type GameFlavor string         // ein Flavor je Client-Verzeichnis

const (
    Retail        GameFlavor = "retail"
    PTR           GameFlavor = "ptr"
    XPTR          GameFlavor = "xptr"
    Beta          GameFlavor = "beta"
    Classic       GameFlavor = "classic"
    ClassicPTR    GameFlavor = "classic-ptr"
    ClassicBeta   GameFlavor = "classic-beta"
    ClassicEra    GameFlavor = "classic-era"
    ClassicEraPTR GameFlavor = "classic-era-ptr"
    Anniversary   GameFlavor = "anniversary"
    ClassicTitan  GameFlavor = "classic-titan"
    Forever       GameFlavor = "forever"
)

type GameType string           // Spielinhalt, nach dem Provider auswählen

const (
    GameMainline GameType = "mainline"
    GameVanilla  GameType = "vanilla"
    GameTBC      GameType = "tbc"
    GameWrath    GameType = "wrath"
    GameTitan    GameType = "titan"
    GameCata     GameType = "cata"
    GameMists    GameType = "mists"
    GameForever  GameType = "forever"
)
```

Zuordnung Ordner → Paket geht nicht zuverlässig über den Ordnernamen.
Die Zuordnung läuft über die Registry-Metadaten aus der Installation.
`add`, `update` und `adopt` verweigern Ordner, die in derselben
Installation schon ein anderes verwaltetes Addon besitzt. Bei `update`
sind die Ordner ausgenommen, die das Addon selbst schon hatte.

### Versionsvergleich

Reiner Semver-Vergleich reicht nicht. Versionen kommen vor als
`1.2.3`, `v1.2.3`, `11.2.0-4`, `@project-version@`,
`release-2026-08-01`. Auflösungsreihenfolge:

1. Provider-Datei-ID bzw. Release-ID
2. Gespeicherte Download-Metadaten aus der Registry
3. Git-Tag oder Release-Name
4. Addon-Version aus der `.toc`
5. Notfalls Hashvergleich (ändert sich = Update verfügbar)

## 5. WoW-Erkennung

- Automatische Suche nach Installationen plus manuelle Pfade in der
  Konfiguration (`[wow] paths`). Automatisch durchsucht werden
  `~/.wine/drive_c/Program Files/World of Warcraft`,
  `~/Games/*/drive_c/Program Files/World of Warcraft` und jeweils die
  Variante mit `Program Files (x86)`. Dort zählt ein Client-Verzeichnis
  nur, wenn es `Interface/` enthält. Ein konfigurierter Pfad, der fehlt
  oder keine Installation enthält, ist ein Fehler
- Flavorkennung über die Verzeichnisnamen, ein Flavor je
  Client-Verzeichnis (`internal/wow/flavor.go`):

  | Verzeichnis | Flavor | Spieltyp |
  |---|---|---|
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

  PTR- und Beta-Clients teilen den Spieltyp des Clients, dessen Inhalt
  sie testen. Die Spieltypen von `_classic_`, `_classic_ptr_`,
  `_classic_beta_`, `_classic_era_ptr_` und `_anniversary_` folgen dem,
  was Blizzard dort aktuell ausliefert, und müssen nachgezogen werden,
  sobald diese Clients die Erweiterung wechseln
- Hat eine Installation keinen bekannten Verzeichnisnamen, leitet
  `scan` den Flavor aus den Interface-Nummern ab. Jeder Addon-Ordner,
  dessen Nummern alle auf denselben Spieltyp zeigen, stimmt für diesen
  Spieltyp. Die Stimme zählt nur, wenn genau ein Client ohne Test-Status
  diesen Spieltyp hat (Wrath und Cata haben derzeit keinen). Bei
  Gleichstand bleibt der Flavor leer
- Addons liegen in `<install>/Interface/AddOns/<Name>/`
- `.toc`-Dateien werden geparst: Version, Interface-Nummern (auch
  kommagetrennt), Titel und `X-`-Felder mit Quellhinweisen für das
  Mapping
- Dieselbe Addon-Quelle kann pro Flavor ein anderes Release brauchen.
  Ausgewählt wird nach dem Spieltyp des Flavors, nicht nach
  "neueste ZIP":
  - GitHub mit `release.json` (vom BigWigs-Packager geschrieben): Es
    gilt der Build, den die Datei für den Spieltyp nennt, volle Builds
    vor `nolib`. PTR- und Beta-Clients nutzen dabei den Spieltyp ihres
    Basis-Clients. Packager-`classic` ist Vanilla, `bcc` ist TBC.
    Listet die Datei Builds, aber keinen für den Spieltyp, scheitert
    die Installation mit einem Fehler, der die unterstützten Spieltypen
    nennt. Ist `release.json` nicht ladbar, größer als 1 MiB, kaputt,
    ohne bekannten Packager-Flavor oder nennt es keine ZIP-Datei des
    Releases, greift die Namensauswahl. `release.json` wird ohne Token
    geladen
  - GitHub nach Namen: Ein einzelnes ZIP-Asset wird immer genommen.
    Bei mehreren gewinnt das Asset, dessen Name den Spieltyp trägt
    (Tokens nach den Dateinamen des BigWigs-Packagers:
    `vanilla`/`era`/`classicera`, `bcc`/`tbc`, `wrath`/`wotlkc`,
    `titan`, `cata`/`cataclysm`, `mists`, `forever`/`camelot`,
    `retail`/`mainline`). Ein nacktes
    `-classic` gilt wie beim Packager als Vanilla und steht hinter
    expliziten Vanilla-Tokens. Danach kommt ein Asset ohne
    Spieltyp-Token, sonst das Quellarchiv des Release-Tags
  - CurseForge: feste Version-Type-ID je Spieltyp
  - WowUpHub: Spieltyp des Hubs (`retail`, `classic`, `burningCrusade`,
    `wotlk`, `cata`, `mists`, `forever`); für Titan gibt es keinen, dort
    findet `classic-titan` nichts
  - Tag- und Branch-Archive, GitLab-Releases, Generic Git und
    Direct-URL: keine Auswahl nach Flavor

## 6. Installations-Pipeline

Transaktional, in dieser Reihenfolge:

```
ZIP nach <install>/Interface/.wowtui-work-*/ herunterladen
→ dabei SHA-256 berechnen, Download auf 512 MiB begrenzt
→ im selben Arbeitsordner nach staging/ entpacken (nie direkt in AddOns)
→ Pfade auf Zip-Slip und absolute Pfade prüfen
→ Inhalt kontrollieren (nur Dateien und Verzeichnisse, höchstens
  20000 Dateien, 512 MiB gesamt, 256 MiB je Datei)
→ Addon-Ordner über .toc-Dateien finden, Wrapper-Ordner auflösen
→ Besitz prüfen: Ordner eines anderen verwalteten Addons → Abbruch
→ bei Tag- und Branch-Archiven: Packager-Warnungen
→ Ordnernamen prüfen: unsichere Namen oder Datei am Ziel → Abbruch
→ zu ersetzende und weggefallene Ordner ins Backup kopieren
→ diese Ordner nach <install>/Interface/.wowtui-rollback-*/ verschieben
→ neue Ordner nach AddOns verschieben
→ Registry aktualisieren
→ bei Fehler: beiseitegelegte Ordner aus dem Rollback-Ordner zurück
```

Arbeits- und Rollback-Ordner sind versteckte Temp-Ordner in
`<install>/Interface/`, direkt neben `AddOns`. Damit liegen sie im
selben Dateisystem und das Verschieben ist ein einfaches `rename`; nur
über eine Dateisystemgrenze hinweg fällt `MoveDir` auf Kopieren und
Löschen zurück. Beide Ordner werden nach Erfolg und nach Fehlern
entfernt. Das Backup ist eine Kopie unter
`<Registry-Verzeichnis>/backups/`, per Default
`~/.local/share/wowtui/backups/<provider>-<ref>-<Zeitstempel>/`
(Sonderzeichen im Namen werden zu `-`).

Regeln beim Ersetzen:

- Enthält das Archiv einen Ordner, den in derselben Installation ein
  anderes verwaltetes Addon besitzt, bricht die Installation ab, bevor
  `AddOns` angefasst wird. Die Meldung nennt Ordner und Besitzer
- Ein Ordner, den kein Addon verwaltet, darf ersetzt werden; wowtui
  meldet das als Warnung mit dem Backup-Pfad
- Ordner, die die alte Version hatte und die neue nicht mehr
  mitbringt, werden entfernt. Sie landen vorher im Backup und kommen
  beim Rollback zurück. Beansprucht ein anderes verwaltetes Addon den
  Ordner noch, bleibt er stehen
- Ein Addon-Ordnername muss ein einfacher Name sein: nicht leer, kein
  Punkt am Anfang, kein `/`, `\` oder NUL. Sonst bricht die Installation
  vor Backup und Verschieben ab

## 7. Sicherheitsmodell

Fünf Stufen, absteigend nach Wichtigkeit. Stufe 1 und 2 sind Pflicht
und plattformunabhängig, der Rest ist Verteidigung in der Tiefe.

1. **Sicherer Go-ZIP-Extractor.** Zip-Slip verhindern, Symlinks und
   Hardlinks ablehnen, maximale Dateianzahl und entpackte Gesamtgröße
   begrenzen, nur reguläre Dateien und Verzeichnisse erlauben. Für
   Generic Git klont wowtui flach in ein Temp-Verzeichnis und zippt den
   Checkout ohne `.git`-Einträge; enthält er einen Symlink oder eine
   andere Nicht-Datei, wird er abgelehnt.
2. **Staging, atomare Installation, Rollback.** Erst validieren, dann
   installieren. Staging und Rollback liegen in `<install>/Interface/`
   (Abschnitt 6). Backups ermöglichen das Rückgängigmachen.
3. **Separater Worker-Prozess** für das Entpacken und Prüfen.
4. **Bubblewrap um den Worker**, nur Linux, optional.
5. **Optional Seccomp-Policy** für den Worker.

Umgesetzt sind Stufe 1 und 2. Stufe 3 bis 5 sind geplant (M3),
`internal/sandbox` ist noch leer.

### Worker und Bubblewrap

Der Hauptprozess lädt herunter und startet den Worker. Der Worker sieht
nur das Archiv (read-only) und ein Staging-Verzeichnis (schreibbar):

```bash
bwrap \
  --unshare-all \
  --die-with-parent \
  --new-session \
  --ro-bind /usr /usr \
  --ro-bind /lib /lib \
  --ro-bind /lib64 /lib64 \
  --ro-bind "$ARCHIVE" /input/addon.zip \
  --bind "$STAGING_DIR" /output \
  --proc /proc \
  --dev /dev \
  --tmpfs /tmp \
  --chdir /output \
  wowtui-worker extract /input/addon.zip /output
```

Kein Zugriff auf `$HOME`, `~/.ssh`, WoW-Verzeichnisse, kein Netzwerk.
Nach erfolgreicher Validierung übernimmt der Hauptprozess die Dateien
aus dem Staging und installiert sie.

Kommunikation über stdin/stdout mit JSON, keine Shell, keine beliebigen
Befehle:

```go
type ExtractRequest struct {
    ArchivePath      string `json:"archive_path"`
    DestinationPath  string `json:"destination_path"`
    MaximumFiles     int    `json:"maximum_files"`
    MaximumTotalSize int64  `json:"maximum_total_size"`
    MaximumFileSize  int64  `json:"maximum_file_size"`
}

type ExtractResult struct {
    Files      []ExtractedFile `json:"files"`
    AddonRoots []string        `json:"addon_roots"`
    TotalSize  int64           `json:"total_size"`
}
```

### Sandbox-Verhalten je Plattform

- Linux + bwrap vorhanden: Sandbox verwenden
- Linux ohne bwrap: Warnung, interner sicherer Extractor
- Windows/macOS: interner Extractor
- Strict Mode über `[security] sandbox = "required"`: Installation
  schlägt fehl, wenn keine Sandbox verfügbar ist

### Grenzen von Bubblewrap

Nur Linux, muss installiert sein, braucht funktionierende
User-Namespaces, Distributionen schränken deren Nutzung unterschiedlich
ein, eine falsche Konfiguration kann Sicherheit nur vortäuschen.
Deshalb bleibt die Sandbox ein optionales Backend hinter dem
`Sandbox`-Interface und nie das Fundament der Anwendung.

## 8. TUI

Bubble Tea nur als Präsentation. Downloads und Dateisystemoperationen
laufen in einer Goroutine, ohne die Oberfläche zu blockieren. Der
Service meldet Fortschritt als Events über einen Channel
(`internal/service/event.go`):

```go
type Event struct {
    Ref     string
    Stage   Stage
    Message string
    Index   int    // Position bei update --all
    Total   int
    Err     error
}

type Stage string

const (
    StageResolve  Stage = "resolve"
    StageDownload Stage = "download"
    StageExtract  Stage = "extract"
    StageBackup   Stage = "backup"
    StageInstall  Stage = "install"
    StageDone     Stage = "done"
    StageCurrent  Stage = "up-to-date"
    StageInfo     Stage = "info"
    StageWarning  Stage = "warning"
    StageError    Stage = "error"
)
```

Die CLI gibt jede Event-Message als Zeile aus. Die TUI zeigt die
laufende Message neben dem Spinner. Warnungen und Fehler einer
Operation sammelt sie in einer Hinweisleiste über der Tastenhilfe,
Fehler zuerst und ohne Duplikate. Die Leiste bleibt stehen, bis
`ctrl+x` sie schließt oder die nächste Operation startet. Sie zeigt
höchstens fünf Zeilen, auf niedrigen Terminals weniger, den Rest als
`+N more`.

Grober Aufbau (Addons-Tab mit offener Hinweisleiste):

```
 wowtui    Addons  Explore  Settings
╭──────────────────────────────────────────────────────────────────────────────────────────╮
│ game: all game versions · install: all installations · status: all · 3 addons ·          │
│ 2 managed · 1 updates                                                                    │
│  Addon       Status       Installed  Latest   Provider  Author     Game                  │
│  Details     not managed  11.0.8                                   retail                │
│  Plater      current      603.0      603.0    github    Tercioo    retail                │
│  WeakAuras   update       5.18.1     5.19.0   github    WeakAuras  retail                │
╰──────────────────────────────────────────────────────────────────────────────────────────╯
 ✗ update failed: Owner/Missing: no stable release found
 ! warning: Owner/Repo replaced unmanaged folders Foo (backup: ...)
 ctrl+x dismiss · ↑/↓ navigate · f search · i game version · I installation · d details ·
 a add · r check · u update · tab explore · q quit
```

Tasten im Addons-Tab: `s` neu scannen, `r` auf Updates prüfen, `u`
gewähltes Addon aktualisieren, `U` alle verwalteten Addons
aktualisieren, `a` Add-Dialog, `f` suchen, `i`/`I` Filter für
Spielversion und Installation, `d`/`enter` Details, `tab` oder
`1`/`2`/`3` Tabs wechseln, `q` beenden. `s` und `U` stehen nicht in
der Tastenhilfe.

Screens: Addons, Explore (Suche in den Provider-Katalogen), Settings,
dazu Details und der Add-Dialog.

## 9. CLI

```
wowtui               # TUI
wowtui scan          # Installationen und Addons neu einlesen
wowtui list          # gescannte Addons mit Version und Interface-Nummern
wowtui installs      # bekannte Installationen
wowtui rename <install-path> <title>
wowtui add github:WeakAuras/WeakAuras2
wowtui add https://github.com/WeakAuras/WeakAuras2
wowtui add github:Owner/Repo --source tag
wowtui add github:Owner/Repo --flavor classic-era
wowtui add github:Owner/Repo --install <install-path>
wowtui add gitlab:Group/Project
wowtui add git:https://example.com/addon.git
wowtui add direct:https://example.com/addon.zip
wowtui add curseforge:<id>
wowtui check         # auf Updates prüfen
wowtui update WeakAuras
wowtui update --all
wowtui update --all --dry-run
wowtui map           # Provider-Kandidaten für nicht verwaltete Addons
wowtui adopt <install-path> <folder> <reference>
wowtui providers     # Status und Reihenfolge
wowtui providers enable|disable <name>
wowtui providers order <name>...
wowtui providers test <name>
```

`add --flavor` hat `retail` als Default und akzeptiert alle zwölf
Flavor-IDs aus Abschnitt 5. Gibt es mehrere Installationen dieses
Flavors, verlangt `add` eine Auswahl mit `--install`.

`update --all` macht nach einem fehlgeschlagenen Addon mit den übrigen
weiter, schließt mit einer Zeile wie
`summary: 3 updated, 5 current, 1 failed` (bei `--dry-run`:
`summary: 3 to update, 5 current, 1 failed (dry run)`) und endet mit
Exit-Status 1, wenn ein Addon fehlgeschlagen ist.

CLI und TUI verwenden denselben `service.Service`. Das hilft bei Skripten,
Tests ohne Terminal, Headless-Nutzung und Debugging.

## 10. Meilensteine

**M1, Kern ohne TUI.** WoW finden, Addons scannen, Registry schreiben,
GitHub-Release abrufen, ZIP sicher validieren, Backup erstellen, atomar
installieren, Rollback testen. Alles als Bibliothek plus Tests.

**M2, CLI vollständig.** Alle Befehle aus Abschnitt 9, Dry-Run,
Konfigurationsdatei, sauberer Fehleroutput.

**M3, Sandbox.** Worker-Prozess, Bubblewrap-Backend, Fallback-Logik,
Strict Mode. Testfälle mit feindlichen Archiven.

**M4, TUI.** Bubble Tea auf dem fertigen Kern. Erst jetzt, damit nicht
Provider, Dateisystem, Nebenläufigkeit und UI-Zustand gleichzeitig
debuggt werden müssen.

**M5, Wago Provider.** Zweiter Provider, API-Key-Konfiguration.

**M6, Ausbau.** SQLite, mehrere Installationen komfortabel verwalten,
Releasehistorie. CurseForge bleibt an den eigenen API-Key des Users
gebunden, ein Key wird nie mitgeliefert.

## 11. Risiken

| Risiko | Auswirkung | Gegenmaßnahme |
|---|---|---|
| GitHub-Repos ohne ZIP-Artefakt am Release | Import schlägt fehl | Klare Fehlermeldung, Verhalten wie bei WowUp dokumentieren |
| GitHub-Rate-Limit (60 Anfragen pro Stunde ohne Token) | `check` und `update --all` scheitern bei vielen Addons | HTTP-Cache mit bedingten Requests (mit Token zählt ein `304` nicht), Fehlermeldung mit Reset-Zeit und Token-Hinweis |
| Unsaubere Versionsformate | Falsche Update-Entscheidungen | Auflösungsreihenfolge aus Abschnitt 4, Hashvergleich als letzte Stufe |
| Flavor-Mismatch | Falsches Release für Classic/PTR | Provider wählen nach dem GameType des Flavors, Test-Clients teilen den Spieltyp ihres Basis-Clients |
| bwrap nicht verfügbar oder User-Namespaces deaktiviert | Sandbox fehlt | Fallback auf internen Extractor, Warnung, Strict Mode |
| CurseForge-ToS | Provider rechtlich/technisch instabil | Nur mit eigenem API-Key des Users, kein mitgelieferter Key, ToS-Einhaltung bleibt Sache von User und Projekt |
| Mehrere Ordner pro Paket | Falsche Zuordnung bei Updates | Paket-Modell statt Ordner-Modell, Registry-Metadaten |

## 12. Teststrategie

- Service-Schicht ohne Terminal testbar (Interface-basiert)
- Fixture-Archive: Zip-Slip-Pfade, Symlinks, Hardlinks, leere Archive,
  Archive über den Größenlimits, verschachtelte Strukturen
- Rollback-Test: Installation künstlich fehlschlagen lassen, prüfen,
  dass der alte Zustand wiederhergestellt wird
- Provider-Tests gegen aufgezeichnete HTTP-Responses, nicht gegen die
  Live-API
- Golden Files für `.toc`-Parsing und Versionsvergleich

## 13. Einschätzung

| Ziel | Schwierigkeit |
|---|---|
| Persönlicher GitHub-Addon-Updater | 4/10 |
| Gute TUI mit Backups und mehreren WoW-Versionen | 6/10 |
| Automatische Erkennung und Zuordnung vorhandener Addons | 7/10 |
| Mehrere Plattformen mit Suche und Migration | 8/10 |
| Vollwertiger WowUp-Ersatz | 9/10 |

Zeitrahmen für den 0.1-Scope: Prototyp in einigen Tagen, brauchbares
persönliches Tool in 2 bis 4 Wochen nebenbei.
