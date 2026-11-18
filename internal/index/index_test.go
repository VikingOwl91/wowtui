package index_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestFormatBuilderTypesRoundTrip(t *testing.T) {
	file := index.File{
		Format:      index.FormatVersion,
		GeneratedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
		Entries: []index.Entry{{
			ID:        "plain",
			Name:      "Plain",
			Source:    "github:Owner/Plain",
			Folders:   []string{"Plain"},
			GameTypes: []wow.GameType{wow.GameMainline},
		}},
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"ids"`) || strings.Contains(string(data), `"funding"`) {
		t.Errorf("empty optional fields are written: %s", data)
	}

	idx := mustLoad(t, data)
	if got := idx.Entries(); len(got) != 1 || got[0].ID != "plain" || got[0].ReleaseJSON || got[0].Reviewed {
		t.Errorf("entries = %+v", got)
	}
}

func TestLoadFormatVersion(t *testing.T) {
	tests := []struct {
		name    string
		format  any
		omit    bool
		extra   bool
		wantErr string
	}{
		{name: "missing", omit: true, wantErr: "index format is missing"},
		{name: "null", format: nil, wantErr: "index format is missing"},
		{name: "zero", format: 0, wantErr: "unsupported index format 0"},
		{name: "future", format: 3, wantErr: "unsupported index format 3"},
		{name: "future with unknown fields", format: 3, extra: true, wantErr: "unsupported index format 3"},
		{name: "string", format: "1", wantErr: "decode index"},
		{name: "fraction", format: 1.5, wantErr: "decode index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := document(validEntry("a"))
			if tt.omit {
				delete(doc, "format")
			} else {
				doc["format"] = tt.format
			}
			if tt.extra {
				doc["signatures_v2"] = []any{"x"}
			}
			idx, err := signedLoad(t, encode(t, doc))
			wantErrContaining(t, err, tt.wantErr)
			if idx != nil {
				t.Fatal("got an index")
			}
		})
	}
}

func TestLoadRejectsNonObjectDocument(t *testing.T) {
	for _, data := range []string{"", "null", "[]", `"index"`, "1"} {
		if _, err := signedLoad(t, []byte(data)); err == nil {
			t.Errorf("Load(%q) succeeded", data)
		}
	}
}

func TestLoadRejectsTrailingData(t *testing.T) {
	data := append(encode(t, document(validEntry("a"))), []byte(` {"format":1}`)...)
	_, err := signedLoad(t, data)
	wantErrContaining(t, err, "trailing data")
}

func TestLoadAcceptsTrailingWhitespace(t *testing.T) {
	data := append(encode(t, document(validEntry("a"))), []byte("\n")...)
	mustLoad(t, data)
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(doc map[string]any, entry map[string]any)
		field  string
	}{
		{"top level", func(doc, _ map[string]any) { doc["mirror"] = "https://example.com/x.zip" }, `unknown field "mirror"`},
		{"entry", func(_, entry map[string]any) { entry["download_url"] = "https://example.com/x.zip" }, `unknown field "download_url"`},
		{"ids", func(_, entry map[string]any) { entry["ids"] = map[string]any{"tukui": "1"} }, `unknown field "tukui"`},
		{"funding", func(_, entry map[string]any) {
			entry["funding"] = []any{map[string]any{"platform": "github", "handle": "a", "url": "https://x"}}
		}, `unknown field "url"`},
		{"field name case", func(_, entry map[string]any) { entry["Folders"] = entry["folders"]; delete(entry, "folders") }, `unknown field "Folders"`},
		{"top level field name case", func(doc, _ map[string]any) { doc["Entries"] = doc["entries"]; delete(doc, "entries") }, `unknown field "Entries"`},
		{"unicode case folding", func(_, entry map[string]any) { entry["ſource"] = entry["source"]; delete(entry, "source") }, `unknown field "ſource"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validEntry("a")
			doc := document(entry)
			tt.mutate(doc, entry)
			_, err := signedLoad(t, encode(t, doc))
			wantErrContaining(t, err, tt.field)
		})
	}
}

