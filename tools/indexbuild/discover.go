package main

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

type DiscoverConfig struct {
	// MirrorOwners are GitHub accounts known to host copies of other
	// people's addons; their repositories never count as originals.
	MirrorOwners []string
	Logf         func(format string, args ...any)
}

const (
	maxTocFetch     = 12
	maxFundingBytes = 16 << 10
	maxReviewPaths  = 3
	maxReviewCands  = 10
)

var (
	genericRepoName = regexp.MustCompile(`(?i)(add-?ons?|(^|[-_])ui($|[-_])|interface|wtf|backup|mods|config|dotfiles|retail|classic|repo)`)
	copyWords       = regexp.MustCompile(`(?i)(\bmirror\b|\bfork of\b|\bbackup\b|\bunofficial\b|\bback-?port\b|\bcopy of\b)`)
	packPath        = regexp.MustCompile(`(?i)^(.*/)?(interface/)?addons/`)
)

// pathHit is a code search hit whose TOC name matches an installed folder.
type pathHit struct {
	Path   string
	Folder string
	Rank   int
}

// cand is a repository that has at least one name-matching hit for an ID.
type cand struct {
	Repo      string
	Fork      bool
	Desc      string
	Hits      []pathHit
	BestRank  int
	MinDepth  int
	Packish   bool
	Folders   map[string]string // lower-case folder to name
	Collect   bool
	NameSim   bool
	Website   bool
	Generic   bool
	ShapeOK   bool
	PrePlaus  bool
	Meta      *repoMeta
	CopyFlag  string
	Plausible bool
	Why       string
}

func (g *cand) key() string { return strings.ToLower(g.Repo) }

func (g *cand) folderNames() []string {
	names := make([]string, 0, len(g.Folders))
	for _, name := range g.Folders {
		names = append(names, name)
	}
	slices.SortFunc(names, compareFold)
	return names
}

