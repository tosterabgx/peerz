package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type TorrentView struct {
	ID       string // hex info hash
	Name     string
	Size     int64
	Pieces   int
	Progress float64 // 0..1
	DownRate int64   // bytes/sec
	UpRate   int64
	Peers    int
	State    string // "idle", "checking", "downloading", "seeding", "paused", "error"
}

type Backend interface {
	Snapshot() []TorrentView
	TogglePause(id string)
}

type tickMsg time.Time

type Model struct {
	backend     Backend
	torrents    []TorrentView
	cursor      int
	showDetails bool
	width       int
	height      int
}

func New(b Backend) Model {
	return Model{backend: b, torrents: b.Snapshot()}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			m.cursor = len(m.torrents) - 1
		case "enter":
			m.showDetails = !m.showDetails
		case "space", "p":
			if t, ok := m.selected(); ok {
				m.backend.TogglePause(t.ID)
				m.torrents = m.backend.Snapshot()
			}
		}
		m.clampCursor()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickMsg:
		m.torrents = m.backend.Snapshot()
		m.clampCursor()
		return m, tick()
	}
	return m, nil
}

func (m Model) selected() (TorrentView, bool) {
	if m.cursor < 0 || m.cursor >= len(m.torrents) {
		return TorrentView{}, false
	}
	return m.torrents[m.cursor], true
}

func (m *Model) clampCursor() {
	m.cursor = min(m.cursor, len(m.torrents)-1)
	m.cursor = max(m.cursor, 0)
}
