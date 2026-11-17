package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/service"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type screen int

const (
	screenAddons screen = iota
	screenExplore
	screenDetails
	screenAdd
	screenSettings
)

type opKind int

const (
	opNone opKind = iota
	opScan
	opCheck
	opUpdate
	opUpdateAll
	opAdd
	opRename
	opSearch
)

const (
	filterAll = iota
	filterManaged
	filterUnmanaged
)

type Deps struct {
	Config       *config.Config
	Providers    map[string]provider.Provider
	RegistryPath string
	BackupRoot   string
}

type addonRow struct {
	InstallPath string
	Flavor      string
	Name        string
	Installed   string
	Managed     bool
	Ref         string
	Provider    string
	Latest      string
	Status      string
	Author      string
}

type registryLoadedMsg struct {
	reg *registry.Registry
	err error
}

type serviceEventMsg service.Event

type opClosedMsg struct{}

type opResult struct {
	statuses []service.UpdateStatus
	remote   []provider.RemoteAddon
	err      error
}

const dismissKey = "ctrl+x"

type noticeKind int

const (
	noticeError noticeKind = iota
	noticeWarning
)

type notice struct {
	kind noticeKind
	text string
}

type Model struct {
	deps Deps

	reg  *registry.Registry
	rows []addonRow

	screen screen

	table        table.Model
	exploreTable table.Model
	spinner      spinner.Model
	input        textinput.Model
	searchInput  textinput.Model
	exploreInput textinput.Model

	addInstall int
	addSource  int
	addField   int

	installFilter int
	gameFilter    string
	installLabels []string

	settingsCursor int
	renaming       bool
	renameInput    textinput.Model

	width  int
	height int

	busy           bool
	busyMsg        string
	errMsg         string
	notices        []notice
	filter         int
	searching      bool
	filterQuery    string
	exploreQuery   string
	exploreResults []provider.RemoteAddon
	exploreCursor  int

	opKind     opKind
	eventChan  chan service.Event
	resultChan chan opResult

	detailRow    addonRow
	detailRemote *provider.RemoteAddon
	autoScanned  bool
}

func New(deps Deps) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "github:Owner/Repo oder https://..."
	ti.Width = 60

	search := textinput.New()
	search.Prompt = "/ "
	search.Placeholder = "search addons"
	search.Width = 42
	exploreInput := textinput.New()
	exploreInput.Prompt = "> "
	exploreInput.Placeholder = "search provider catalogs"
	exploreInput.Width = 52

	ri := textinput.New()
	ri.Placeholder = "display title"
	ri.Width = 40

	m := &Model{
		deps:          deps,
		spinner:       sp,
		input:         ti,
		searchInput:   search,
		exploreInput:  exploreInput,
		renameInput:   ri,
		installFilter: -1,
	}
	m.initTable()
	m.initExploreTable()
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadRegistryCmd(), spinner.Tick)
}

func (m *Model) buildService(onEvent service.EmitFunc) *service.Service {
	return &service.Service{
		Providers:    m.deps.Providers,
		RegistryPath: m.deps.RegistryPath,
		BackupRoot:   m.deps.BackupRoot,
		OnEvent:      onEvent,
	}
}

