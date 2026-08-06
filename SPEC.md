# wowtui Spec Sheet

Stand: 2026-08-06

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
- CurseForge, Wago

### Danach

- v0.2: Wago als zweiter Provider (dokumentierte API, API-Key nötig)
- v0.3+: SQLite statt JSON, sobald mehrere Installationen,
  Releasehistorien oder umfangreiche Metadaten dazukommen
- CurseForge erst, wenn offizieller API-Zugang für das Nutzungsszenario
  geklärt ist (ToS, Downloadfreigaben)

## 3. Tech Stack

| Baustein | Wahl | Anmerkung |
|---|---|---|
| Sprache | Go | |
| TUI | Bubble Tea | Model-Update-View |
| TUI-Komponenten | Bubbles | Listen, Spinner, Progress |
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
├── Update Planner         (Versionsabgleich, Flavor-Matching)
└── Package Installer      (transaktionale Installation)
     │
     ├── GitHub Provider
     ├── Direct-URL Provider
     └── (später) Wago Provider
```

Bubble Tea ist reine Präsentationsschicht. Der Kern funktioniert ohne
Terminal, damit CLI, Tests und spätere Headless-Nutzung denselben Code
verwenden.

### Repository-Layout

```
cmd/
├── root.go
├── scan.go
├── update.go
└── tui.go

internal/
├── addon/
│   ├── addon.go
│   ├── scanner.go
│   └── matcher.go
├── provider/
│   ├── provider.go
│   ├── github.go
│   └── direct.go
├── installer/
│   ├── download.go
│   ├── archive.go
│   ├── backup.go
│   └── transaction.go
├── wow/
│   ├── installation.go
│   ├── flavor.go
│   └── discovery.go
├── registry/
│   └── registry.go
├── sandbox/
│   ├── sandbox.go
│   ├── bubblewrap.go
│   └── worker.go
└── tui/
    ├── app.go
    ├── screen_addons.go
    ├── screen_details.go
    └── screen_settings.go
```

### Kern-Interfaces

```go
type Provider interface {
    Search(ctx context.Context, query string) ([]RemoteAddon, error)
    Resolve(ctx context.Context, ref AddonReference) (RemoteAddon, error)
    LatestRelease(ctx context.Context, addon RemoteAddon, flavor GameFlavor) (Release, error)
    Download(ctx context.Context, release Release) (io.ReadCloser, error)
}

type AddonService interface {
    Scan(ctx context.Context) ([]Addon, error)
    CheckUpdates(ctx context.Context) ([]Update, error)
    Update(ctx context.Context, addonID string) error
    UpdateAll(ctx context.Context) error
}

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
api_key = ""        # erforderlich, Provider selbst erst nach v0.1
```

Regeln:

- Keys liegen nur in der Konfigurationsdatei (XDG-Pfad, Dateirechte
  0600) oder in Umgebungsvariablen, nie in der Registry oder im Repo
- Ein Provider ohne gültigen Key wird übersprungen und meldet einen
  klaren Fehler, statt still zu scheitern
- GitHub funktioniert ohne Token, mit Token nur für höhere
  Rate-Limits
- Direct-URL braucht keinen Key

Provider können zusätzlich aktiviert/deaktiviert und priorisiert werden:

```toml
[providers]
order = ["wowuphub", "wago", "curseforge", "github", "gitlab", "direct"]

[providers.wago]
enabled = true
api_key = "..."
```

Die CLI stellt dafür `wowtui providers`,
`wowtui providers enable <name>`,
`wowtui providers disable <name>` und
`wowtui providers order <name>...` bereit. Die TUI zeigt dieselbe
Reihenfolge und den Aktivierungsstatus in den Settings.

GitHub und Direct sind implementiert und standardmäßig aktiv. Wago,
CurseForge, WowUpHub und GitLab sind als Provider-IDs und
Konfigurationsstellen vorbereitet; ihre Adapter werden separat gegen
ihre aktuellen APIs implementiert.

### GitHub ohne Releases (Fallback-Kette)

Viele Repos haben keine Releases oder keine ZIP-Artefakte. Der
GitHub-Provider nutzt deshalb eine Fallback-Kette:

1. Neuestes stabiles Release mit passendem ZIP-Asset für den Flavor
2. Release vorhanden, aber ohne ZIP: Quellarchiv des Release-Tags
3. Kein Release: Quellarchiv des neuesten Tags (semver-Sortierung,
   bei nicht parsebaren Tags API-Reihenfolge)
4. Auch kein Tag: Quellarchiv des Default-Branch, Version ist der
   Commit-SHA

GitHub liefert diese Archive automatisch unter
`github.com/Owner/Repo/archive/refs/tags/<Tag>.zip` beziehungsweise
`.../refs/heads/<Branch>.zip`. Der Installer entpackt den
Wrapper-Ordner (`Repo-Tag/`), den diese Archive mitbringen.

Versionsidentität je Quelle: Release = Release-ID, Tag = Tag-Name,
Branch = Commit-SHA. Die Registry speichert die Quelle im
`source`-Feld. Damit funktionieren auch Übergänge: per Tag
installiert, das Repo bekommt später ein Release, der Check meldet
das als Update.

`add --source release|tag|branch` pinnt die Quelle. Updates folgen
dann dem Pin statt der Kette.

Einschränkung: Tag- und Branch-Archive kennen kein Flavor-Filtering
und keine Asset-Auswahl, das gesamte Repo wird installiert.

### Datenmodell

Ein Ordner ist nicht immer ein Addon. Ein Paket kann mehrere Ordner
enthalten (Details/, Details_Streamer/, Details_TinyThreat/). Deshalb
zwei Ebenen:

```go
type InstalledFolder struct {
    Name      string
    TocFiles  []string
    Version   string
    Interface []int
}

