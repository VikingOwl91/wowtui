package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// world is a fake GitHub: searches, repositories, files (directory listings
// are derived from the files) and releases, in the shapes the real API
// returns.
type world struct {
	searches map[string][]searchItem
	repos    map[string]worldRepo
	files    map[string]string
	releases map[string]worldRelease
	assets   map[string]string
}

type searchItem struct {
	Repo string
	Path string
}

type worldRepo struct {
	Name        string
	Stars       int
	Description string
	Fork        bool
	Archived    bool
	Branch      string
	PushedAt    string
}

type worldRelease struct {
	Tag    string
	Assets []string
}

func newWorld() *world {
	return &world{
		searches: map[string][]searchItem{},
		repos:    map[string]worldRepo{},
		files:    map[string]string{},
		releases: map[string]worldRelease{},
		assets:   map[string]string{},
	}
}

func (w *world) repo(name string, stars int, description string) {
	w.repos[strings.ToLower(name)] = worldRepo{Name: name, Stars: stars, Description: description, Branch: "main", PushedAt: "2026-11-01T00:00:00Z"}
}

func (w *world) file(repo, path, content string) {
	w.files[strings.ToLower(repo)+"\x00"+path] = content
}

func (w *world) release(repo, tag string, assets map[string]string) {
	rel := worldRelease{Tag: tag}
	for name, content := range assets {
		rel.Assets = append(rel.Assets, name)
		w.assets[strings.ToLower(repo)+"\x00"+name] = content
	}
	sort.Strings(rel.Assets)
	w.releases[strings.ToLower(repo)] = rel
}

func searchQueryFor(kind, id string) string {
	key := map[string]string{"curse": "X-Curse-Project-ID", "wago": "X-Wago-ID", "wowi": "X-WoWI-ID"}[kind]
	return fmt.Sprintf(`"%s: %s" extension:toc`, key, id)
}

func (w *world) hit(kind, id, repo, path string) {
	q := searchQueryFor(kind, id)
	w.searches[q] = append(w.searches[q], searchItem{Repo: repo, Path: path})
}

func (w *world) searched(kind, id string) bool {
	_, ok := w.searches[searchQueryFor(kind, id)]
	return ok
}

func tocText(title, iface string, extra ...string) string {
	lines := []string{"## Interface: " + iface, "## Title: " + title, "## Version: 1.0"}
	lines = append(lines, extra...)
	return strings.Join(lines, "\n") + "\n\nMain.lua\n"
}

func jsonResp(v any) Response {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return Response{Status: 200, Body: data}
}

func notFound() Response {
	return Response{Status: 404, Body: []byte(`{"message":"Not Found"}`)}
}

func (w *world) handle(req Request) Response {
	switch {
	case req.Path == "search/code":
		return w.handleSearch(req.Params["q"])
	case strings.HasPrefix(req.Path, "repos/"):
		parts := strings.SplitN(strings.TrimPrefix(req.Path, "repos/"), "/", 3)
		if len(parts) < 2 {
			return notFound()
		}
		repo := parts[0] + "/" + parts[1]
		rest := ""
		if len(parts) == 3 {
			rest = parts[2]
		}
		return w.handleRepo(repo, rest)
	}
	return notFound()
}

