package tui

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"hippodrop/internal/assets"
)

// ArrowKeyMap rebinds huh's default up/down/left/right navigation to
// plain arrow keys and Esc-to-quit, used across the setup wizard's forms.
func ArrowKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()

	nextKeys := key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next"),
	)
	prevKeys := key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "prev"),
	)

	km.Input.Next = nextKeys
	km.Input.Prev = prevKeys
	km.Note.Next = nextKeys
	km.Note.Prev = prevKeys
	km.Confirm.Next = nextKeys
	km.Confirm.Prev = prevKeys

	km.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("esc", "quit"),
	)

	return km
}

type credSetupModel struct {
	form      *huh.Form
	quitting  bool
	showGuide bool
	guide     guideModel
}

func (m credSetupModel) Init() tea.Cmd {
	return tea.Batch(m.form.Init(), m.guide.Init())
}

func (m credSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.showGuide {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc":
				m.showGuide = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
		case tea.WindowSizeMsg:
			updatedGuide, _ := m.guide.Update(msg)
			if g, ok := updatedGuide.(guideModel); ok {
				m.guide = g
			}
			// Update form with window size too just in case
			form, _ := m.form.Update(msg)
			if f, ok := form.(*huh.Form); ok {
				m.form = f
			}
			return m, nil
		}

		updatedGuide, cmd := m.guide.Update(msg)
		if g, ok := updatedGuide.(guideModel); ok {
			m.guide = g
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.showGuide = true
			return m, nil
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// Send window size to guide even when hidden so it can initialize its viewport size
		updatedGuide, _ := m.guide.Update(msg)
		if g, ok := updatedGuide.(guideModel); ok {
			m.guide = g
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		return m, tea.Quit
	}

	return m, cmd
}

func (m credSetupModel) View() string {
	if m.quitting {
		return ""
	}
	if m.showGuide {
		return m.guide.View()
	}
	footer := "\n  \033[2m[?] setup guide   [Esc] quit\033[0m"
	return m.form.View() + footer
}

// RunCredSetup drives the credential-input step of the init wizard,
// including the embedded "?"-triggered beginner's GCP guide.
func RunCredSetup(credPath, rootDir, portStr *string, confirm *bool, gcpSetupURL string) bool {
	form2 := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Google Drive Setup").
				Description(fmt.Sprintf(
					"HippoDrop connects to your Google Drive via OAuth 2.0.\n\n"+
						"[Warning] NEVER USED GOOGLE CLOUD BEFORE?\n"+
						"Press '?' on your keyboard to open the beginner's step-by-step guide.\n\n"+
						"Quick Summary (if you know what you're doing):\n"+
						"1. Go to: %s\n"+
						"2. Set up OAuth Consent Screen (Add your Gmail to 'Test users'!).\n"+
						"3. Create an OAuth Client ID (Type: 'Desktop app').\n"+
						"4. Download the JSON file and enter its absolute path below.",
					gcpSetupURL,
				)),
			huh.NewInput().
				Title("Absolute path to client_secret.json:").
				Value(credPath),
			huh.NewInput().
				Title("Port for this daemon (auto-detected):").
				Value(portStr).
				Validate(func(str string) error {
					p, err := strconv.Atoi(str)
					if err != nil || p <= 0 || p > 65535 {
						return errors.New("must be a valid port number (1-65535)")
					}
					return nil
				}),
			huh.NewInput().
				Title("Google Drive Root Directory Name:").
				Value(rootDir),
			huh.NewConfirm().
				Title("Ready to save configuration?").
				Value(confirm).
				Validate(func(v bool) error {
					if v {
						if *credPath == "" {
							return errors.New("credentials path cannot be empty")
						}
						if _, err := os.Stat(*credPath); os.IsNotExist(err) {
							return fmt.Errorf("credentials file not found: %s", *credPath)
						}
					}
					return nil
				}),
		),
	).WithKeyMap(ArrowKeyMap())

	model := credSetupModel{
		form:  form2,
		guide: newGuideModel(assets.GCPGuideText),
	}
	p := tea.NewProgram(model, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return false
	}
	if m, ok := result.(credSetupModel); ok && m.quitting {
		return false
	}
	return true
}
