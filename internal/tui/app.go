package tui

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/chat"
	"github.com/opzc35/tuikit/internal/proxy"
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
	screenCheckIn
	screenAPIUser
)

type App struct {
	store      *auth.Store
	chat       *chat.Store
	proxyStore *proxy.Store
	apiAddr    string
}

// paneModel wraps a screen model in a pane with a unique ID.
type paneModel struct {
	id        paneID
	screen    screen
	dashboard dashboardModel
	chatView  chatModel
	profile   profileModel
	admin     adminModel
	checkin   checkinModel
	apiUser   apiUserModel
}

type rootModel struct {
	user      *auth.User
	width     int
	height    int
	ctx       context.Context
	store     *auth.Store
	chat      *chat.Store
	proxyStore *proxy.Store
	apiAddr   string
	session   sshserver.Session
	layout   *Layout
	panes    map[paneID]*paneModel
	nextPane paneID

	mainMenu mainMenuModel
	login    loginModel
	register registerModel

	loggedIn bool
}

type navigateMsg screen
type userLoginMsg auth.User
type logoutMsg struct{}
type openChatMsg struct{ channel string }
type splitMsg struct{ dir splitDir }
type closePaneMsg struct{}
type focusNextMsg struct{}
type focusPrevMsg struct{}

func New(store *auth.Store, chatStore *chat.Store, proxyStore *proxy.Store, apiAddr string) *App {
	return &App{store: store, chat: chatStore, proxyStore: proxyStore, apiAddr: apiAddr}
}

