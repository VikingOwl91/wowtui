package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	WoW       WoW       `toml:"wow"`
	Providers Providers `toml:"providers"`
	Index     Index     `toml:"index,omitempty"`
	Security  Security  `toml:"security"`

	fileCredentials credentials
	envCredentials  credentials
}

// Security controls which isolation layers are required for archive extraction.
// Empty fields in a programmatically constructed Config use automatic selection.
type Security struct {
	Sandbox string `toml:"sandbox"`
	Seccomp string `toml:"seccomp"`
}

func (s Security) withDefaults() Security {
	if s.Sandbox == "" {
		s.Sandbox = "auto"
	}
	if s.Seccomp == "" {
		s.Seccomp = "auto"
	}
	return s
}

func (s Security) validate() error {
	if s.Sandbox != "auto" && s.Sandbox != "required" {
		return errors.New("security.sandbox must be auto or required")
	}
	if s.Seccomp != "auto" && s.Seccomp != "required" {
		return errors.New("security.seccomp must be auto or required")
	}
	return nil
}

type credentials struct {
	githubToken      string
	wagoAPIKey       string
	curseForgeAPIKey string
}

type WoW struct {
	Paths []string `toml:"paths"`
}

// Index locates the signed addon index: the JSON file at Path, its detached
// signature at Path + ".sig", and the base64 ed25519 PublicKey that signs it.
type Index struct {
	Path      string `toml:"path"`
	PublicKey string `toml:"public_key"`
}

// Configured reports whether any index setting is present. A partly
// configured index counts, so loading it reports the missing setting.
func (i Index) Configured() bool {
	return strings.TrimSpace(i.Path) != "" || strings.TrimSpace(i.PublicKey) != ""
}

type Providers struct {
	PriorityOrder []string       `toml:"order"`
	GitHub        GitHub         `toml:"github"`
	GitLab        ProviderConfig `toml:"gitlab"`
	Git           ProviderConfig `toml:"git"`
	WowUpHub      ProviderConfig `toml:"wowuphub"`
	Wago          Wago           `toml:"wago"`
	CurseForge    CurseForge     `toml:"curseforge"`
	Direct        ProviderConfig `toml:"direct"`
}

type GitHub struct {
	Enabled *bool  `toml:"enabled"`
	Token   string `toml:"token"`
}

type Wago struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

type CurseForge struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

type ProviderConfig struct {
	Enabled *bool  `toml:"enabled"`
	APIKey  string `toml:"api_key"`
}

var defaultProviderOrder = []string{"wowuphub", "wago", "curseforge", "github", "gitlab", "git", "direct"}
var retiredProviders = map[string]bool{}
var pausedProviders = map[string]bool{"wowuphub": true, "wago": true}
var pausedProviderStates = map[string]string{"wowuphub": "pending-upstream", "wago": "pending-approval"}

func (p Providers) Enabled(name string) bool {
	if retiredProviders[name] || pausedProviders[name] {
		return false
	}
	if name == "curseforge" {
		return p.CurseForge.APIKey != ""
	}
	var configured *bool
	switch name {
	case "github":
		configured = p.GitHub.Enabled
	case "wago":
		configured = p.Wago.Enabled
	case "gitlab":
		configured = p.GitLab.Enabled
	case "git":
		configured = p.Git.Enabled
	case "wowuphub":
		configured = p.WowUpHub.Enabled
	case "direct":
		configured = p.Direct.Enabled
	default:
		return false
	}
	if configured != nil {
		return *configured
	}
	return name == "github" || name == "gitlab" || name == "git" || name == "direct"
}