func TestLoadRejectsDuplicateKeys(t *testing.T) {
	entry := `{"id":"a","name":"A","source":"github:Owner/A","folders":["A"],"game_types":["mainline"],"reviewed":false,"reviewed":true}`
	tests := []struct {
		name string
		data string
		key  string
	}{
		{"entry", `{"format":1,"generated_at":"2026-11-18T10:00:00Z","entries":[` + entry + `]}`, `duplicate field "reviewed"`},
		{"top level", `{"format":1,"generated_at":"2026-11-18T10:00:00Z","entries":[],"entries":[]}`, `duplicate field "entries"`},
		{"ids", `{"format":1,"generated_at":"2026-11-18T10:00:00Z","entries":[{"id":"a","name":"A","source":"github:Owner/A","folders":["A"],"game_types":["mainline"],"ids":{"wago":"x","wago":"y"}}]}`, `duplicate field "wago"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := signedLoad(t, []byte(tt.data))
			wantErrContaining(t, err, tt.key)
		})
	}
}

func TestLoadGeneratedAt(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		omit    bool
		wantErr string
	}{
		{name: "missing", omit: true, wantErr: "generated_at is missing"},
		{name: "date only", value: "2026-11-18", wantErr: "generated_at"},
		{name: "no zone", value: "2026-11-18T10:00:00", wantErr: "generated_at"},
		{name: "number", value: 1763460000, wantErr: "generated_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := document(validEntry("a"))
			if tt.omit {
				delete(doc, "generated_at")
			} else {
				doc["generated_at"] = tt.value
			}
			_, err := signedLoad(t, encode(t, doc))
			wantErrContaining(t, err, tt.wantErr)
		})
	}

	doc := document(validEntry("a"))
	doc["generated_at"] = "2026-11-18T11:00:00.5+01:00"
	idx := mustLoad(t, encode(t, doc))
	if want := time.Date(2026, 11, 18, 10, 0, 0, 500_000_000, time.UTC); !idx.GeneratedAt().Equal(want) {
		t.Errorf("GeneratedAt = %v, want %v", idx.GeneratedAt(), want)
	}
}

func TestLoadEntriesList(t *testing.T) {
	doc := document()
	delete(doc, "entries")
	_, err := signedLoad(t, encode(t, doc))
	wantErrContaining(t, err, "entries is missing")

	doc["entries"] = nil
	_, err = signedLoad(t, encode(t, doc))
	wantErrContaining(t, err, "entries is missing")

	idx := mustLoad(t, encode(t, document()))
	if got := idx.Entries(); len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

func TestLoadValidatesEntries(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(entry map[string]any)
		wantErr string
	}{
		{"missing id", func(e map[string]any) { delete(e, "id") }, "is not a lowercase slug"},
		{"uppercase id", func(e map[string]any) { e["id"] = "MyAddon" }, `id "MyAddon" is not a lowercase slug`},
		{"id with leading dash", func(e map[string]any) { e["id"] = "-addon" }, "is not a lowercase slug"},
		{"id with leading dot", func(e map[string]any) { e["id"] = ".addon" }, "is not a lowercase slug"},
		{"id with slash", func(e map[string]any) { e["id"] = "owner/addon" }, "is not a lowercase slug"},
		{"id with space", func(e map[string]any) { e["id"] = "my addon" }, "is not a lowercase slug"},
		{"id with non-ascii", func(e map[string]any) { e["id"] = "addön" }, "is not a lowercase slug"},

		{"missing name", func(e map[string]any) { delete(e, "name") }, "name is empty"},
		{"blank name", func(e map[string]any) { e["name"] = " \t" }, "name is empty"},
		{"name with escape sequence", func(e map[string]any) { e["name"] = "Addon\x1b[2J" }, "name contains a control character"},
		{"name with newline", func(e map[string]any) { e["name"] = "Addon\nFake" }, "name contains a control character"},

		{"missing source", func(e map[string]any) { delete(e, "source") }, "source"},
		{"unparsable source", func(e map[string]any) { e["source"] = "github:not a repo" }, "invalid github reference"},
		{"direct url", func(e map[string]any) { e["source"] = "https://example.com/addon.zip" }, `want github or gitlab`},
		{"direct prefix", func(e map[string]any) { e["source"] = "direct:https://example.com/addon.zip" }, `want github or gitlab`},
		{"git url", func(e map[string]any) { e["source"] = "git:https://example.com/addon.git" }, `want github or gitlab`},
		{"curseforge", func(e map[string]any) { e["source"] = "curseforge:12345" }, `want github or gitlab`},
		{"wowup", func(e map[string]any) { e["source"] = "wowup:12345" }, `want github or gitlab`},

		{"missing folders", func(e map[string]any) { delete(e, "folders") }, "folders is empty"},
		{"empty folders", func(e map[string]any) { e["folders"] = []any{} }, "folders is empty"},
		{"empty folder", func(e map[string]any) { e["folders"] = []any{""} }, `folder "" is not a safe folder name`},
		{"dot dot folder", func(e map[string]any) { e["folders"] = []any{".."} }, `folder ".." is not a safe folder name`},
		{"hidden folder", func(e map[string]any) { e["folders"] = []any{".git"} }, "is not a safe folder name"},
		{"nested folder", func(e map[string]any) { e["folders"] = []any{"Addon/Sub"} }, "is not a safe folder name"},
		{"backslash folder", func(e map[string]any) { e["folders"] = []any{`..\Addon`} }, "is not a safe folder name"},
		{"nul folder", func(e map[string]any) { e["folders"] = []any{"Addon\x00"} }, "is not a safe folder name"},
		{"duplicate folder ignoring case", func(e map[string]any) { e["folders"] = []any{"Addon", "ADDON"} }, `duplicate folder "ADDON"`},

		{"missing game types", func(e map[string]any) { delete(e, "game_types") }, "game_types is empty"},
		{"empty game types", func(e map[string]any) { e["game_types"] = []any{} }, "game_types is empty"},
		{"flavor instead of game type", func(e map[string]any) { e["game_types"] = []any{"retail"} }, `unknown game type "retail"`},
		{"game type case", func(e map[string]any) { e["game_types"] = []any{"Mainline"} }, `unknown game type "Mainline"`},
		{"duplicate game type", func(e map[string]any) { e["game_types"] = []any{"vanilla", "vanilla"} }, `duplicate game type "vanilla"`},

		{"wago id with space", func(e map[string]any) { e["ids"] = map[string]any{"wago": "ab cd"} }, "ids.wago"},
		{"wowinterface id with control", func(e map[string]any) { e["ids"] = map[string]any{"wowinterface": "12\x1b"} }, "ids.wowinterface"},
		{"numeric id value", func(e map[string]any) { e["ids"] = map[string]any{"curseforge": 12345} }, "decode index"},

		{"unknown funding platform", func(e map[string]any) {
			e["funding"] = []any{map[string]any{"platform": "paypal", "handle": "me"}}
		}, `unknown funding platform "paypal"`},
		{"funding handle with path", func(e map[string]any) {
			e["funding"] = []any{map[string]any{"platform": "patreon", "handle": "me/../../evil"}}
		}, `invalid patreon handle`},
		{"funding custom http", func(e map[string]any) {
			e["funding"] = []any{map[string]any{"platform": "custom", "handle": "http://example.com/donate"}}
		}, "is not an https URL"},
		{"duplicate funding", func(e map[string]any) {
			link := map[string]any{"platform": "ko_fi", "handle": "me"}
			e["funding"] = []any{link, link}
		}, `duplicate funding ko_fi "me"`},

		{"release_json not bool", func(e map[string]any) { e["release_json"] = "yes" }, "decode index"},
		{"reviewed not bool", func(e map[string]any) { e["reviewed"] = 1 }, "decode index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validEntry("a")
			tt.mutate(entry)
			idx, err := signedLoad(t, encode(t, document(entry)))
			wantErrContaining(t, err, tt.wantErr)
			if idx != nil {
				t.Fatal("got an index")
			}
		})
	}
}

func TestLoadAcceptsValidEntryVariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(entry map[string]any)
	}{
		{"github prefix", func(e map[string]any) { e["source"] = "github:Owner/Repo" }},
		{"github url", func(e map[string]any) { e["source"] = "https://github.com/Owner/Repo" }},
		{"bare owner repo", func(e map[string]any) { e["source"] = "Owner/Repo" }},
		{"gitlab subgroup", func(e map[string]any) { e["source"] = "gitlab:group/sub/project" }},
		{"gitlab url", func(e map[string]any) { e["source"] = "https://gitlab.com/group/project" }},
		{"slug punctuation", func(e map[string]any) { e["id"] = "0my.addon_x-y" }},
		{"folder with spaces and dots", func(e map[string]any) { e["folders"] = []any{"My Addon", "Addon..Core"} }},
		{"every game type", func(e map[string]any) {
			e["game_types"] = []any{"mainline", "vanilla", "tbc", "wrath", "titan", "cata", "mists", "forever"}
		}},
		{"unicode name", func(e map[string]any) { e["name"] = "Ränge & Söhne" }},
		{"all ids", func(e map[string]any) {
			e["ids"] = map[string]any{"curseforge": "1", "wago": "aB3_-x", "wowinterface": "2"}
		}},
		{"empty ids object", func(e map[string]any) { e["ids"] = map[string]any{} }},
		{"no optional fields", func(e map[string]any) {
			delete(e, "release_json")
			delete(e, "reviewed")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := validEntry("a")
			tt.mutate(entry)
			mustLoad(t, encode(t, document(entry)))
		})
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	_, err := signedLoad(t, encode(t, document(validEntry("a"), validEntry("b"), validEntry("a"))))
	wantErrContaining(t, err, `entries[2] (id "a"): duplicate id "a", first used by entries[0]`)
}

func TestLoadRejectsSharedProviderIDs(t *testing.T) {
	a, b := validEntry("a"), validEntry("b")
	a["ids"] = map[string]any{"curseforge": "100", "wago": "abc"}
	b["ids"] = map[string]any{"curseforge": "101", "wago": "abc"}
	_, err := signedLoad(t, encode(t, document(a, b)))
	wantErrContaining(t, err, `entries[1] (id "b"): ids.wago "abc" is already used by entries[0]`)

	b["ids"] = map[string]any{"curseforge": "101", "wowinterface": "abc"}
	mustLoad(t, encode(t, document(a, b)))
}

func TestLoadAllowsFolderSharedAcrossEntries(t *testing.T) {
	a, b := validEntry("a"), validEntry("b")
	a["folders"] = []any{"Shared", "OnlyA"}
	b["folders"] = []any{"shared"}
	mustLoad(t, encode(t, document(a, b)))
}

func TestOneInvalidEntryRejectsWholeIndex(t *testing.T) {
	bad := validEntry("bad")
	bad["source"] = "direct:https://example.com/x.zip"
	idx, err := signedLoad(t, encode(t, document(validEntry("good"), bad, validEntry("also-good"))))
	wantErrContaining(t, err, `entries[1] (id "bad")`)
	if idx != nil {
		t.Fatal("got an index with an invalid entry")
	}
}

func TestLoadReportsEveryInvalidEntry(t *testing.T) {
	first, second := validEntry("first"), validEntry("second")
	first["folders"] = []any{"../x"}
	second["game_types"] = []any{"retail"}
	_, err := signedLoad(t, encode(t, document(first, validEntry("ok"), second)))
	wantErrContaining(t, err, `entries[0] (id "first")`)
	wantErrContaining(t, err, `entries[2] (id "second")`)
	if errors.Is(err, index.ErrInvalidSignature) {
		t.Fatal("validation error reported as signature error")
	}
}