func (a *App) HandleSession(ctx context.Context, session sshserver.Session) int {
	p := tea.NewProgram(
		newRootModel(ctx, a.store, a.chat, a.proxyStore, a.apiAddr, session),
		tea.WithContext(ctx),
		tea.WithInput(session.Stdin),
		tea.WithOutput(session.Stdout),
		tea.WithAltScreen(),
	)

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

func newRootModel(ctx context.Context, store *auth.Store, chatStore *chat.Store, proxyStore *proxy.Store, apiAddr string, session sshserver.Session) rootModel {
	m := rootModel{
		width:      80,
		height:     24,
		ctx:        ctx,
		store:      store,
		chat:       chatStore,
		proxyStore: proxyStore,
		apiAddr:    apiAddr,
		session:    session,
		panes:    map[paneID]*paneModel{},
		nextPane: paneID(1),
		loggedIn: false,
	}

	m.mainMenu = newMainMenu()
	m.login = newLogin(store)
	m.register = newRegister(store)

	return m
}

func (m rootModel) Init() tea.Cmd {
	return nil
}

func (m *rootModel) allocPaneID() paneID {
	id := m.nextPane
	m.nextPane++
	return id
}

func (m *rootModel) createPane(screen screen) *Pane {
	id := m.allocPaneID()

	pm := &paneModel{
		id:     id,
		screen: screen,
	}

	switch screen {
	case screenDashboard:
		pm.dashboard = newDashboard(m.chat)
		if m.user != nil {
			pm.dashboard.SetUser(*m.user)
		}
	case screenChat:
		pm.chatView = newChat(m.chat)
	case screenProfile:
		pm.profile = newProfile()
		if m.user != nil {
			pm.profile.user = *m.user
		}
	case screenAdmin:
		pm.admin = newAdmin(m.store, m.chat, m.proxyStore)
		if m.user != nil {
			pm.admin.user = *m.user
		}
	case screenCheckIn:
		pm.checkin = newCheckIn(m.store)
		if m.user != nil {
			pm.checkin.user = *m.user
		}
		pm.checkin.ranking = m.store.GetCheckInRanking(10)
	case screenAPIUser:
		pm.apiUser = newAPIUser(m.proxyStore, m.apiAddr)
	}

	m.panes[id] = pm
	return &Pane{id: id, screen: screen, focused: true}
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.mainMenu, cmd = m.mainMenu.Update(msg)
		cmds = append(cmds, cmd)
		m.login, cmd = m.login.Update(msg)
		cmds = append(cmds, cmd)
		m.register, cmd = m.register.Update(msg)
		cmds = append(cmds, cmd)

		if m.loggedIn && m.layout != nil {
			for id, dim := range m.layout.PaneDimensions(m.width, m.height-3) {
				pm := m.panes[id]
				sizeMsg := tea.WindowSizeMsg{Width: dim[0] - 2, Height: dim[1] - 2}
				var c tea.Cmd
				switch pm.screen {
				case screenDashboard:
					pm.dashboard, c = pm.dashboard.Update(sizeMsg)
				case screenChat:
					pm.chatView, c = pm.chatView.Update(sizeMsg)
				case screenProfile:
					pm.profile, c = pm.profile.Update(sizeMsg)
				case screenAdmin:
					pm.admin, c = pm.admin.Update(sizeMsg)
				case screenCheckIn:
					pm.checkin, c = pm.checkin.Update(sizeMsg)
				case screenAPIUser:
					pm.apiUser, c = pm.apiUser.Update(sizeMsg)
				}
				cmds = append(cmds, c)
			}
		}
		return m, tea.Batch(cmds...)

	case userLoginMsg:
		user := auth.User(msg)
		m.user = &user
		m.loggedIn = true

		pane := m.createPane(screenDashboard)
		m.layout = singlePane(pane)
		return m, nil

	case logoutMsg:
		m.user = nil
		m.loggedIn = false
		m.layout = nil
		m.panes = map[paneID]*paneModel{}
		m.nextPane = paneID(1)
		return m, nil

	case openChatMsg:
		if m.user == nil || m.layout == nil {
			return m, nil
		}
		// Open chat in a new split pane
		pane := m.createPane(screenChat)
		pm := m.panes[pane.id]
		cmd := pm.chatView.joinChannel(msg.channel, m.user.Username)
		m.layout.SplitFocused(pane, splitHorizontal)
		return m, cmd

	case navigateMsg:
		target := screen(msg)
		if !m.loggedIn {
			// Before login: only menu/login/register
			switch target {
			case screenLogin:
				pane := m.createPane(screenLogin)
				m.layout = singlePane(pane)
				return m, textinput.Blink
			case screenRegister:
				pane := m.createPane(screenRegister)
				m.layout = singlePane(pane)
				return m, textinput.Blink
			}
			return m, nil
		}
		// After login: replace focused pane with the target screen
		if m.layout == nil {
			return m, nil
		}
		focused := m.layout.FocusedPane()
		if focused == nil {
			return m, nil
		}
		// Unsubscribe old pane if it was a chat
		oldPm := m.panes[focused.id]
		if oldPm.screen == screenChat && oldPm.chatView.unsub != nil {
			oldPm.chatView.unsub()
		}
		delete(m.panes, focused.id)
		newPane := m.createPane(target)
		// Replace the focused pane in the layout
		m.layout.ReplacePane(focused.id, newPane)
		m.layout.SetFocus(newPane.id)
		return m, nil

	case splitMsg:
		if !m.loggedIn || m.layout == nil {
			return m, nil
		}
		focused := m.layout.FocusedPane()
		if focused == nil {
			return m, nil
		}
		// Split focused pane, creating a new dashboard pane
		pane := m.createPane(screenDashboard)
		m.layout.SplitFocused(pane, msg.dir)
		return m, nil

	case closePaneMsg:
		if m.layout == nil || m.layout.PaneCount() <= 1 {
			return m, nil
		}
		focused := m.layout.FocusedPane()
		if focused == nil {
			return m, nil
		}
		// Unsubscribe from chat if needed
		pm := m.panes[focused.id]
		if pm.screen == screenChat && pm.chatView.unsub != nil {
			pm.chatView.unsub()
		}

		newLayout := m.layout.RemovePane(focused.id)
		delete(m.panes, focused.id)
		if newLayout == nil {
			// Last pane removed, go back to dashboard
			pane := m.createPane(screenDashboard)
			m.layout = singlePane(pane)
		} else {
			m.layout = newLayout
			// Focus the nearest pane
			remaining := m.layout.AllPanes()
			if len(remaining) > 0 {
				m.layout.SetFocus(remaining[0].id)
			}
		}
		return m, nil

	case focusNextMsg:
		if m.layout != nil {
			focused := m.layout.FocusedPane()
			if focused != nil {
				nextID := m.layout.NextPane(focused.id)
				m.layout.SetFocus(nextID)
			}
		}
		return m, nil

	case focusPrevMsg:
		if m.layout != nil {
			focused := m.layout.FocusedPane()
			if focused != nil {
				prevID := m.layout.PrevPane(focused.id)
				m.layout.SetFocus(prevID)
			}
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// Pane management shortcuts (only when logged in)
		if m.loggedIn {
			switch msg.String() {
			case "ctrl+s":
				return m, func() tea.Msg { return splitMsg{dir: splitHorizontal} }
			case "ctrl+d":
				return m, func() tea.Msg { return splitMsg{dir: splitVertical} }
			case "ctrl+w":
				return m, func() tea.Msg { return closePaneMsg{} }
			case "ctrl+l":
				return m, func() tea.Msg { return focusNextMsg{} }
			case "ctrl+h":
				return m, func() tea.Msg { return focusPrevMsg{} }
			}
		}

		// Before login: route to login/register/mainMenu
		if !m.loggedIn {
			var cmd tea.Cmd
			switch {
			case m.layout == nil:
				m.mainMenu, cmd = m.mainMenu.Update(msg)
			default:
				focused := m.layout.FocusedPane()
				if focused == nil {
					return m, nil
				}
				pm := m.panes[focused.id]
				switch pm.screen {
				case screenLogin:
					m.login, cmd = m.login.Update(msg)
				case screenRegister:
					m.register, cmd = m.register.Update(msg)
				}
			}
			return m, cmd
		}
	}

	// Route messages to focused pane
	if !m.loggedIn {
		var cmd tea.Cmd
		m.mainMenu, cmd = m.mainMenu.Update(msg)
		return m, cmd
	}

	if m.layout == nil {
		return m, nil
	}

	focused := m.layout.FocusedPane()
	if focused == nil {
		return m, nil
	}

	pm := m.panes[focused.id]
	var cmd tea.Cmd

	switch pm.screen {
	case screenDashboard:
		pm.dashboard, cmd = pm.dashboard.Update(msg)
	case screenChat:
		pm.chatView, cmd = pm.chatView.Update(msg)
	case screenProfile:
		pm.profile, cmd = pm.profile.Update(msg)
	case screenAdmin:
		pm.admin, cmd = pm.admin.Update(msg)
	case screenCheckIn:
		pm.checkin, cmd = pm.checkin.Update(msg)
	case screenAPIUser:
		pm.apiUser, cmd = pm.apiUser.Update(msg)
	}

	// Also update non-focused chat panes to process events
	var cmds []tea.Cmd
	cmds = append(cmds, cmd)
	for _, p := range m.panes {
		if p.screen == screenChat && p.id != focused.id {
			var c tea.Cmd
			p.chatView, c = p.chatView.Update(msg)
			cmds = append(cmds, c)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m rootModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Not logged in: show centered menu/login/register
	if !m.loggedIn {
		var content string
		if m.layout == nil {
			content = m.mainMenu.View()
		} else {
			focused := m.layout.FocusedPane()
			if focused == nil {
				return "Loading..."
			}
			pm := m.panes[focused.id]
			switch pm.screen {
			case screenLogin:
				content = m.login.View()
			case screenRegister:
				content = m.register.View()
			}
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	// Logged in: render pane layout
	if m.layout == nil {
		return "Loading..."
	}

	mainHeight := m.height - 3 // Reserve space for status bar

	// Update pane models with their allocated content dimensions before rendering
	paneDims := m.layout.PaneDimensions(m.width, mainHeight)
	for id, dim := range paneDims {
		pm := m.panes[id]
		sizeMsg := tea.WindowSizeMsg{Width: dim[0] - 2, Height: dim[1] - 2}
		switch pm.screen {
		case screenDashboard:
			pm.dashboard, _ = pm.dashboard.Update(sizeMsg)
		case screenChat:
			pm.chatView, _ = pm.chatView.Update(sizeMsg)
		case screenProfile:
			pm.profile, _ = pm.profile.Update(sizeMsg)
		case screenAdmin:
			pm.admin, _ = pm.admin.Update(sizeMsg)
		case screenCheckIn:
			pm.checkin, _ = pm.checkin.Update(sizeMsg)
		case screenAPIUser:
			pm.apiUser, _ = pm.apiUser.Update(sizeMsg)
		}
	}

	views := map[paneID]string{}
	for id, pm := range m.panes {
		switch pm.screen {
		case screenDashboard:
			views[id] = pm.dashboard.View()
		case screenChat:
			views[id] = pm.chatView.View()
		case screenProfile:
			views[id] = pm.profile.View()
		case screenAdmin:
			views[id] = pm.admin.View()
		case screenCheckIn:
			views[id] = pm.checkin.View()
		case screenAPIUser:
			views[id] = pm.apiUser.View()
		}
	}

	layoutView := m.layout.Render(m.width, mainHeight, views)

	statusBar := statusBarStyle.Width(m.width - 4).Render(
		fmt.Sprintf(" %s %s | User: %s (%s) | Panes: %s | Ctrl+s/d split | Ctrl+w close | Ctrl+h/l focus",
			time.Now().Format("15:04:05"),
			time.Now().Format("2006-01-02"),
			m.user.Username,
			m.user.Role,
			m.layout.StatusLine(),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		layoutView,
		statusBar,
	)
}