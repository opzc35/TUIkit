package tui

import (
	"errors"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
)

type loginModel struct {
	store    *auth.Store
	username textinput.Model
	password textinput.Model
	focus    int
	err      error
	width    int
	height   int
}

func newLogin(store *auth.Store) loginModel {
	username := textinput.New()
	username.Placeholder = "Username"
	username.Focus()
	username.CharLimit = 32
	username.Width = 30

	password := textinput.New()
	password.Placeholder = "Password"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'
	password.CharLimit = 64
	password.Width = 30

	return loginModel{
		store:    store,
		username: username,
		password: password,
		focus:    0,
	}
}

func (m loginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m loginModel) Update(msg tea.Msg) (loginModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Responsive input width
		boxWidth := m.width / 2
		if boxWidth < 40 {
			boxWidth = 40
		}
		if boxWidth > 80 {
			boxWidth = 80
		}
		inputWidth := boxWidth - 16
		m.username.Width = inputWidth
		m.password.Width = inputWidth
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				m.focus--
			} else {
				m.focus++
			}

			if m.focus > 1 {
				m.focus = 0
			} else if m.focus < 0 {
				m.focus = 1
			}

			if m.focus == 0 {
				m.username.Focus()
				m.password.Blur()
			} else {
				m.username.Blur()
				m.password.Focus()
			}

		case "enter":
			user, err := m.store.Authenticate(m.username.Value(), m.password.Value())
			if err != nil {
				m.err = err
				return m, nil
			}
			return m, func() tea.Msg { return userLoginMsg(user) }

		case "esc":
			return m, func() tea.Msg { return navigateMsg(screenMainMenu) }
		}
	}

	var cmd tea.Cmd
	m.username, cmd = m.username.Update(msg)
	cmds = append(cmds, cmd)

	m.password, cmd = m.password.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m loginModel) View() string {
	// Calculate responsive box width
	boxWidth := m.width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	usernameField := lipgloss.JoinHorizontal(lipgloss.Left,
		labelStyle.Render("Username:"),
		" ",
		m.username.View(),
	)

	passwordField := lipgloss.JoinHorizontal(lipgloss.Left,
		labelStyle.Render("Password:"),
		" ",
		m.password.View(),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		usernameField,
		"",
		passwordField,
	)

	if m.err != nil {
		errMsg := ""
		if errors.Is(m.err, auth.ErrInactiveUser) {
			errMsg = "This account is disabled"
		} else {
			errMsg = "Invalid username or password"
		}
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			errorStyle.Render("Error: "+errMsg),
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		dimStyle.Render("Press Enter to login, Esc to go back"),
	)

	return Box("Login", content, boxWidth)
}
