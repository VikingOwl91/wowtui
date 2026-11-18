package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

type fileReview struct {
	title, body string
	approval    *service.LocalChangesError
	offset      int
	width       int
	lines       []string
}

func changeCount(n int) string {
	switch n {
	case 0:
		return "all files match"
	case 1:
		return "1 local change"
	default:
		return fmt.Sprintf("%d local changes", n)
	}
}

func writeFileChanges(b *strings.Builder, changes []fingerprint.Change) {
	b.WriteString(changeCount(len(changes)))
	for _, f := range changes {
		fmt.Fprintf(b, "\n%s  %s", importText(f.Kind), importText(f.Path))
	}
}

func (m *Model) showImportFiles(c service.MappingCandidate) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s · release %s\n%s\n\nImport includes %s\n", importText(c.Reference), importText(c.ReleaseVersion), importText(c.InstallPath), folderCount(len(c.PackageFolders), ""))
	for _, name := range c.PackageFolders {
		fmt.Fprintln(&b, importText(name))
	}
	fmt.Fprintln(&b)
	writeFileChanges(&b, c.FileChanges)
	m.files = fileReview{title: "Package details", body: b.String()}
	m.screen = screenFileReview
}

func (m *Model) showUpdateFiles(review *service.LocalChangesError) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\nUpdating replaces these local changes with the release files.\n\n", importText(review.Ref), importText(review.InstallPath))
	writeFileChanges(&b, review.Changes)
	m.files = fileReview{title: "Review local changes", body: b.String(), approval: review}
	m.screen = screenFileReview
	for i := range m.rows {
		if m.rows[i].Managed && m.rows[i].InstallPath == review.InstallPath && m.rows[i].Ref == review.Ref {
			m.rows[i].Status = "local changes"
		}
	}
	m.rebuildTable()
}

// Wrap once per terminal width so that paging a large file list stays cheap.
func (m *Model) reviewPageSize() int {
	f := &m.files
	width := max(1, m.width-contentPanel.GetHorizontalFrameSize())
	if width != f.width {
		f.width = width
		f.lines = strings.Split(lipgloss.NewStyle().Width(width).Render(f.body), "\n")
	}
	return max(1, m.height-6-len(m.noticeLines()))
}

func (m *Model) fileReviewView() string {
	f := &m.files
	size := m.reviewPageSize()
	f.offset = min(f.offset, max(0, len(f.lines)-size))
	end := min(f.offset+size, len(f.lines))
	return fmt.Sprintf("%s\n%s\n%d–%d/%d", headerStyle.Render(f.title), strings.Join(f.lines[f.offset:end], "\n"), f.offset+1, end, len(f.lines))
}

func (m *Model) handleFileReviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.files
	size := m.reviewPageSize()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "enter", "n":
		m.screen = screenImport
		if f.approval != nil {
			m.screen = screenAddons
		}
		m.files = fileReview{}
	case "y":
		if review := f.approval; review != nil {
			m.screen = screenAddons
			m.files = fileReview{}
			return m, m.startReviewedUpdate(review)
		}
	case "up", "k":
		f.offset--
	case "down", "j":
		f.offset++
	case "pgup":
		f.offset -= size
	case "pgdown":
		f.offset += size
	case "home":
		f.offset = 0
	case "end":
		f.offset = len(f.lines)
	}
	f.offset = max(0, min(f.offset, len(f.lines)-size))
	return m, nil
}
