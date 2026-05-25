package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
)

type profileModel struct {
	user   auth.User
	width  int
	height int
}

func newProfile() profileModel {
	return profileModel{}
}

func (m profileModel) Init() tea.Cmd {
	return nil
}

func (m profileModel) Update(msg tea.Msg) (profileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case userLoginMsg:
		m.user = auth.User(msg)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "enter":
			return m, func() tea.Msg { return navigateMsg(screenDashboard) }
		}
	}

	return m, nil
}

func (m profileModel) View() string {
	boxWidth := m.width / 2
	if boxWidth < 45 {
		boxWidth = 45
	}
	if boxWidth > 70 {
		boxWidth = 70
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s %s", labelStyle.Render("Username:"), m.user.Username),
		fmt.Sprintf("%s %s", labelStyle.Render("Role:"), string(m.user.Role)),
		fmt.Sprintf("%s %t", labelStyle.Render("Active:"), m.user.Active),
		fmt.Sprintf("%s %s", labelStyle.Render("Created:"), m.user.CreatedAt.Format("2006-01-02 15:04")),
		fmt.Sprintf("%s %s", labelStyle.Render("Updated:"), m.user.UpdatedAt.Format("2006-01-02 15:04")),
		"",
		dimStyle.Render("Press Esc or q to go back"),
	)

	return Box("Profile", content, boxWidth)
}
