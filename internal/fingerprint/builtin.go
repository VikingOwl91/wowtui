package fingerprint

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
)

//go:embed data/*.json
var manifests embed.FS

var builtinCatalog = sync.OnceValue(func() map[string][]Release {
	sources := map[string][]string{
		"github:WeakAuras/WeakAuras2": {"data/weakauras-5.21.1.json", "data/weakauras-5.22.1.json"},
	}
	catalog := make(map[string][]Release, len(sources))
	for ref, files := range sources {
		for _, name := range files {
			data, err := manifests.ReadFile(name)
			if err != nil {
				panic(err)
			}
			var release Release
			if err := json.Unmarshal(data, &release); err != nil {
				panic(fmt.Errorf("%s: %w", name, err))
			}
			if err := release.Validate(); err != nil {
				panic(fmt.Errorf("%s: %w", name, err))
			}
			catalog[ref] = append(catalog[ref], release)
		}
	}
	return catalog
})

// Builtin returns a copy of the release manifests shipped with wowtui, keyed
// by canonical provider reference.
func Builtin() map[string][]Release {
	catalog := builtinCatalog()
	result := make(map[string][]Release, len(catalog))
	for ref, releases := range catalog {
		releases = slices.Clone(releases)
		for i := range releases {
			releases[i].Files = slices.Clone(releases[i].Files)
		}
		result[ref] = releases
	}
	return result
}
