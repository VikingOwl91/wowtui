package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityModes(t *testing.T) {
	auto := Security{Sandbox: "auto", Seccomp: "auto"}
	for _, tc := range []struct {
		name string
		text string
		want Security
	}{
		{"missing file", "", auto},
		{"legacy config", "[wow]\npaths=[]", auto},
		{"both automatic", "[security]\nsandbox='auto'\nseccomp='auto'", auto},
		{"sandbox required", "[security]\nsandbox='required'", Security{"required", "auto"}},
		{"seccomp required", "[security]\nseccomp='required'", Security{"auto", "required"}},
		{"both required", "[security]\nsandbox='required'\nseccomp='required'", Security{"required", "required"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if tc.text != "" {
				path = writeConfig(t, tc.text)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Security != tc.want {
				t.Fatalf("loaded security = %+v, want %+v", cfg.Security, tc.want)
			}
			if err := Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			reloaded, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Security != tc.want {
				t.Errorf("saved security = %+v, want %+v", reloaded.Security, tc.want)
			}
		})
	}
}

func TestLoadInvalidSecurityModes(t *testing.T) {
	for _, key := range []string{"sandbox", "seccomp"} {
		for _, value := range []string{"off", "", "AUTO"} {
			path := writeConfig(t, "[security]\n"+key+"='"+value+"'")
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "security."+key) {
				t.Errorf("%s=%q: got %v, want field error", key, value, err)
			}
		}
	}
}

func TestSaveSecurityModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security != (Security{"auto", "auto"}) {
		t.Fatalf("zero value saved as %+v", cfg.Security)
	}
	before := readConfigFile(t, path)
	for _, invalid := range []Security{{Sandbox: "off"}, {Seccomp: "typo"}} {
		cfg.Security = invalid
		if err := Save(path, cfg); err == nil {
			t.Fatalf("saved invalid modes %+v", invalid)
		}
		if after := readConfigFile(t, path); after != before {
			t.Fatal("invalid save changed file")
		}
	}
}
