package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/chat"
)

type adminScreen int

const (
	adminMain adminScreen = iota
	adminListUsers
	adminChatModeration
	adminPromoteUser
	adminDemoteUser
	adminDisableUser
	adminEnableUser
	adminResetPassword
	adminDeleteUser
	adminViewMessages
	adminDeleteMessage
	adminClearChannel
	adminMuteUser
	adminUnmuteUser
	adminListMutes
)

type adminModel struct {
	screen     adminScreen
	store      *auth.Store
	chatStore  *chat.Store
	user       auth.User
	users      []auth.User
	channels   []chat.Channel
	messages   []chat.Message
	mutes      []chat.Mute
	cursor     int
	input      textinput.Model
	notice     string
	width      int
	height     int
}

func newAdmin(store *auth.Store, chatStore *chat.Store) adminModel {
	ti := textinput.New()
	ti.Placeholder = "Enter username..."
	ti.CharLimit = 32
	ti.Width = 30

	return adminModel{
		screen:    adminMain,
		store:     store,
		chatStore: chatStore,
		input:     ti,
	}
}

func (m adminModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m adminModel) Update(msg tea.Msg) (adminModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Responsive input width for admin screens
		boxWidth := m.width / 2
		if boxWidth < 40 {
			boxWidth = 40
		}
		if boxWidth > 80 {
			boxWidth = 80
		}
		inputWidth := boxWidth - 16
		m.input.Width = inputWidth
		return m, nil

	case userLoginMsg:
		m.user = auth.User(msg)
		return m, nil

	case tea.KeyMsg:
		if m.screen == adminMain {
			return m.updateMainMenu(msg)
		}
		return m.updateSubScreen(msg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m adminModel) updateMainMenu(msg tea.KeyMsg) (adminModel, tea.Cmd) {
	options := []string{
		"List users",
		"Promote user",
		"Demote user",
		"Enable user",
		"Disable user",
		"Reset password",
		"Delete user",
		"Chat moderation",
		"Back",
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(options)-1 {
			m.cursor++
		}
	case "enter", " ":
		switch m.cursor {
		case 0:
			m.users = m.store.ListUsers()
			m.screen = adminListUsers
		case 1:
			m.screen = adminPromoteUser
			m.input.SetValue("")
			m.input.Focus()
		case 2:
			m.screen = adminDemoteUser
			m.input.SetValue("")
			m.input.Focus()
		case 3:
			m.screen = adminEnableUser
			m.input.SetValue("")
			m.input.Focus()
		case 4:
			m.screen = adminDisableUser
			m.input.SetValue("")
			m.input.Focus()
		case 5:
			m.screen = adminResetPassword
			m.input.SetValue("")
			m.input.Focus()
		case 6:
			m.screen = adminDeleteUser
			m.input.SetValue("")
			m.input.Focus()
		case 7:
			m.screen = adminChatModeration
			m.cursor = 0
		case 8:
			return m, func() tea.Msg { return navigateMsg(screenDashboard) }
		}
	case "esc", "q":
		return m, func() tea.Msg { return navigateMsg(screenDashboard) }
	}

	return m, nil
}

func (m adminModel) updateSubScreen(msg tea.KeyMsg) (adminModel, tea.Cmd) {
	if m.screen == adminChatModeration {
		return m.updateChatModerationMenu(msg)
	}

	switch msg.String() {
	case "esc":
		m.screen = adminMain
		m.cursor = 0
		m.notice = ""
		m.input.Blur()
		return m, nil
	case "enter":
		username := m.input.Value()
		if username == "" {
			m.notice = "Username required"
			return m, nil
		}
		m.executeAdminAction(username)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m adminModel) updateChatModerationMenu(msg tea.KeyMsg) (adminModel, tea.Cmd) {
	options := []string{
		"View channel messages",
		"Delete message",
		"Clear channel",
		"Mute user",
		"Unmute user",
		"List muted users",
		"Back",
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(options)-1 {
			m.cursor++
		}
	case "enter", " ":
		switch m.cursor {
		case 0:
			m.channels = m.chatStore.ListChannels()
			m.screen = adminViewMessages
			m.input.SetValue("")
			m.input.Placeholder = "Enter channel name..."
			m.input.Focus()
		case 1:
			m.screen = adminDeleteMessage
			m.input.SetValue("")
			m.input.Placeholder = "Enter message ID..."
			m.input.Focus()
		case 2:
			m.screen = adminClearChannel
			m.input.SetValue("")
			m.input.Placeholder = "Enter channel name..."
			m.input.Focus()
		case 3:
			m.screen = adminMuteUser
			m.input.SetValue("")
			m.input.Placeholder = "Enter username..."
			m.input.Focus()
		case 4:
			m.screen = adminUnmuteUser
			m.input.SetValue("")
			m.input.Placeholder = "Enter username..."
			m.input.Focus()
		case 5:
			m.mutes = m.chatStore.ListMutes()
			m.screen = adminListMutes
		case 6:
			m.screen = adminMain
			m.cursor = 0
		}
	case "esc", "q":
		m.screen = adminMain
		m.cursor = 0
	}

	return m, nil
}

func (m *adminModel) executeAdminAction(username string) {
	var err error
	switch m.screen {
	case adminPromoteUser:
		err = m.store.SetRole(username, auth.RoleAdmin)
	case adminDemoteUser:
		err = m.store.SetRole(username, auth.RoleUser)
	case adminEnableUser:
		err = m.store.SetActive(username, true)
	case adminDisableUser:
		err = m.store.SetActive(username, false)
	case adminResetPassword:
		err = m.store.ResetPassword(username, "newpassword123")
	case adminDeleteUser:
		err = m.store.DeleteUser(username)
	case adminDeleteMessage:
		id, parseErr := strconv.ParseInt(username, 10, 64)
		if parseErr != nil {
			m.notice = "Invalid message ID"
			return
		}
		err = m.chatStore.DeleteMessage(id, m.user.Username)
	case adminClearChannel:
		_, err = m.chatStore.ClearChannel(username, m.user.Username)
	case adminMuteUser:
		err = m.chatStore.MuteUser(username, 24, "Admin action", m.user.Username)
	case adminUnmuteUser:
		err = m.chatStore.UnmuteUser(username)
	}

	if err != nil {
		m.notice = fmt.Sprintf("Error: %v", err)
	} else {
		m.notice = "Action completed successfully"
	}
	m.input.SetValue("")
}

func (m adminModel) View() string {
	switch m.screen {
	case adminMain:
		return m.viewMainMenu()
	case adminListUsers:
		return m.viewListUsers()
	case adminChatModeration:
		return m.viewChatModerationMenu()
	case adminViewMessages:
		return m.viewInputScreen("View Messages", "Enter channel name")
	case adminDeleteMessage:
		return m.viewInputScreen("Delete Message", "Enter message ID")
	case adminClearChannel:
		return m.viewInputScreen("Clear Channel", "Enter channel name")
	case adminMuteUser:
		return m.viewInputScreen("Mute User", "Enter username")
	case adminUnmuteUser:
		return m.viewInputScreen("Unmute User", "Enter username")
	case adminListMutes:
		return m.viewListMutes()
	default:
		return m.viewInputScreen("Admin Action", "Enter username")
	}
}

func (m adminModel) viewMainMenu() string {
	boxWidth := m.width / 2
	if boxWidth < 45 {
		boxWidth = 45
	}
	if boxWidth > 70 {
		boxWidth = 70
	}

	options := []string{
		"List users",
		"Promote user",
		"Demote user",
		"Enable user",
		"Disable user",
		"Reset password",
		"Delete user",
		"Chat moderation",
		"Back",
	}

	var items []string
	for i, option := range options {
		cursor := "  "
		style := menuItemStyle
		if m.cursor == i {
			cursor = "> "
			style = selectedMenuItemStyle
		}
		items = append(items, cursor+style.Render(option))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		ClockBlock(),
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		dimStyle.Render("Use ↑↓ or j/k to navigate, Enter to select, Esc to go back"),
	)

	return Box("Administration", content, boxWidth)
}

func (m adminModel) viewChatModerationMenu() string {
	boxWidth := m.width / 2
	if boxWidth < 45 {
		boxWidth = 45
	}
	if boxWidth > 70 {
		boxWidth = 70
	}

	options := []string{
		"View channel messages",
		"Delete message",
		"Clear channel",
		"Mute user",
		"Unmute user",
		"List muted users",
		"Back",
	}

	var items []string
	for i, option := range options {
		cursor := "  "
		style := menuItemStyle
		if m.cursor == i {
			cursor = "> "
			style = selectedMenuItemStyle
		}
		items = append(items, cursor+style.Render(option))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		ClockBlock(),
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		dimStyle.Render("Use ↑↓ or j/k to navigate, Enter to select, Esc to go back"),
	)

	return Box("Chat Moderation", content, boxWidth)
}

func (m adminModel) viewListUsers() string {
	boxWidth := m.width - 20
	if boxWidth < 60 {
		boxWidth = 60
	}
	if boxWidth > 100 {
		boxWidth = 100
	}

	var userList []string
	userList = append(userList, fmt.Sprintf("%-20s %-10s %-8s %s",
		labelStyle.Render("USERNAME"),
		labelStyle.Render("ROLE"),
		labelStyle.Render("ACTIVE"),
		labelStyle.Render("CREATED"),
	))
	userList = append(userList, "")

	for _, user := range m.users {
		userList = append(userList, fmt.Sprintf("%-20s %-10s %-8t %s",
			user.Username,
			user.Role,
			user.Active,
			user.CreatedAt.Format("2006-01-02"),
		))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		ClockBlock(),
		"",
		lipgloss.JoinVertical(lipgloss.Left, userList...),
		"",
		dimStyle.Render("Press Esc to go back"),
	)

	return Box("Users", content, boxWidth)
}

func (m adminModel) viewListMutes() string {
	boxWidth := m.width - 20
	if boxWidth < 60 {
		boxWidth = 60
	}
	if boxWidth > 100 {
		boxWidth = 100
	}

	var muteList []string
	muteList = append(muteList, fmt.Sprintf("%-20s %-20s %s",
		labelStyle.Render("USERNAME"),
		labelStyle.Render("UNTIL"),
		labelStyle.Render("REASON"),
	))
	muteList = append(muteList, "")

	for _, mute := range m.mutes {
		muteList = append(muteList, fmt.Sprintf("%-20s %-20s %s",
			mute.Username,
			mute.Until.Format("2006-01-02 15:04"),
			mute.Reason,
		))
	}

	if len(m.mutes) == 0 {
		muteList = append(muteList, dimStyle.Render("No muted users"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		ClockBlock(),
		"",
		lipgloss.JoinVertical(lipgloss.Left, muteList...),
		"",
		dimStyle.Render("Press Esc to go back"),
	)

	return Box("Mutes", content, boxWidth)
}

func (m adminModel) viewInputScreen(title, prompt string) string {
	boxWidth := m.width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		ClockBlock(),
		"",
		labelStyle.Render(prompt+":"),
		"",
		m.input.View(),
	)

	if m.notice != "" {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			warningStyle.Render(m.notice),
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		dimStyle.Render("Press Enter to submit, Esc to go back"),
	)

	return Box(title, content, boxWidth)
}
