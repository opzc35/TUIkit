package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/proxy"
)

type apiUserModel struct {
	proxyStore *proxy.Store
	apiAddr    string
	routes     []proxy.Route
	width      int
	height     int
}

func newAPIUser(proxyStore *proxy.Store, apiAddr string) apiUserModel {
	return apiUserModel{
		proxyStore: proxyStore,
		apiAddr:    apiAddr,
		routes:     proxyStore.EnabledRoutes(),
	}
}

func (m apiUserModel) Init() tea.Cmd {
	return nil
}

func (m apiUserModel) Update(msg tea.Msg) (apiUserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.routes = m.proxyStore.EnabledRoutes()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return navigateMsg(screenDashboard) }
		}
	}

	return m, nil
}

func (m apiUserModel) View() string {
	boxWidth := m.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 120 {
		boxWidth = 120
	}

	baseURL := "http://" + m.apiAddr

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Available API Endpoints"),
		"",
		dimStyle.Render("Proxy server: "+baseURL),
		"",
	)

	if len(m.routes) == 0 {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			dimStyle.Render("No API routes available"),
		)
	} else {
		for _, r := range m.routes {
			endpoint := baseURL + r.PathPrefix
			keyInfo := ""
			if r.KeyHeader == "Authorization" {
				keyInfo = "Bearer token in Authorization header"
			} else {
				keyInfo = "Key in " + r.KeyHeader + " header"
			}

			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				selectedMenuItemStyle.Render("> "+r.Name),
				fmt.Sprintf("  Upstream: %s", r.Upstream),
				fmt.Sprintf("  Endpoint: %s", endpoint),
				fmt.Sprintf("  Auth: %s", keyInfo),
				"",
			)
		}

		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			dimStyle.Render("Usage example:"),
			"",
		)

		if len(m.routes) > 0 {
			r := m.routes[0]
			endpoint := baseURL + r.PathPrefix
			example := fmt.Sprintf("  curl %s/chat/completions", endpoint)
			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				lipgloss.NewStyle().Foreground(primaryColor).Render(example),
				"",
			)
		}
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		dimStyle.Render("Press Esc or q to go back"),
	)

	return Box("API Relay", content, boxWidth)
}