func compareFold(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// idRun is the lookup of one provider ID.
type idRun struct {
	Kind     string
	ID       string
	Folders  []string
	Query    string
	Skipped  bool
	Err      error
	Total    int
	Returned int
	Cands    []*cand
	Class    string
	Chosen   *cand
	Basis    string
	Plaus    []*cand
}

func (r *idRun) source() string { return r.Kind + ":" + r.ID }

type hintConfirm struct {
	Repo   string
	Folder string
	Tocs   []string
}

type discoverer struct {
	ctx    context.Context
	c      *Client
	a      *api
	cfg    DiscoverConfig
	mirror map[string]bool

	seeds       []*seed
	registry    []string
	hints       map[string]*hintConfirm
	runs        []*idRun
	repoFolders map[string]map[string]bool
	metas       map[string]*repoMeta
	metaDone    map[string]bool
	review      []ReviewItem
}

func (d *discoverer) logf(format string, args ...any) {
	if d.cfg.Logf != nil {
		d.cfg.Logf(format, args...)
	}
}

// Discover looks up the GitHub source of every addon in reg and returns the
// chosen packages and everything that needs a human look. All GitHub access
// goes through c, so a rerun with the same cache makes no network calls.
func Discover(ctx context.Context, c *Client, reg *registry.Registry, cfg DiscoverConfig) (*Candidates, *Review, error) {
	d := &discoverer{
		ctx: ctx, c: c, a: &api{c: c}, cfg: cfg,
		mirror:      map[string]bool{},
		hints:       map[string]*hintConfirm{},
		repoFolders: map[string]map[string]bool{},
		metas:       map[string]*repoMeta{},
		metaDone:    map[string]bool{},
	}
	for _, owner := range cfg.MirrorOwners {
		d.mirror[strings.ToLower(owner)] = true
	}
	d.seeds, d.registry = loadSeeds(reg)

	if err := d.confirmHints(); err != nil {
		return nil, nil, err
	}
	if err := d.buildRuns(); err != nil {
		return nil, nil, err
	}
	d.computeRepoFolders()
	for _, run := range d.runs {
		if err := d.classify(run); err != nil {
			return nil, nil, err
		}
	}
	d.monorepoPass()
	return d.assemble()
}

func (d *discoverer) seedFor(folder string) *seed {
	for _, s := range d.seeds {
		if strings.EqualFold(s.Folder, folder) {
			return s
		}
	}
	return nil
}

// confirmHints checks every GitHub URL hint against the repository's own
// TOC files. Code search quota is not used.
func (d *discoverer) confirmHints() error {
	for _, s := range d.seeds {
		var confirmed []*hintConfirm
		for _, repo := range s.Repos {
			hc, reason, err := d.confirmHint(s, repo)
			if err != nil {
				return err
			}
			if hc == nil {
				d.review = append(d.review, ReviewItem{
					Reason: reasonHintUnconfirmed, Folders: []string{s.Folder}, Source: "github:" + repo,
					Detail: "the GitHub URL in the folder's TOC was not confirmed: " + reason,
				})
				continue
			}
			if !slices.ContainsFunc(confirmed, func(o *hintConfirm) bool { return strings.EqualFold(o.Repo, hc.Repo) }) {
				confirmed = append(confirmed, hc)
			}
		}
		switch len(confirmed) {
		case 0:
		case 1:
			d.hints[strings.ToLower(s.Folder)] = confirmed[0]
		default:
			var repos []string
			for _, hc := range confirmed {
				repos = append(repos, hc.Repo)
			}
			d.review = append(d.review, ReviewItem{
				Reason: reasonConflict, Folders: []string{s.Folder},
				Detail: "several GitHub URL hints were confirmed, none is used: " + strings.Join(repos, ", "),
			})
		}
	}
	return nil
}

func (d *discoverer) confirmHint(s *seed, repo string) (hc *hintConfirm, reason string, err error) {
	tocs, found, err := d.locateTocs(repo, s.Folder)
	if err != nil {
		if isStatusError(err) {
			return nil, err.Error(), nil
		}
		return nil, "", err
	}
	if !found {
		return nil, "repository not found", nil
	}
	if len(tocs) == 0 {
		return nil, "no TOC named after the folder at the repository root or in a folder of that name", nil
	}
	data, ok, err := d.a.readFile(d.ctx, repo, preferredToc(tocs, s.Folder))
	if err != nil {
		if isStatusError(err) {
			return nil, err.Error(), nil
		}
		return nil, "", err
	}
	if ok {
		fields := parseTOC(data)
		for _, hint := range idHints {
			value := fields[strings.ToLower(hint.queryKey)]
			if value != "" && len(s.IDs[hint.kind]) > 0 && !slices.Contains(s.IDs[hint.kind], value) {
				return nil, fmt.Sprintf("the TOC states %s %s, the installed folder has %s", hint.queryKey, value, strings.Join(s.IDs[hint.kind], ", ")), nil
			}
		}
	}
	return &hintConfirm{Repo: repo, Folder: s.Folder, Tocs: tocs}, "", nil
}

// locateTocs finds the TOC files of folder in repo: at the root, or in a
// root directory named like the folder.
func (d *discoverer) locateTocs(repo, folder string) (tocs []string, found bool, err error) {
	root, found, err := d.a.listDir(d.ctx, repo, "")
	if err != nil || !found {
		return nil, found, err
	}
	names := map[string]string{strings.ToLower(folder): folder}
	for _, entry := range root {
		if entry.Type == "file" {
			if _, ok := tocMatchesFolder(entry.Name, names); ok {
				tocs = append(tocs, entry.Path)
			}
		}
	}
	if len(tocs) == 0 {
		for _, entry := range root {
			if entry.Type != "dir" || !strings.EqualFold(entry.Name, folder) {
				continue
			}
			inner, ok, err := d.a.listDir(d.ctx, repo, entry.Path)
			if err != nil {
				return nil, true, err
			}
			if !ok {
				continue
			}
			for _, in := range inner {
				if in.Type == "file" {
					if _, ok := tocMatchesFolder(in.Name, names); ok {
						tocs = append(tocs, in.Path)
					}
				}
			}
		}
	}
	slices.Sort(tocs)
	return tocs, true, nil
}

// preferredToc picks the TOC without a client suffix when there is one.
func preferredToc(tocs []string, folder string) string {
	for _, p := range tocs {
		if strings.EqualFold(strings.TrimSuffix(path.Base(p), path.Ext(p)), folder) {
			return p
		}
	}
	return tocs[0]
}

// buildRuns searches every ID that still has a folder without a confirmed
// URL hint.
func (d *discoverer) buildRuns() error {
	byID := map[[2]string]*idRun{}
	var order [][2]string
	for _, s := range d.seeds {
		for _, hint := range idHints {
			for _, id := range s.IDs[hint.kind] {
				key := [2]string{hint.kind, id}
				run := byID[key]
				if run == nil {
					run = &idRun{Kind: hint.kind, ID: id, Query: fmt.Sprintf(`"%s: %s" extension:toc`, hint.queryKey, id)}
					byID[key] = run
					order = append(order, key)
				}
				run.Folders = append(run.Folders, s.Folder)
			}
		}
	}
	slices.SortFunc(order, func(a, b [2]string) int {
		if c := strings.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return strings.Compare(a[1], b[1])
	})
	for _, key := range order {
		run := byID[key]
		slices.SortFunc(run.Folders, compareFold)
		d.runs = append(d.runs, run)
		run.Skipped = true
		for _, folder := range run.Folders {
			if d.hints[strings.ToLower(folder)] == nil {
				run.Skipped = false
			}
		}
		if run.Skipped {
			continue
		}
		d.logf("searching %s", run.source())
		res, err := d.a.searchCode(d.ctx, run.Query)
		if err != nil {
			if isStatusError(err) {
				run.Err = err
				continue
			}
			return err
		}
		run.Total, run.Returned = res.Total, len(res.Hits)
		d.collectCands(run, res)
	}
	return nil
}

func (d *discoverer) collectCands(run *idRun, res searchResult) {
	folders := map[string]string{}
	for _, f := range run.Folders {
		folders[strings.ToLower(f)] = f
	}
	byRepo := map[string]*cand{}
	for i, hit := range res.Hits {
		folder, ok := tocMatchesFolder(hit.Path, folders)
		if !ok {
			continue
		}
		g := byRepo[strings.ToLower(hit.Repo)]
		if g == nil {
			g = &cand{Repo: hit.Repo, Fork: hit.Fork, Desc: hit.Description, BestRank: i + 1, MinDepth: 99, Packish: true, Folders: map[string]string{}}
			byRepo[strings.ToLower(hit.Repo)] = g
			run.Cands = append(run.Cands, g)
		}
		g.Hits = append(g.Hits, pathHit{Path: hit.Path, Folder: folder, Rank: i + 1})
		g.Folders[strings.ToLower(folder)] = folder
		g.MinDepth = min(g.MinDepth, strings.Count(hit.Path, "/"))
		g.Packish = g.Packish && (packPath.MatchString(hit.Path) || strings.HasPrefix(hit.Path, "_"))
	}
}

func (d *discoverer) computeRepoFolders() {
	for _, run := range d.runs {
		for _, g := range run.Cands {
			set := d.repoFolders[g.key()]
			if set == nil {
				set = map[string]bool{}
				d.repoFolders[g.key()] = set
			}
			for lower := range g.Folders {
				set[lower] = true
			}
		}
	}
}

func repoNamePart(repo string) string {
	_, name, _ := strings.Cut(repo, "/")
	return name
}

// isCollection reports repositories that hold unrelated addons: three or
// more matched folders without a shared name prefix tied to the repo name.
func (d *discoverer) isCollection(repo string) bool {
	set := d.repoFolders[strings.ToLower(repo)]
	if len(set) < 3 {
		return false
	}
	var norms []string
	for lower := range set {
		norms = append(norms, normName(lower))
	}
	pref := norms[0]
	for _, n := range norms[1:] {
		for !strings.HasPrefix(n, pref) {
			pref = pref[:len(pref)-1]
		}
	}
	rn := strings.TrimPrefix(normName(repoNamePart(repo)), "wow")
	if len(pref) >= 5 && (strings.HasPrefix(rn, pref) || strings.HasPrefix(pref, rn[:min(5, len(rn))])) {
		return false
	}
	return true
}

// meta returns repository metadata, nil when the repository is gone or
// blocked.
func (d *discoverer) meta(repo string) (*repoMeta, error) {
	key := strings.ToLower(repo)
	if d.metaDone[key] {
		return d.metas[key], nil
	}
	meta, err := d.a.repoInfo(d.ctx, repo)
	if err != nil && !isStatusError(err) {
		return nil, err
	}
	d.metaDone[key] = true
	d.metas[key] = meta
	return meta, nil
}

func (d *discoverer) hintRepoMatches(run *idRun, repo string) bool {
	for _, folder := range run.Folders {
		if hc := d.hints[strings.ToLower(folder)]; hc != nil && strings.EqualFold(hc.Repo, repo) {
			return true
		}
	}
	return false
}

func (d *discoverer) copyFlag(g *cand) string {
	owner, name, _ := strings.Cut(g.Repo, "/")
	if d.mirror[strings.ToLower(owner)] {
		return "owner is a configured mirror account"
	}
	desc := g.Desc
	if g.Meta != nil {
		desc = g.Meta.Description
		if g.Meta.Parent != "" {
			return "GitHub fork of " + g.Meta.Parent
		}
	}
	if copyWords.MatchString(name) || copyWords.MatchString(desc) {
		return "name or description says mirror, fork, backup or port"
	}
	return ""
}

// classify applies the filters and tie-breaks to one ID's hits.
func (d *discoverer) classify(run *idRun) error {
	switch {
	case run.Skipped:
		run.Class = "skipped"
		return nil
	case run.Err != nil:
		run.Class = "api_error"
		return nil
	case run.Returned == 0:
		run.Class = "no_hits"
		return nil
	case len(run.Cands) == 0:
		run.Class = "wrong_names"
		return nil
	}
	for _, g := range run.Cands {
		first := g.Hits[0].Folder
		g.Collect = d.isCollection(g.Repo)
		g.NameSim = nameSimilar(first, g.Repo)
		g.Website = d.hintRepoMatches(run, g.Repo)
		g.Generic = genericRepoName.MatchString(repoNamePart(g.Repo))
		g.ShapeOK = g.MinDepth == 0 || (g.MinDepth == 1 && (g.NameSim || (!g.Generic && len(d.repoFolders[g.key()]) == 1)))
		g.PrePlaus = !g.Fork && !g.Collect && (g.Website || g.ShapeOK)
		switch {
		case g.Fork:
			g.Why = "GitHub fork"
		case g.Collect:
			g.Why = fmt.Sprintf("collection repo: %d installed folders without a shared name", len(d.repoFolders[g.key()]))
		case !g.PrePlaus:
			g.Why = fmt.Sprintf("TOC at %s is nested or sits in a multi-addon repository", g.Hits[0].Path)
		}
		if !g.PrePlaus {
			continue
		}
		meta, err := d.meta(g.Repo)
		if err != nil {
			return err
		}
		if meta == nil {
			g.Why = "repository metadata is unavailable"
			continue
		}
		g.Meta = meta
		if g.CopyFlag = d.copyFlag(g); g.CopyFlag != "" && !g.Website {
			g.Why = "copy or mirror (" + g.CopyFlag + ")"
			continue
		}
		g.Plausible = true
		run.Plaus = append(run.Plaus, g)
	}
	if len(run.Plaus) == 0 {
		run.Class = "filtered"
		return nil
	}
	var site []*cand
	for _, g := range run.Plaus {
		if g.Website {
			site = append(site, g)
		}
	}
	switch {
	case len(run.Plaus) == 1:
		run.Class, run.Chosen, run.Basis = "unique", run.Plaus[0], "single plausible repository"
	case len(site) == 1:
		run.Class, run.Chosen, run.Basis = "unique", site[0], fmt.Sprintf("%d plausible repositories; the GitHub URL hint points to this one", len(run.Plaus))
	default:
		ranked := slices.Clone(run.Plaus)
		slices.SortStableFunc(ranked, func(a, b *cand) int { return b.Meta.Stars - a.Meta.Stars })
		top, second := ranked[0], ranked[1]
		if top.Meta.Stars >= 10 && top.Meta.Stars >= 3*max(1, second.Meta.Stars) && top.NameSim && !top.Meta.Archived {
			run.Class, run.Chosen = "dominant", top
			run.Basis = fmt.Sprintf("%d plausible repositories; stars %d vs next %d (at least 3x and 10), name similar, not archived", len(run.Plaus), top.Meta.Stars, second.Meta.Stars)
		} else {
			run.Class = "ambiguous"
			run.Plaus = ranked
		}
	}
	return nil
}

// monorepoPass moves an ID to a repository that is already chosen for
// another folder when the ID's TOC sits inside it (outside AddOns/ packs).
func (d *discoverer) monorepoPass() {
	anchors := map[string]map[string]bool{}
	for _, run := range d.runs {
		if run.Chosen == nil {
			continue
		}
		set := anchors[run.Chosen.key()]
		if set == nil {
			set = map[string]bool{}
			anchors[run.Chosen.key()] = set
		}
		for _, f := range run.Folders {
			set[strings.ToLower(f)] = true
		}
	}
	for _, run := range d.runs {
		if len(run.Cands) == 0 {
			continue
		}
		mine := map[string]bool{}
		for _, f := range run.Folders {
			mine[strings.ToLower(f)] = true
		}
		var mono []*cand
		for _, g := range run.Cands {
			set := anchors[g.key()]
			if set != nil && !subset(set, mine) && !g.Packish && !g.Fork && !g.Collect {
				mono = append(mono, g)
			}
		}
		if len(mono) == 0 || (run.Chosen != nil && slices.Contains(mono, run.Chosen)) {
			continue
		}
		g := mono[0]
		var others []string
		for lower := range anchors[g.key()] {
			if !mine[lower] {
				others = append(others, lower)
			}
		}
		slices.Sort(others)
		run.Class, run.Chosen = "monorepo", g
		run.Basis = fmt.Sprintf("monorepo: TOC with this ID at %s inside %s, which is the chosen repository for %s", g.Hits[0].Path, g.Repo, strings.Join(others, ", "))
	}
}

func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

type assignment struct {
	Repo     string
	Evidence Evidence
	FromHint bool
}

type pkgBuild struct {
	key     string
	repo    string
	folders map[string]string // seed folders, lower to name
	evid    []Evidence
	hits    map[string][]string // lower folder to TOC paths
	hintTOC map[string][]string
}

// assemble merges the per-ID results into one repository per folder, builds
// the packages and collects the review list.
func (d *discoverer) assemble() (*Candidates, *Review, error) {
	assigned := map[string][]assignment{}
	for _, run := range d.runs {
		if run.Chosen == nil {
			continue
		}
		for lower, folder := range run.Chosen.Folders {
			assigned[lower] = append(assigned[lower], assignment{
				Repo: run.Chosen.Repo,
				Evidence: Evidence{
					Kind: "code-search", Source: run.source(), Folder: folder, Query: run.Query,
					Basis: run.Basis, Rank: run.Chosen.BestRank, Path: run.Chosen.Hits[0].Path,
				},
			})
		}
	}
	for lower, hc := range d.hints {
		assigned[lower] = append(assigned[lower], assignment{
			Repo: hc.Repo, FromHint: true,
			Evidence: Evidence{Kind: "hint", Source: "github:" + hc.Repo, Folder: hc.Folder, Basis: "GitHub URL hint confirmed by the TOC in the repository", Path: hc.Tocs[0]},
		})
	}

	final := map[string]string{} // lower folder to lower repo
	builds := map[string]*pkgBuild{}
	lowers := make([]string, 0, len(assigned))
	for lower := range assigned {
		lowers = append(lowers, lower)
	}
	slices.Sort(lowers)
	for _, lower := range lowers {
		list := assigned[lower]
		s := d.seedFor(lower)
		name := lower
		if s != nil {
			name = s.Folder
		}
		repos := map[string]bool{}
		for _, a := range list {
			repos[strings.ToLower(a.Repo)] = true
		}
		var chosen string
		if hc := d.hints[lower]; hc != nil {
			chosen = strings.ToLower(hc.Repo)
			for _, a := range list {
				if !a.FromHint && !strings.EqualFold(a.Repo, hc.Repo) {
					d.review = append(d.review, ReviewItem{
						Reason: reasonHintDisagrees, Folders: []string{name}, Source: a.Evidence.Source,
						Detail: fmt.Sprintf("code search chose %s, the confirmed GitHub URL hint names %s; the hint is used", a.Repo, hc.Repo),
					})
				}
			}
		} else if len(repos) == 1 {
			for r := range repos {
				chosen = r
			}
		} else {
			var names []string
			for _, a := range list {
				names = append(names, a.Repo+" ("+a.Evidence.Source+")")
			}
			slices.Sort(names)
			d.review = append(d.review, ReviewItem{
				Reason: reasonConflict, Folders: []string{name},
				Detail: "the IDs of this folder point to different repositories: " + strings.Join(slices.Compact(names), ", "),
			})
			continue
		}
		final[lower] = chosen
		b := builds[chosen]
		if b == nil {
			b = &pkgBuild{key: chosen, folders: map[string]string{}, hits: map[string][]string{}, hintTOC: map[string][]string{}}
			builds[chosen] = b
		}
		b.folders[lower] = name
		for _, a := range list {
			if strings.EqualFold(a.Repo, chosen) {
				if b.repo == "" {
					b.repo = a.Repo
				}
				b.evid = append(b.evid, a.Evidence)
			}
		}
		if hc := d.hints[lower]; hc != nil {
			b.hintTOC[lower] = hc.Tocs
		}
	}
	for _, run := range d.runs {
		if run.Chosen == nil {
			continue
		}
		if b := builds[run.Chosen.key()]; b != nil {
			for _, h := range run.Chosen.Hits {
				b.hits[strings.ToLower(h.Folder)] = append(b.hits[strings.ToLower(h.Folder)], h.Path)
			}
		}
	}

	repoKeys := make([]string, 0, len(builds))
	for key := range builds {
		repoKeys = append(repoKeys, key)
	}
	slices.Sort(repoKeys)
	cands := &Candidates{Format: candidatesFormat}
	covered := map[string]bool{}
	for _, key := range repoKeys {
		pkg, err := d.buildPackage(builds[key])
		if err != nil {
			return nil, nil, err
		}
		if pkg == nil {
			for lower := range builds[key].folders {
				delete(final, lower)
			}
			continue
		}
		for _, f := range pkg.Folders {
			covered[strings.ToLower(f)] = true
		}
		cands.Packages = append(cands.Packages, *pkg)
	}

	resolvedFolders := len(final)
	d.reviewRuns(final)

	rev := &Review{Format: candidatesFormat}
	seeded := len(d.seeds)
	for _, folder := range d.registry {
		if !covered[strings.ToLower(folder)] && d.seedFor(folder) == nil {
			rev.FoldersWithoutIDs = append(rev.FoldersWithoutIDs, folder)
		}
	}
	coveredRegistry := 0
	for _, folder := range d.registry {
		if covered[strings.ToLower(folder)] {
			coveredRegistry++
		}
	}
	sortReview(d.review)
	rev.Items = d.review
	if rev.Items == nil {
		rev.Items = []ReviewItem{}
	}
	rev.Summary = ReviewSummary{
		RegistryFolders: len(d.registry), SeededFolders: seeded, ResolvedFolders: resolvedFolders,
		CoveredFolders: coveredRegistry, Packages: len(cands.Packages), ReviewItems: len(rev.Items),
	}
	if cands.Packages == nil {
		cands.Packages = []Package{}
	}
	when := d.c.DataTime()
	if when.IsZero() {
		when = d.c.cfg.Clock.Now()
	}
	cands.GeneratedAt = when.UTC()
	rev.GeneratedAt = cands.GeneratedAt
	return cands, rev, nil
}

func sortReview(items []ReviewItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		fa, fb := "", ""
		if len(a.Folders) > 0 {
			fa = strings.ToLower(a.Folders[0])
		}
		if len(b.Folders) > 0 {
			fb = strings.ToLower(b.Folders[0])
		}
		if fa != fb {
			return fa < fb
		}
		return a.Source < b.Source
	})
}