func (m *Model) loadRegistryCmd() tea.Cmd {
	return func() tea.Msg {
		reg, err := registry.Load(m.deps.RegistryPath)
		return registryLoadedMsg{reg: reg, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTable()
		m.resizeExploreTable()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case registryLoadedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.reg = msg.reg
		m.installLabels = installationLabels(m.reg.Installations)
		if m.installFilter >= len(m.reg.Installations) {
			m.installFilter = -1
		}
		m.buildRows()
		m.rebuildTable()
		if len(m.reg.Installations) == 0 && !m.autoScanned {
			m.autoScanned = true
			return m, m.startScan()
		}
		return m, nil

	case serviceEventMsg:
		e := service.Event(msg)
		if e.Message != "" {
			m.busyMsg = e.Message
		}
		m.collectNotice(e)
		m.applyEvent(e)
		return m, waitForEvent(m.eventChan)

	case opClosedMsg:
		return m.finishOp()

	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == dismissKey && m.noticeVisible() {
		m.notices = nil
		m.errMsg = ""
		return m, nil
	}
	switch m.screen {
	case screenAddons:
		return m.handleAddonsKey(msg)
	case screenExplore:
		return m.handleExploreKey(msg)
	case screenDetails:
		return m.handleDetailsKey(msg)
	case screenAdd:
		return m.handleAddKey(msg)
	case screenSettings:
		return m.handleSettingsKey(msg)
	}
	return m, nil
}

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	var body string
	switch m.screen {
	case screenAddons:
		body = m.addonsView()
	case screenExplore:
		body = m.exploreView()
	case screenDetails:
		body = m.detailsView()
	case screenAdd:
		body = m.addView()
	case screenSettings:
		body = m.settingsView()
	}

	notices := ""
	if lines := m.noticeLines(); len(lines) > 0 {
		notices = noticeBar.Render(strings.Join(lines, "\n")) + "\n"
	}
	return m.headerView() + "\n" + contentPanel.Render(body) + "\n" + notices + footerBar.Render(m.footerView())
}

func (m *Model) headerView() string {
	tabs := []string{"Addons", "Explore", "Settings"}
	var rendered string
	for i, t := range tabs {
		active := false
		if m.screen == screenAddons && i == 0 {
			active = true
		}
		if m.screen == screenExplore && i == 1 {
			active = true
		}
		if m.screen == screenSettings && i == 2 {
			active = true
		}
		if active {
			rendered += tabActive.Render(t)
		} else {
			rendered += tabInactive.Render(t)
		}
	}
	title := titleStyle.Render("wowtui")
	return title + rendered
}

func (m *Model) footerView() string {
	if m.busy {
		return statusBusy.Render(m.spinner.View() + " " + m.busyMsg)
	}
	help := m.helpText()
	if m.noticeVisible() {
		help = dismissKey + " dismiss · " + help
	}
	return helpStyle.Render(help)
}

func (m *Model) collectNotice(e service.Event) {
	var kind noticeKind
	switch e.Stage {
	case service.StageWarning:
		kind = noticeWarning
	case service.StageError:
		kind = noticeError
	default:
		return
	}
	text := e.Message
	if text == "" && e.Err != nil {
		text = e.Err.Error()
	}
	if text != "" {
		m.notices = append(m.notices, notice{kind: kind, text: text})
	}
}

func (m *Model) noticeEntries() []notice {
	var entries []notice
	if m.errMsg != "" {
		entries = append(entries, notice{kind: noticeError, text: m.errMsg})
	}
	return append(entries, m.notices...)
}

func (m *Model) noticeVisible() bool {
	return !m.busy && len(m.noticeEntries()) > 0
}

func (m *Model) noticeLines() []string {
	if !m.noticeVisible() {
		return nil
	}
	entries := m.noticeEntries()
	lines := make([]string, 0, len(entries))
	for _, n := range entries {
		if n.kind == noticeWarning {
			lines = append(lines, warnStyle.Render("! "+n.text))
		} else {
			lines = append(lines, errStyle.Render("✗ "+n.text))
		}
	}
	return lines
}

func (m *Model) helpText() string {
	switch m.screen {
	case screenAddons:
		if m.searching {
			return "enter apply · esc cancel · type to search"
		}
		return "↑/↓ navigate · f search · i game version · I installation · d details · a add · r check · u update · tab explore · q quit"
	case screenExplore:
		return "enter search · ↑/↓ select · a install selected · esc clear · tab settings · q quit"
	case screenDetails:
		return "esc back"
	case screenAdd:
		return "enter submit · esc cancel"
	case screenSettings:
		return "esc/tab addons · ↑/↓ select installation · r rename"
	}
	return ""
}

func waitForEvent(ch chan service.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return opClosedMsg{}
		}
		return serviceEventMsg(e)
	}
}

func (m *Model) startOp(kind opKind, run func(svc *service.Service) opResult) tea.Cmd {
	m.busy = true
	m.errMsg = ""
	m.notices = nil
	m.opKind = kind
	m.eventChan = make(chan service.Event, 64)
	m.resultChan = make(chan opResult, 1)
	evCh := m.eventChan
	resCh := m.resultChan

	go func() {
		defer close(evCh)
		svc := m.buildService(func(e service.Event) { evCh <- e })
		resCh <- run(svc)
	}()
	return waitForEvent(m.eventChan)
}

func (m *Model) finishOp() (tea.Model, tea.Cmd) {
	m.busy = false
	m.busyMsg = ""
	kind := m.opKind
	m.opKind = opNone

	var result opResult
	select {
	case result = <-m.resultChan:
	default:
	}

	if result.err != nil {
		m.notices = append([]notice{{kind: noticeError, text: result.err.Error()}}, m.notices...)
	}

	switch kind {
	case opCheck:
		m.mergeStatuses(result.statuses)
	case opSearch:
		m.exploreResults = result.remote
		m.exploreCursor = 0
		return m, nil
	default:
		return m, m.loadRegistryCmd()
	}
	return m, nil
}

