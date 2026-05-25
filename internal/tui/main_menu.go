package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mainMenuModel struct {
	choices  []string
	cursor   int
	width    int
	height   int
}

func newMainMenu() mainMenuModel {
	return mainMenuModel{
		choices: []string{"Login", "Register", "Quit"},
		cursor:  0,
	}
}

func (m mainMenuModel) Init() tea.Cmd {
	return nil
}

func (m mainMenuModel) Update(msg tea.Msg) (mainMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			switch m.cursor {
			case 0:
				return m, func() tea.Msg { return navigateMsg(screenLogin) }
			case 1:
				return m, func() tea.Msg { return navigateMsg(screenRegister) }
			case 2:
				return m, tea.Quit
			}
		case "1":
			return m, func() tea.Msg { return navigateMsg(screenLogin) }
		case "2":
			return m, func() tea.Msg { return navigateMsg(screenRegister) }
		case "3", "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m mainMenuModel) View() string {
	// Calculate responsive box width
	boxWidth := m.width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	title := titleStyle.Render("TUIkit")
	subtitle := subtitleStyle.Render("SSH Terminal Workspace")

	var items []string
	for i, choice := range m.choices {
		cursor := "  "
		style := menuItemStyle
		if m.cursor == i {
			cursor = "> "
			style = selectedMenuItemStyle
		}
		items = append(items, cursor+style.Render(choice))
	}

	menu := lipgloss.JoinVertical(lipgloss.Left, items...)

	content := lipgloss.JoinVertical(lipgloss.Center,
		ClockBlock(),
		"",
		title,
		subtitle,
		"",
		menu,
		"",
		dimStyle.Render("Use ↑↓ or j/k to navigate, Enter to select"),
	)

	return Box("Welcome", content, boxWidth)
}

var dimStyle = lipgloss.NewStyle().Foreground(dimColor)