func (w *world) handleSearch(query string) Response {
	items := []map[string]any{}
	for _, item := range w.searches[query] {
		meta := w.repos[strings.ToLower(item.Repo)]
		items = append(items, map[string]any{
			"name": item.Path[strings.LastIndex(item.Path, "/")+1:],
			"path": item.Path,
			"repository": map[string]any{
				"full_name":   item.Repo,
				"fork":        meta.Fork,
				"description": nullable(meta.Description),
				"owner":       map[string]any{"login": strings.SplitN(item.Repo, "/", 2)[0], "type": "User"},
			},
		})
	}
	return jsonResp(map[string]any{"total_count": len(items), "incomplete_results": false, "items": items})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (w *world) handleRepo(repo, rest string) Response {
	meta, ok := w.repos[strings.ToLower(repo)]
	if !ok {
		return notFound()
	}
	switch {
	case rest == "":
		return jsonResp(map[string]any{
			"full_name": meta.Name, "stargazers_count": meta.Stars, "fork": meta.Fork, "archived": meta.Archived,
			"pushed_at": meta.PushedAt, "default_branch": meta.Branch, "description": nullable(meta.Description),
		})
	case rest == "contents" || strings.HasPrefix(rest, "contents/"):
		path := ""
		if rest != "contents" {
			for _, seg := range strings.Split(strings.TrimPrefix(rest, "contents/"), "/") {
				unescaped, err := url.PathUnescape(seg)
				if err != nil {
					return notFound()
				}
				path += unescaped + "/"
			}
			path = strings.TrimSuffix(path, "/")
		}
		return w.contents(repo, path)
	case rest == "releases/latest":
		rel, ok := w.releases[strings.ToLower(repo)]
		if !ok {
			return notFound()
		}
		assets := []map[string]any{}
		for _, name := range rel.Assets {
			assets = append(assets, map[string]any{"id": assetID(repo, name), "name": name})
		}
		return jsonResp(map[string]any{"tag_name": rel.Tag, "published_at": "2026-10-10T17:21:31Z", "assets": assets})
	case strings.HasPrefix(rest, "releases/assets/"):
		id := strings.TrimPrefix(rest, "releases/assets/")
		for _, name := range w.releases[strings.ToLower(repo)].Assets {
			if strconv.Itoa(assetID(repo, name)) == id {
				return Response{Status: 200, Body: []byte(w.assets[strings.ToLower(repo)+"\x00"+name])}
			}
		}
	}
	return notFound()
}

func assetID(repo, name string) int {
	sum := 0
	for _, r := range strings.ToLower(repo) + name {
		sum = (sum*31 + int(r)) % 1000003
	}
	return sum + 1000
}

func (w *world) contents(repo, path string) Response {
	prefix := strings.ToLower(repo) + "\x00"
	if content, ok := w.files[prefix+path]; ok {
		return jsonResp(map[string]any{
			"type": "file", "name": path[strings.LastIndex(path, "/")+1:], "path": path,
			"encoding": "base64", "content": wrapBase64(base64.StdEncoding.EncodeToString([]byte(content))),
		})
	}
	dirPrefix := path
	if dirPrefix != "" {
		dirPrefix += "/"
	}
	seen := map[string]string{}
	for key := range w.files {
		file, ok := strings.CutPrefix(key, prefix)
		if !ok || !strings.HasPrefix(file, dirPrefix) {
			continue
		}
		name, _, nested := strings.Cut(strings.TrimPrefix(file, dirPrefix), "/")
		if nested {
			seen[name] = "dir"
		} else if _, isDir := seen[name]; !isDir {
			seen[name] = "file"
		}
	}
	if len(seen) == 0 {
		return notFound()
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	entries := []map[string]any{}
	for _, name := range names {
		entries = append(entries, map[string]any{"name": name, "path": dirPrefix + name, "type": seen[name], "sha": "0"})
	}
	return jsonResp(entries)
}

func wrapBase64(s string) string {
	var b strings.Builder
	for len(s) > 60 {
		b.WriteString(s[:60] + "\n")
		s = s[60:]
	}
	return b.String() + s + "\n"
}

const testInstallPath = "/home/privateuser/Games/World of Warcraft/_retail_/Interface/AddOns"

type regFolder struct {
	name  string
	hints map[string]string
}

// testRegistry builds a registry with two installations; the second repeats
// a folder with different casing, as a classic install of the same addon does.
func testRegistry(retail []regFolder, classic ...regFolder) *registry.Registry {
	toFolders := func(in []regFolder) []registry.Folder {
		out := []registry.Folder{}
		for _, f := range in {
			out = append(out, registry.Folder{Name: f.name, Version: "1", TocFiles: []string{f.name + ".toc"}, Hints: f.hints})
		}
		return out
	}
	reg := &registry.Registry{Version: 1}
	reg.Installations = append(reg.Installations, registry.Installation{Path: testInstallPath, Flavor: "retail", Folders: toFolders(retail)})
	if len(classic) > 0 {
		reg.Installations = append(reg.Installations, registry.Installation{Path: strings.Replace(testInstallPath, "_retail_", "_classic_", 1), Flavor: "classic", Folders: toFolders(classic)})
	}
	return reg
}
