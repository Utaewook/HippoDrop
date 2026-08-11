package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type guideModel struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func newGuideModel(content string) guideModel {
	return guideModel{
		content: content,
	}
}

func (m guideModel) Init() tea.Cmd {
	return nil
}

func (m guideModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		headerHeight := 2
		footerHeight := 2
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m guideModel) View() string {
	if !m.ready {
		return "\n  Initializing guide..."
	}
	header := "Google Drive GCP Setup Guide (Press 'q' or 'Esc' to exit)\n-----------------------------------------------------------"
	footer := fmt.Sprintf("-----------------------------------------------------------\nScroll: ↑/↓/PgUp/PgDn | %3.f%%", m.viewport.ScrollPercent()*100)
	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}
