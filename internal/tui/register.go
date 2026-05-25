package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
)

type registerModel struct {
	store    *auth.Store
	username textinput.Model
	password textinput.Model
	confirm  textinput.Model
	focus    int
	err      error
	success  bool
	width    int
	height   int
}

func newRegister(store *auth.Store) registerModel {
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

	confirm := textinput.New()
	confirm.Placeholder = "Confirm Password"
	confirm.EchoMode = textinput.EchoPassword
	confirm.EchoCharacter = '•'
	confirm.CharLimit = 64
	confirm.Width = 30

	return registerModel{
		store:    store,
		username: username,
		password: password,
		confirm:  confirm,
		focus:    0,
	}
}

func (m registerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m registerModel) Update(msg tea.Msg) (registerModel, tea.Cmd) {
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
		m.confirm.Width = inputWidth
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				m.focus--
			} else {
				m.focus++
			}

			if m.focus > 2 {
				m.focus = 0
			} else if m.focus < 0 {
				m.focus = 2
			}

			inputs := []*textinput.Model{&m.username, &m.password, &m.confirm}
			for i, input := range inputs {
				if i == m.focus {
					input.Focus()
				} else {
					input.Blur()
				}
			}

		case "enter":
			if m.password.Value() != m.confirm.Value() {
				m.err = &registerError{msg: "Passwords do not match"}
				return m, nil
			}

			err := m.store.CreateUser(m.username.Value(), m.password.Value(), auth.RoleUser)
			if err != nil {
				m.err = err
				return m, nil
			}

			m.success = true
			return m, func() tea.Msg { return navigateMsg(screenLogin) }

		case "esc":
			return m, func() tea.Msg { return navigateMsg(screenMainMenu) }
		}
	}

	var cmd tea.Cmd
	m.username, cmd = m.username.Update(msg)
	cmds = append(cmds, cmd)

	m.password, cmd = m.password.Update(msg)
	cmds = append(cmds, cmd)

	m.confirm, cmd = m.confirm.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m registerModel) View() string {
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

	confirmField := lipgloss.JoinHorizontal(lipgloss.Left,
		labelStyle.Render("Confirm:"),
		" ",
		m.confirm.View(),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		usernameField,
		"",
		passwordField,
		"",
		confirmField,
	)

	if m.err != nil {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			errorStyle.Render("Error: "+m.err.Error()),
		)
	}

	if m.success {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			successStyle.Render("✓ Account created successfully!"),
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		dimStyle.Render("Press Enter to register, Esc to go back"),
	)

	return Box("Register", content, boxWidth)
}

type registerError struct {
	msg string
}

func (e *registerError) Error() string {
	return e.msg
}
