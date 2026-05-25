package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/chat"
	"github.com/opzc35/tuikit/internal/sshserver"
)

type screen int

const (
	screenMainMenu screen = iota
	screenLogin
	screenRegister
	screenDashboard
	screenChat
	screenProfile
	screenAdmin
)

type App struct {
	store *auth.Store
	chat  *chat.Store
}

type rootModel struct {
	screen  screen
	user    *auth.User
	width   int
	height  int
	ctx     context.Context
	store   *auth.Store
	chat    *chat.Store
	session sshserver.Session

	mainMenu  mainMenuModel
	login     loginModel
	register  registerModel
	dashboard dashboardModel
	chatView  chatModel
	profile   profileModel
	admin     adminModel
}

type navigateMsg screen
type userLoginMsg auth.User
type logoutMsg struct{}
type openChatMsg struct{ channel string }

func New(store *auth.Store, chatStore *chat.Store) *App {
	return &App{store: store, chat: chatStore}
}

func (a *App) HandleSession(ctx context.Context, session sshserver.Session) int {
	p := tea.NewProgram(
		newRootModel(ctx, a.store, a.chat, session),
		tea.WithContext(ctx),
		tea.WithInput(session.Stdin),
		tea.WithOutput(session.Stdout),
		tea.WithAltScreen(),
	)

	// Bridge SSH window changes to bubbletea
	go func() {
		for win := range session.WindowChanges {
			p.Send(tea.WindowSizeMsg{
				Width:  int(win.Width),
				Height: int(win.Height),
			})
		}
	}()

	if _, err := p.Run(); err != nil {
		if err == io.EOF {
			return 0
		}
		return 1
	}
	return 0
}

func newRootModel(ctx context.Context, store *auth.Store, chatStore *chat.Store, session sshserver.Session) rootModel {
	m := rootModel{
		screen:  screenMainMenu,
		width:   80,
		height:  24,
		ctx:     ctx,
		store:   store,
		chat:    chatStore,
		session: session,
	}

	m.mainMenu = newMainMenu()
	m.login = newLogin(store)
	m.register = newRegister(store)
	m.dashboard = newDashboard(chatStore)
	m.chatView = newChat(chatStore)
	m.profile = newProfile()
	m.admin = newAdmin(store, chatStore)

	return m
}

func (m rootModel) Init() tea.Cmd {
	return nil
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Forward WindowSizeMsg to all sub-models so they can recompute layouts
		var cmd tea.Cmd
		m.mainMenu, _ = m.mainMenu.Update(msg)
		m.login, _ = m.login.Update(msg)
		m.register, _ = m.register.Update(msg)
		m.dashboard, _ = m.dashboard.Update(msg)
		m.chatView, cmd = m.chatView.Update(msg)
		m.profile, _ = m.profile.Update(msg)
		m.admin, _ = m.admin.Update(msg)
		return m, cmd

	case navigateMsg:
		m.screen = screen(msg)
		return m, nil

	case userLoginMsg:
		user := auth.User(msg)
		m.user = &user
		m.screen = screenDashboard
		m.dashboard = newDashboard(m.chat)
		m.dashboard.SetUser(user)
		return m, nil

	case logoutMsg:
		m.user = nil
		m.screen = screenMainMenu
		return m, nil

	case openChatMsg:
		if m.user == nil {
			return m, nil
		}
		cmd := m.chatView.joinChannel(msg.channel, m.user.Username)
		m.screen = screenChat
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenMainMenu:
		m.mainMenu, cmd = m.mainMenu.Update(msg)
	case screenLogin:
		m.login, cmd = m.login.Update(msg)
	case screenRegister:
		m.register, cmd = m.register.Update(msg)
	case screenDashboard:
		m.dashboard, cmd = m.dashboard.Update(msg)
	case screenChat:
		m.chatView, cmd = m.chatView.Update(msg)
	case screenProfile:
		m.profile, cmd = m.profile.Update(msg)
	case screenAdmin:
		m.admin, cmd = m.admin.Update(msg)
	}

	return m, cmd
}

func (m rootModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var content string
	switch m.screen {
	case screenMainMenu:
		content = m.mainMenu.View()
	case screenLogin:
		content = m.login.View()
	case screenRegister:
		content = m.register.View()
	case screenDashboard:
		content = m.dashboard.View()
	case screenChat:
		content = m.chatView.View()
	case screenProfile:
		content = m.profile.View()
	case screenAdmin:
		content = m.admin.View()
	}

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
