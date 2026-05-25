package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/chat"
)

type dashScreen int

const (
	dashMain dashScreen = iota
	dashCreateChannel
)

type dashboardModel struct {
	chatStore   *chat.Store
	user        auth.User
	channels    []chat.Channel
	channelList list.Model
	screen      dashScreen
	nameInput   textinput.Model
	topicInput  textinput.Model
	inputFocus  int // 0 = name, 1 = topic
	createErr   error
	width       int
	height      int
}

type channelItem struct {
	channel chat.Channel
}

func (i channelItem) Title() string       { return "#" + i.channel.Name }
func (i channelItem) Description() string { return i.channel.Topic }
func (i channelItem) FilterValue() string { return i.channel.Name }

func newDashboard(chatStore *chat.Store) dashboardModel {
	channels := chatStore.ListChannels()
	items := make([]list.Item, len(channels))
	for i, ch := range channels {
		items[i] = channelItem{channel: ch}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(primaryColor).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(dimColor)

	l := list.New(items, delegate, 30, 20)
	l.Title = "Channels"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	ni := textinput.New()
	ni.Placeholder = "Channel name (letters, numbers, _ -)"
	ni.Focus()
	ni.CharLimit = 32
	ni.Width = 30

	ti := textinput.New()
	ti.Placeholder = "Topic (optional)"
	ti.CharLimit = 100
	ti.Width = 30

	return dashboardModel{
		chatStore:   chatStore,
		channels:    channels,
		channelList: l,
		screen:      dashMain,
		nameInput:   ni,
		topicInput:  ti,
		inputFocus:  0,
	}
}

func (m *dashboardModel) SetUser(user auth.User) {
	m.user = user
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	if m.screen == dashCreateChannel {
		return m.updateCreateChannel(msg)
	}
	return m.updateMain(msg)
}

func (m dashboardModel) updateMain(msg tea.Msg) (dashboardModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := m.width/2 - 16
		if inputWidth < 20 {
			inputWidth = 20
		}
		if inputWidth > 60 {
			inputWidth = 60
		}
		m.nameInput.Width = inputWidth
		m.topicInput.Width = inputWidth
		sidebarWidth := m.width / 3
		if sidebarWidth < 28 {
			sidebarWidth = 28
		}
		contentWidth := m.width - sidebarWidth - 8
		if contentWidth < 40 {
			contentWidth = 40
		}
		listHeight := m.height - 12
		if listHeight < 10 {
			listHeight = 10
		}
		m.channelList.SetSize(contentWidth-4, listHeight)
		return m, nil

	case tea.KeyMsg:
		// Intercept shortcut keys before passing to list
		switch msg.String() {
		case "enter":
			if item, ok := m.channelList.SelectedItem().(channelItem); ok {
				return m, func() tea.Msg { return openChatMsg{channel: item.channel.Name} }
			}
			return m, nil
		case "c":
			m.screen = dashCreateChannel
			m.nameInput.SetValue("")
			m.topicInput.SetValue("")
			m.nameInput.Focus()
			m.topicInput.Blur()
			m.inputFocus = 0
			m.createErr = nil
			return m, textinput.Blink
		case "p":
			return m, func() tea.Msg { return navigateMsg(screenProfile) }
		case "a":
			if m.user.Role == auth.RoleAdmin {
				return m, func() tea.Msg { return navigateMsg(screenAdmin) }
			}
			return m, nil
		case "q":
			return m, func() tea.Msg { return logoutMsg{} }
		}
	}

	var cmd tea.Cmd
	m.channelList, cmd = m.channelList.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m dashboardModel) updateCreateChannel(msg tea.Msg) (dashboardModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.inputFocus = 1
			m.nameInput.Blur()
			m.topicInput.Focus()
			return m, nil
		case "shift+tab", "up":
			m.inputFocus = 0
			m.topicInput.Blur()
			m.nameInput.Focus()
			return m, nil
		case "esc":
			m.screen = dashMain
			m.nameInput.Blur()
			m.topicInput.Blur()
			return m, nil
		case "enter":
			name := m.nameInput.Value()
			topic := m.topicInput.Value()
			if name == "" {
				m.createErr = fmt.Errorf("channel name is required")
				return m, nil
			}
			err := m.chatStore.CreateChannel(name, topic, m.user.Username)
			if err != nil {
				m.createErr = err
				return m, nil
			}
			// Channel created, navigate to chat
			return m, func() tea.Msg { return openChatMsg{channel: name} }
		}
	}

	var cmd tea.Cmd
	if m.inputFocus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.topicInput, cmd = m.topicInput.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m dashboardModel) View() string {
	if m.screen == dashCreateChannel {
		return m.viewCreateChannel()
	}
	return m.viewMain()
}

func (m dashboardModel) viewMain() string {
	// Calculate responsive widths
	sidebarWidth := m.width / 3
	if sidebarWidth < 28 {
		sidebarWidth = 28
	}
	contentWidth := m.width - sidebarWidth - 8
	if contentWidth < 40 {
		contentWidth = 40
	}

	dynamicSidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(sidebarWidth)

	dynamicContentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(contentWidth)

	sidebar := dynamicSidebarStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Navigation"),
			"",
			menuItemStyle.Render("> Channels"),
			"  Create (c)",
			"  Profile (p)",
			"  Admin (a)",
			"  Logout (q)",
		),
	)

	channelView := dynamicContentStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			m.channelList.View(),
			"",
			dimStyle.Render("Enter = join, c = create, p/a/q = navigate"),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("TUIkit Dashboard"),
		"",
		SplitScreen(sidebar, channelView),
	)
}

func (m dashboardModel) viewCreateChannel() string {
	boxWidth := m.width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	nameField := lipgloss.JoinHorizontal(lipgloss.Left,
		labelStyle.Render("Name:"),
		" ",
		m.nameInput.View(),
	)

	topicField := lipgloss.JoinHorizontal(lipgloss.Left,
		labelStyle.Render("Topic:"),
		" ",
		m.topicInput.View(),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		dimStyle.Render("2-32 chars, letters/numbers/_/-"),
		"",
		nameField,
		"",
		topicField,
	)

	if m.createErr != nil {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			errorStyle.Render("Error: "+m.createErr.Error()),
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		dimStyle.Render("Enter = create, Tab = next field, Esc = back"),
	)

	return Box("Create Channel", content, boxWidth)
}