func (m *Model) applyEvent(e service.Event) {
	if e.Ref == "" {
		return
	}
	stage := ""
	switch e.Stage {
	case service.StageResolve:
		stage = "checking"
	case service.StageDownload:
		stage = "downloading"
	case service.StageExtract:
		stage = "extracting"
	case service.StageBackup:
		stage = "backing up"
	case service.StageInstall:
		stage = "installing"
	case service.StageDone:
		stage = "installed"
	case service.StageCurrent:
		stage = "current"
	}
	if stage == "" {
		return
	}
	for i := range m.rows {
		if m.rows[i].Managed && m.rows[i].Ref == e.Ref {
			m.rows[i].Status = stage
		}
	}
	m.rebuildTable()
}

func (m *Model) selectedRow() (addonRow, bool) {
	visible := m.filteredRows()
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(visible) {
		return addonRow{}, false
	}
	return visible[idx], true
}

func (m *Model) startCheck() tea.Cmd {
	m.busyMsg = "checking for updates..."
	return m.startOp(opCheck, func(svc *service.Service) opResult {
		statuses, err := svc.Check(context.Background())
		return opResult{statuses: statuses, err: err}
	})
}

func (m *Model) startScan() tea.Cmd {
	m.busyMsg = "scanning installations..."
	paths := m.deps.Config.WoW.Paths
	return m.startOp(opScan, func(svc *service.Service) opResult {
		_, err := svc.Scan(context.Background(), paths)
		return opResult{err: err}
	})
}

func (m *Model) startSearch(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		m.exploreResults = nil
		return nil
	}
	m.busyMsg = "searching providers..."
	return m.startOp(opSearch, func(svc *service.Service) opResult {
		enabled := make(map[string]bool)
		for _, name := range m.deps.Config.Providers.Order() {
			enabled[name] = m.deps.Config.Providers.Enabled(name)
		}
		manager := provider.NewManager(m.deps.Providers, m.deps.Config.Providers.Order(), enabled)
		results, err := manager.Search(context.Background(), query, m.selectedFlavor())
		return opResult{remote: results, err: err}
	})
}

func (m *Model) selectedFlavor() wow.GameFlavor {
	if m.gameFilter != "" && m.gameFilter != "all" {
		return wow.GameFlavor(m.gameFilter)
	}
	if m.reg != nil && len(m.reg.Installations) > 0 {
		return wow.GameFlavor(m.reg.Installations[0].Flavor)
	}
	return wow.Retail
}

func (m *Model) startUpdate(ref string) tea.Cmd {
	m.busyMsg = "updating " + ref + "..."
	return m.startOp(opUpdate, func(svc *service.Service) opResult {
		return opResult{err: svc.Update(context.Background(), ref, false)}
	})
}

func (m *Model) startUpdateAll() tea.Cmd {
	m.busyMsg = "updating all addons..."
	return m.startOp(opUpdateAll, func(svc *service.Service) opResult {
		return opResult{err: svc.UpdateAll(context.Background(), false)}
	})
}

func (m *Model) startAdd(ref string, installIdx int, source string) tea.Cmd {
	if m.reg == nil || installIdx < 0 || installIdx >= len(m.reg.Installations) {
		m.errMsg = "no installation selected, run scan first"
		return nil
	}
	reference, err := provider.ParseReference(ref)
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	inst := m.reg.Installations[installIdx]
	target := wow.Installation{Path: inst.Path, Flavor: wow.GameFlavor(inst.Flavor)}
	m.busyMsg = "installing " + ref + "..."
	return m.startOp(opAdd, func(svc *service.Service) opResult {
		return opResult{err: svc.Add(context.Background(), reference, target.Flavor, target, source)}
	})
}

var sourceOptions = []string{"auto", "release", "tag", "branch"}

func installationLabels(installs []registry.Installation) []string {
	flavorCount := map[string]int{}
	for _, inst := range installs {
		flavorCount[inst.Flavor]++
	}
	labels := make([]string, len(installs))
	for i, inst := range installs {
		switch {
		case inst.Title != "":
			labels[i] = inst.Title
		case flavorCount[inst.Flavor] == 1:
			labels[i] = inst.Flavor
		default:
			labels[i] = inst.Flavor + " · " + pathTail(inst.Path)
		}
	}
	return labels
}

func pathTail(p string) string {
	clean := strings.TrimRight(p, "/")
	parts := strings.Split(clean, "/")
	if len(parts) <= 2 {
		return clean
	}
	return "…/" + parts[len(parts)-2] + "/" + parts[len(parts)-1]
}
