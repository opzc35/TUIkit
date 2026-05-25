package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/chat"
)

type chatModel struct {
	chatStore  *chat.Store
	channel    string
	channelObj chat.Channel
	user       string
	messages   []chat.Message
	viewport   viewport.Model
	input      textinput.Model
	events     <-chan chat.Event
	unsub      func()
	width      int
	height     int
	notice     string
}

type chatEventMsg chat.Event

func newChat(chatStore *chat.Store) chatModel {
	vp := viewport.New(60, 20)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 58

	return chatModel{
		chatStore: chatStore,
		viewport:  vp,
		input:     ti,
		notice:    "Type /back to leave, /help for commands",
	}
}

func (m chatModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *chatModel) joinChannel(channel, user string) tea.Cmd {
	m.channel = channel
	m.user = user
	ch, ok := m.chatStore.Channel(channel)
	if !ok {
		m.notice = "Channel not found"
		return nil
	}
	m.channelObj = ch
	m.messages = m.chatStore.RecentMessages(channel, 30, false)

	events, unsub := m.chatStore.Subscribe(channel)
	m.events = events
	m.unsub = unsub

	m.updateViewport()
	return m.waitForEvent()
}

func (m *chatModel) waitForEvent() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-m.events
		if !ok {
			return nil
		}
		return chatEventMsg(event)
	}
}

func (m *chatModel) updateViewport() {
	var content strings.Builder
	if len(m.messages) == 0 {
		content.WriteString(dimStyle.Render("  No messages yet.\n"))
	} else {
		for _, msg := range m.messages {
			author := messageAuthorStyle.Render(msg.Author + ":")
			time := messageTimeStyle.Render(msg.CreatedAt.Format("15:04"))
			body := msg.Body
			if msg.Deleted {
				body = deletedMessageStyle.Render(fmt.Sprintf("<deleted by %s>", msg.DeletedBy))
			} else {
				body = messageBodyStyle.Render(body)
			}
			content.WriteString(fmt.Sprintf("  %s %s %s\n", time, author, body))
		}
	}
	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

func (m chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Responsive sizing
		sidebarWidth := m.width / 4
		if sidebarWidth < 24 {
			sidebarWidth = 24
		}
		contentWidth := m.width - sidebarWidth - 10
		if contentWidth < 40 {
			contentWidth = 40
		}
		viewportHeight := m.height - 14
		if viewportHeight < 10 {
			viewportHeight = 10
		}
		m.viewport.Width = contentWidth - 6
		m.viewport.Height = viewportHeight
		m.input.Width = contentWidth - 8
		m.updateViewport()
		return m, nil

	case openChatMsg:
		return m, func() tea.Msg { return msg }

	case chatEventMsg:
		event := chat.Event(msg)
		m.messages = m.chatStore.RecentMessages(m.channel, 30, false)
		if event.Text != "" {
			m.notice = event.Text
		} else {
			m.notice = fmt.Sprintf("Updated at %s", event.Time.Format("15:04:05"))
		}
		m.updateViewport()
		return m, m.waitForEvent()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.unsub != nil {
				m.unsub()
			}
			return m, func() tea.Msg { return navigateMsg(screenDashboard) }
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			switch strings.ToLower(text) {
			case "/back", "/quit", "/exit":
				if m.unsub != nil {
					m.unsub()
				}
				return m, func() tea.Msg { return navigateMsg(screenDashboard) }
			case "/help":
				m.notice = "Commands: /back leaves, /help shows this"
				m.input.SetValue("")
				return m, nil
			default:
				_, err := m.chatStore.PostMessage(m.channel, m.user, text)
				if err != nil {
					m.notice = fmt.Sprintf("Failed: %v", err)
				} else {
					m.notice = "Message sent"
				}
				m.input.SetValue("")
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) View() string {
	// Calculate responsive widths
	sidebarWidth := m.width / 4
	if sidebarWidth < 24 {
		sidebarWidth = 24
	}
	contentWidth := m.width - sidebarWidth - 10
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Create dynamic styles based on window size
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
			titleStyle.Render("Chat"),
			"",
			selectedMenuItemStyle.Render("> #"+m.channel),
			"",
			dimStyle.Render("Session"),
			"  User: "+m.user,
			"",
			dimStyle.Render("Press Esc to go back"),
		),
	)

	chatHeader := titleStyle.Render("#"+m.channel) + " " + subtitleStyle.Render("live channel")
	if m.channelObj.Topic != "" {
		chatHeader += "\n" + dimStyle.Render(m.channelObj.Topic)
	}

	noticeView := ""
	if m.notice != "" {
		noticeView = warningStyle.Render("Status: "+m.notice) + "\n"
	}

	chatView := dynamicContentStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			chatHeader,
			"",
			m.viewport.View(),
			"",
			noticeView,
			labelStyle.Render("> ")+m.input.View(),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("TUIkit Chat"),
		"",
		SplitScreen(sidebar, chatView),
	)
}