// reviewRuns turns every lookup that did not settle a folder into a review
// item.
func (d *discoverer) reviewRuns(final map[string]string) {
	for _, run := range d.runs {
		if run.Chosen != nil || run.Skipped {
			continue
		}
		open := false
		for _, f := range run.Folders {
			if _, ok := final[strings.ToLower(f)]; !ok {
				open = true
			}
		}
		if !open {
			continue
		}
		item := ReviewItem{Folders: run.Folders, Source: run.source()}
		more := ""
		if run.Total > run.Returned {
			more = fmt.Sprintf(" (the first %d of %d hits were read)", run.Returned, run.Total)
		}
		switch run.Class {
		case "api_error":
			item.Reason, item.Detail = reasonAPIError, "code search failed: "+run.Err.Error()
		case "no_hits":
			item.Reason, item.Detail = reasonUnresolved, "code search returned no hits"
		case "wrong_names":
			item.Reason = reasonUnresolved
			item.Detail = fmt.Sprintf("%d hits, none with a TOC file named after the folder%s", run.Returned, more)
		case "filtered":
			item.Reason = reasonFiltered
			item.Detail = "every repository with a matching TOC was filtered out" + more
			item.Candidates = reviewCands(run.Cands, false)
		case "ambiguous":
			item.Reason = reasonAmbiguous
			item.Detail = "several plausible repositories and no clear winner" + more
			item.Candidates = reviewCands(run.Plaus, true)
		default:
			continue
		}
		d.review = append(d.review, item)
	}
}