func (p Providers) Order() []string {
	seen := make(map[string]bool)
	order := make([]string, 0, len(defaultProviderOrder))
	for _, name := range append(p.PriorityOrder, defaultProviderOrder...) {
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	return order
}

func KnownProviders() []string {
	return append([]string(nil), defaultProviderOrder...)
}

func (p *Providers) SetEnabled(name string, enabled bool) error {
	if enabled && retiredProviders[name] {
		return fmt.Errorf("provider %q is retired", name)
	}
	if enabled && pausedProviders[name] {
		return fmt.Errorf("provider %q is paused pending upstream approval", name)
	}
	if name == "curseforge" {
		return fmt.Errorf("provider %q is enabled when its API key is configured", name)
	}
	value := enabled
	switch name {
	case "github":
		p.GitHub.Enabled = &value
	case "gitlab":
		p.GitLab.Enabled = &value
	case "git":
		p.Git.Enabled = &value
	case "wowuphub":
		p.WowUpHub.Enabled = &value
	case "wago":
		p.Wago.Enabled = &value
	case "direct":
		p.Direct.Enabled = &value
	default:
		return fmt.Errorf("unknown provider %q", name)
	}
	return nil
}

func (p Providers) State(name string) string {
	if retiredProviders[name] {
		return "retired"
	}
	if pausedProviders[name] {
		return pausedProviderStates[name]
	}
	if name == "curseforge" {
		if p.Enabled(name) {
			return "enabled"
		}
		return "requires-api-key"
	}
	if name == "github" || name == "direct" || name == "gitlab" || name == "git" {
		if p.Enabled(name) {
			return "enabled"
		}
		return "disabled"
	}
	if p.Enabled(name) {
		return "planned-enabled"
	}
	return "planned"
}

func (p *Providers) SetOrder(order []string) error {
	known := make(map[string]bool)
	for _, name := range defaultProviderOrder {
		known[name] = true
	}
	seen := make(map[string]bool)
	for _, name := range order {
		if !known[name] {
			return fmt.Errorf("unknown provider %q", name)
		}
		if seen[name] {
			return fmt.Errorf("provider %q appears more than once", name)
		}
		seen[name] = true
	}
	p.PriorityOrder = append([]string(nil), order...)
	return nil
}

func Load(path string) (*Config, error) {
	cfg := &Config{Security: Security{}.withDefaults()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := cfg.Security.validate(); err != nil {
		return nil, err
	}
	applyEnv(cfg)
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	out := *cfg
	out.Security = out.Security.withDefaults()
	if err := out.Security.validate(); err != nil {
		return err
	}
	out.Providers.GitHub.Token = persistedCredential(cfg.Providers.GitHub.Token, cfg.envCredentials.githubToken, cfg.fileCredentials.githubToken)
	out.Providers.Wago.APIKey = persistedCredential(cfg.Providers.Wago.APIKey, cfg.envCredentials.wagoAPIKey, cfg.fileCredentials.wagoAPIKey)
	out.Providers.CurseForge.APIKey = persistedCredential(cfg.Providers.CurseForge.APIKey, cfg.envCredentials.curseForgeAPIKey, cfg.fileCredentials.curseForgeAPIKey)
	data, err := toml.Marshal(&out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// persistedCredential returns the value Save writes for a credential. A value
// that still equals the one applied from the environment is replaced by the
// value read from the config file.
func persistedCredential(current, fromEnv, fromFile string) string {
	if fromEnv != "" && current == fromEnv {
		return fromFile
	}
	return current
}

func applyEnv(cfg *Config) {
	cfg.fileCredentials = credentials{
		githubToken:      cfg.Providers.GitHub.Token,
		wagoAPIKey:       cfg.Providers.Wago.APIKey,
		curseForgeAPIKey: cfg.Providers.CurseForge.APIKey,
	}
	if v := os.Getenv("WOWTUI_GITHUB_TOKEN"); v != "" {
		cfg.Providers.GitHub.Token = v
		cfg.envCredentials.githubToken = v
	}
	if v := os.Getenv("WOWTUI_WAGO_API_KEY"); v != "" {
		cfg.Providers.Wago.APIKey = v
		cfg.envCredentials.wagoAPIKey = v
	}
	if v := os.Getenv("WOWTUI_CURSEFORGE_API_KEY"); v != "" {
		cfg.Providers.CurseForge.APIKey = v
		cfg.envCredentials.curseForgeAPIKey = v
	}
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wowtui", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "wowtui", "config.toml")
}