type AddonPackage struct {
    ProviderID string
    Folders    []InstalledFolder
    Version    string
}

type GameFlavor string

const (
    Retail     GameFlavor = "retail"
    Classic    GameFlavor = "classic"
    ClassicEra GameFlavor = "classic-era"
    PTR        GameFlavor = "ptr"
)
```

Zuordnung Ordner → Paket geht nicht zuverlässig über den Ordnernamen.
Die Zuordnung läuft über die Registry-Metadaten aus der Installation.

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
  Konfiguration
- Flavorkennung über die Verzeichnisnamen: `_retail_`, `_classic_`,
  `_classic_era_`, `_beta_`, `_ptr_`
- Addons liegen in `<install>/Interface/AddOns/<Name>/`
- `.toc`-Dateien werden geparst: Version, Interface-Nummer, Titel
- Dieselbe Addon-Quelle kann pro Flavor ein anderes Release brauchen.
  Der Update Planner filtert Releases nach Flavor, nicht nach
  "neueste ZIP".

## 6. Installations-Pipeline

Transaktional, in dieser Reihenfolge:

```
ZIP herunterladen
→ ZIP validieren (SHA-256, Größenlimits)
→ in temporären Ordner entpacken (nie direkt in den WoW-Ordner)
→ Pfade auf Zip-Slip prüfen
→ Inhalt kontrollieren (Struktur, Dateitypen, Limits)
→ alte Ordner sichern (Backup)
→ neue Ordner verschieben (atomar)
→ Registry aktualisieren
→ bei Fehler: Rollback aus dem Backup
```

## 7. Sicherheitsmodell

Fünf Stufen, absteigend nach Wichtigkeit. Stufe 1 und 2 sind Pflicht
und plattformunabhängig, der Rest ist Verteidigung in der Tiefe.

1. **Sicherer Go-ZIP-Extractor.** Zip-Slip verhindern, Symlinks und
   Hardlinks ablehnen, maximale Dateianzahl und entpackte Gesamtgröße
   begrenzen, nur reguläre Dateien und Verzeichnisse erlauben.
2. **Staging, atomare Installation, Rollback.** Erst validieren, dann
   installieren. Backups ermöglichen das Rückgängigmachen.
3. **Separater Worker-Prozess** für das Entpacken und Prüfen.
4. **Bubblewrap um den Worker**, nur Linux, optional.
5. **Optional Seccomp-Policy** für den Worker.

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
laufen als Commands, ohne die Oberfläche zu blockieren. Der Installer
meldet Fortschritt als Messages:

```go
type UpdateProgressMsg struct {
    AddonID    string
    Stage      UpdateStage
    Percentage float64
}

type UpdateStage string

const (
    StageDownloading UpdateStage = "downloading"
    StageValidating  UpdateStage = "validating"
    StageBackingUp   UpdateStage = "backing-up"
    StageInstalling  UpdateStage = "installing"
    StageCompleted   UpdateStage = "completed"
)
```

Grober Aufbau:

```
┌ WoW Addon Manager ──────────────────────────────────────────┐
│ Installation: Retail                     Updates available: 3│
├──────────────────────────────────────────────────────────────┤
│   Addon                 Installed      Available      Status │
│ > WeakAuras             5.18.1         5.19.0         Update │
│   Details               11.0.8.13000   11.0.8.13120   Update │
│   DBM                   11.2.4         11.2.4         Current│
│   Plater                602.1          603.0          Update │
├──────────────────────────────────────────────────────────────┤
│ [u] Update  [a] Update all  [r] Rescan  [d] Details  [q] Quit│
└──────────────────────────────────────────────────────────────┘
```

Screens: Addon-Liste, Details, Einstellungen.

## 9. CLI

```
wowtui               # TUI
wowtui scan          # Installationen und Addons neu einlesen
wowtui list          # Addons mit Version und Status
wowtui add github:WeakAuras/WeakAuras2
wowtui add https://github.com/WeakAuras/WeakAuras2
wowtui add github:Owner/Repo --source tag
wowtui check         # auf Updates prüfen
wowtui update WeakAuras
wowtui update --all
wowtui update --all --dry-run
```

CLI und TUI verwenden denselben AddonService. Das hilft bei Skripten,
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
Releasehistorie. CurseForge nur nach geklärtem API-Zugang.

## 11. Risiken

| Risiko | Auswirkung | Gegenmaßnahme |
|---|---|---|
| GitHub-Repos ohne ZIP-Artefakt am Release | Import schlägt fehl | Klare Fehlermeldung, Verhalten wie bei WowUp dokumentieren |
| Unsaubere Versionsformate | Falsche Update-Entscheidungen | Auflösungsreihenfolge aus Abschnitt 4, Hashvergleich als letzte Stufe |
| Flavor-Mismatch | Falsches Release für Classic/PTR | Update Planner filtert nach GameFlavor |
| bwrap nicht verfügbar oder User-Namespaces deaktiviert | Sandbox fehlt | Fallback auf internen Extractor, Warnung, Strict Mode |
| CurseForge-ToS | Provider rechtlich/technisch instabil | Nicht in 0.1, erst nach Klärung |
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