func reviewCands(cands []*cand, plausible bool) []ReviewCandidate {
	var out []ReviewCandidate
	for _, g := range cands {
		rc := ReviewCandidate{Repo: g.Repo, Why: g.Why}
		if g.Meta != nil {
			rc.Stars = g.Meta.Stars
		}
		if plausible {
			rc.Why = "plausible"
		}
		for i, h := range g.Hits {
			if i < maxReviewPaths {
				rc.Paths = append(rc.Paths, h.Path)
			}
		}
		out = append(out, rc)
	}
	slices.SortStableFunc(out, func(a, b ReviewCandidate) int {
		if a.Stars != b.Stars {
			return b.Stars - a.Stars
		}
		return strings.Compare(strings.ToLower(a.Repo), strings.ToLower(b.Repo))
	})
	if len(out) > maxReviewCands {
		out = out[:maxReviewCands]
	}
	return out
}

// buildPackage collects everything build needs for one repository. It
// returns nil when the repository cannot be read.
func (d *discoverer) buildPackage(b *pkgBuild) (*Package, error) {
	meta, err := d.meta(b.repo)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		d.review = append(d.review, ReviewItem{
			Reason: reasonAPIError, Folders: sortedValues(b.folders), Source: "github:" + b.repo,
			Detail: "the chosen repository can no longer be read",
		})
		return nil, nil
	}
	repo := b.repo
	if meta.FullName != "" {
		repo = meta.FullName
	}
	pkg := &Package{
		Repo: repo, Source: "github:" + repo, Stars: meta.Stars, Archived: meta.Archived,
		DefaultBranch: meta.DefaultBranch, PushedAt: meta.PushedAt,
		SeedFolders: sortedValues(b.folders),
	}
	pkg.PrimaryFolder = d.primaryFolder(pkg.SeedFolders, repo)
	pkg.IDs = d.packageIDs(pkg.SeedFolders, pkg.PrimaryFolder)

	folderSet := map[string]string{}
	addFolder := func(name string) {
		if installer.SafeFolderName(name) {
			if _, ok := folderSet[strings.ToLower(name)]; !ok {
				folderSet[strings.ToLower(name)] = name
			}
		}
	}
	for _, f := range pkg.SeedFolders {
		addFolder(f)
	}

	if err := d.readTocs(pkg, b); err != nil {
		return nil, err
	}
	if data, ok, err := d.a.readFile(d.ctx, repo, ".pkgmeta"); err != nil {
		if !isStatusError(err) {
			return nil, err
		}
	} else if ok {
		pm := parsePkgmeta(data)
		pkg.Pkgmeta = &PkgmetaInfo{PackageAs: pm.PackageAs, MoveFolders: pm.MoveFolders, SkippedMoves: pm.SkippedMoves}
		addFolder(pm.PackageAs)
		for _, f := range pm.MoveFolders {
			addFolder(f)
		}
	}
	if err := d.expandRoot(repo, folderSet, addFolder); err != nil {
		return nil, err
	}
	pkg.Folders = sortedValues(folderSet)

	if err := d.readRelease(pkg); err != nil {
		return nil, err
	}
	if data, ok, err := d.a.readFile(d.ctx, repo, ".github/FUNDING.yml"); err != nil {
		if !isStatusError(err) {
			return nil, err
		}
	} else if ok {
		if len(data) > maxFundingBytes {
			data = data[:maxFundingBytes]
		}
		pkg.FundingYML = string(data)
	}

	pkg.Evidence = b.evid
	slices.SortStableFunc(pkg.Evidence, func(a, b Evidence) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if c := compareFold(a.Folder, b.Folder); c != 0 {
			return c
		}
		return strings.Compare(a.Source, b.Source)
	})
	return pkg, nil
}

func sortedValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	slices.SortFunc(out, compareFold)
	return out
}

// primaryFolder picks the folder whose TOC names the package: the one named
// like the repository, else the shortest name.
func (d *discoverer) primaryFolder(folders []string, repo string) string {
	want := normName(repoNamePart(repo))
	for _, f := range folders {
		if normName(f) == want {
			return f
		}
	}
	best := folders[0]
	for _, f := range folders[1:] {
		if len(f) < len(best) {
			best = f
		}
	}
	return best
}

func (d *discoverer) packageIDs(folders []string, primary string) map[string][]string {
	ids := map[string][]string{}
	ordered := append([]string{primary}, slices.DeleteFunc(slices.Clone(folders), func(f string) bool { return f == primary })...)
	for _, f := range ordered {
		s := d.seedFor(f)
		if s == nil {
			continue
		}
		for kind, list := range s.IDs {
			for _, id := range list {
				if !slices.Contains(ids[kind], id) {
					ids[kind] = append(ids[kind], id)
				}
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// readTocs reads the TOC files of the primary folder for its title and
// interface numbers.
func (d *discoverer) readTocs(pkg *Package, b *pkgBuild) error {
	lower := strings.ToLower(pkg.PrimaryFolder)
	paths := append(slices.Clone(b.hits[lower]), b.hintTOC[lower]...)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) > maxTocFetch {
		paths = paths[:maxTocFetch]
	}
	var interfaces []int
	bestTitle, titleExact := "", false
	for _, p := range paths {
		data, ok, err := d.a.readFile(d.ctx, pkg.Repo, p)
		if err != nil {
			if isStatusError(err) {
				continue
			}
			return err
		}
		if !ok {
			continue
		}
		pkg.TocFiles = append(pkg.TocFiles, p)
		fields := parseTOC(data)
		interfaces = append(interfaces, tocInterfaces(fields)...)
		title := fields["title"]
		exact := strings.EqualFold(strings.TrimSuffix(path.Base(p), path.Ext(p)), pkg.PrimaryFolder)
		if title != "" && (bestTitle == "" || (exact && !titleExact)) {
			bestTitle, titleExact = title, exact
		}
	}
	pkg.Title = bestTitle
	slices.Sort(interfaces)
	pkg.Interfaces = slices.Compact(interfaces)
	return nil
}

// expandRoot adds registry folders that are root directories of the
// repository holding a TOC of that name.
func (d *discoverer) expandRoot(repo string, have map[string]string, add func(string)) error {
	root, found, err := d.a.listDir(d.ctx, repo, "")
	if err != nil {
		if isStatusError(err) {
			return nil
		}
		return err
	}
	if !found {
		return nil
	}
	for _, name := range d.registry {
		if _, ok := have[strings.ToLower(name)]; ok {
			continue
		}
		for _, entry := range root {
			if entry.Type != "dir" || !strings.EqualFold(entry.Name, name) {
				continue
			}
			inner, ok, err := d.a.listDir(d.ctx, repo, entry.Path)
			if err != nil {
				if isStatusError(err) {
					continue
				}
				return err
			}
			names := map[string]string{strings.ToLower(entry.Name): entry.Name}
			if ok && slices.ContainsFunc(inner, func(e dirEntry) bool {
				_, match := tocMatchesFolder(e.Name, names)
				return e.Type == "file" && match
			}) {
				add(entry.Name)
			}
		}
	}
	return nil
}

func (d *discoverer) readRelease(pkg *Package) error {
	rel, err := d.a.latestRelease(d.ctx, pkg.Repo)
	if err != nil {
		if isStatusError(err) {
			return nil
		}
		return err
	}
	if rel == nil {
		return nil
	}
	info := &ReleaseInfo{Tag: rel.Tag, PublishedAt: rel.PublishedAt, Assets: []string{}}
	var releaseJSON *assetInfo
	for i, asset := range rel.Assets {
		info.Assets = append(info.Assets, asset.Name)
		if asset.Name == "release.json" {
			releaseJSON = &rel.Assets[i]
		}
	}
	slices.Sort(info.Assets)
	if releaseJSON != nil {
		info.ReleaseJSON = true
		data, ok, err := d.a.assetBody(d.ctx, pkg.Repo, releaseJSON.ID)
		if err != nil && !isStatusError(err) {
			return err
		}
		if ok {
			if types, known := releaseJSONGameTypes(data); known {
				info.ReleaseJSONGameTypes = types
			}
		}
	}
	pkg.Release = info
	return nil
}
