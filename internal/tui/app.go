package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/chat"
	"github.com/opzc35/tuikit/internal/sshserver"
)

type App struct {
	store *auth.Store
	chat  *chat.Store
}

type terminal struct {
	ctx context.Context
	in  *bufio.Reader
	out io.Writer
	mu  sync.Mutex
}

func New(store *auth.Store, chatStore *chat.Store) *App {
	return &App{store: store, chat: chatStore}
}

func (a *App) HandleSession(ctx context.Context, session sshserver.Session) int {
	term := &terminal{
		ctx: ctx,
		in:  bufio.NewReader(session.Stdin),
		out: session.Stdout,
	}

	term.clear()
	term.printf("TUIkit SSH Console\r\n")
	term.printf("Remote: %s\r\n\r\n", session.RemoteAddr)

	for {
		switch term.menu("Main Menu", []string{"Login", "Register", "Quit"}) {
		case "1":
			user, ok := a.login(term)
			if ok {
				a.userMenu(term, user)
			}
		case "2":
			a.register(term)
		case "3", "q", "quit":
			term.println("Bye.")
			return 0
		default:
			term.pause("Invalid choice.")
		}
	}
}

func (a *App) login(term *terminal) (auth.User, bool) {
	term.clear()
	term.header("Login")

	username := term.prompt("Username")
	password := term.password("Password")

	user, err := a.store.Authenticate(username, password)
	if err != nil {
		if errors.Is(err, auth.ErrInactiveUser) {
			term.pause("This account is disabled.")
		} else {
			term.pause("Invalid username or password.")
		}
		return auth.User{}, false
	}
	return user, true
}

func (a *App) register(term *terminal) {
	term.clear()
	term.header("Register")
	term.println("Username: 3-32 characters; letters, numbers, underscore, hyphen.")
	term.println("")

	username := term.prompt("Username")
	password := term.password("Password")
	confirm := term.password("Confirm password")
	if password != confirm {
		term.pause("Passwords do not match.")
		return
	}

	if err := a.store.CreateUser(username, password, auth.RoleUser); err != nil {
		term.pause(fmt.Sprintf("Registration failed: %v", err))
		return
	}
	term.pause("Account created. You can now log in.")
}

func (a *App) userMenu(term *terminal, user auth.User) {
	for {
		term.clear()
		term.header(fmt.Sprintf("Welcome, %s", user.Username))
		term.printf("Role: %s\r\n\r\n", user.Role)

		options := []string{"Profile", "Chat", "Logout"}
		if user.Role == auth.RoleAdmin {
			options = []string{"Profile", "Chat", "Admin", "Logout"}
		}

		choice := term.menu("User Menu", options)
		switch {
		case choice == "1":
			a.profile(term, user)
		case choice == "2":
			a.chatMenu(term, user)
		case user.Role == auth.RoleAdmin && choice == "3":
			a.adminMenu(term, user)
		case (user.Role == auth.RoleAdmin && choice == "4") || (user.Role != auth.RoleAdmin && choice == "3"):
			return
		default:
			term.pause("Invalid choice.")
		}
	}
}

func (a *App) profile(term *terminal, user auth.User) {
	term.clear()
	term.header("Profile")
	term.printf("Username : %s\r\n", user.Username)
	term.printf("Role     : %s\r\n", user.Role)
	term.printf("Active   : %t\r\n", user.Active)
	term.printf("Created  : %s\r\n", user.CreatedAt.Format(time.RFC3339))
	term.printf("Updated  : %s\r\n", user.UpdatedAt.Format(time.RFC3339))
	term.pause("")
}

func (a *App) chatMenu(term *terminal, user auth.User) {
	for {
		switch term.menu("Chat", []string{"Open channel", "Create channel", "List channels", "Back"}) {
		case "1":
			channel := term.prompt("Channel")
			a.channelMenu(term, user, channel)
		case "2":
			a.createChannel(term, user)
		case "3":
			a.listChannels(term)
		case "4":
			return
		default:
			term.pause("Invalid choice.")
		}
	}
}

func (a *App) createChannel(term *terminal, user auth.User) {
	term.clear()
	term.header("Create Channel")
	term.println("Channel names allow letters, numbers, underscore, and hyphen.")
	term.println("")
	name := term.prompt("Name")
	topic := term.prompt("Topic")
	if err := a.chat.CreateChannel(name, topic, user.Username); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("Channel created.")
}

func (a *App) listChannels(term *terminal) {
	term.clear()
	term.header("Channels")
	term.printf("%-24s %-18s %s\r\n", "NAME", "CREATED BY", "TOPIC")
	term.println(strings.Repeat("-", 72))
	for _, channel := range a.chat.ListChannels() {
		term.printf("%-24s %-18s %s\r\n", channel.Name, channel.CreatedBy, channel.Topic)
	}
	term.pause("")
}

func (a *App) channelMenu(term *terminal, user auth.User, channelName string) {
	channel, ok := a.chat.Channel(channelName)
	if !ok {
		term.pause("Channel not found.")
		return
	}

	events, unsubscribe := a.chat.Subscribe(channel.Name)
	defer unsubscribe()

	draft := &liveDraft{}
	inputs := make(chan string)
	acks := make(chan bool)
	draftChanged := make(chan struct{}, 1)
	errs := make(chan error, 1)
	go func() {
		for {
			line, err := readLiveLine(term, draft, draftChanged)
			if err != nil {
				errs <- err
				return
			}
			inputs <- strings.TrimSpace(line)
			if !<-acks {
				return
			}
		}
	}()

	notice := "Live chat started. Type /back to leave, /help for commands."
	for {
		a.renderLiveChat(term, channel, notice, draft.String())
		select {
		case <-term.ctx.Done():
			return
		case err := <-errs:
			if err != nil {
				return
			}
		case <-draftChanged:
			continue
		case event := <-events:
			if event.Text != "" {
				notice = event.Text
			} else {
				notice = fmt.Sprintf("Updated at %s", event.Time.Format("15:04:05"))
			}
			continue
		case line := <-inputs:
			switch strings.ToLower(line) {
			case "":
				notice = "No message sent."
			case "/back", "/quit", "/exit":
				acks <- false
				return
			case "/help":
				notice = "Commands: /back leaves the channel, /help shows this line. Any other text sends a message."
				acks <- true
			default:
				if _, err := a.chat.PostMessage(channel.Name, user.Username, line); err != nil {
					notice = fmt.Sprintf("Failed: %v", err)
				} else {
					notice = "Message sent."
				}
				acks <- true
			}
			if line == "" {
				acks <- true
			}
		}
	}
}

func (a *App) renderLiveChat(term *terminal, channel chat.Channel, notice string, draft string) {
	var screen strings.Builder
	screen.WriteString("\x1b[2J\x1b[H")
	screen.WriteString("\x1b[1m#")
	screen.WriteString(channel.Name)
	screen.WriteString("\x1b[0m\r\n")
	screen.WriteString(strings.Repeat("=", len(channel.Name)+1))
	screen.WriteString("\r\n")
	if channel.Topic != "" {
		screen.WriteString(channel.Topic)
		screen.WriteString("\r\n")
	}
	screen.WriteString("\r\n")

	messages := a.chat.RecentMessages(channel.Name, 30, false)
	if len(messages) == 0 {
		screen.WriteString("No messages yet.\r\n")
	} else {
		for _, message := range messages {
			screen.WriteString(formatMessage(message))
			screen.WriteString("\r\n")
		}
	}

	screen.WriteString("\r\n")
	if notice != "" {
		screen.WriteString("Status: ")
		screen.WriteString(notice)
		screen.WriteString("\r\n")
	}
	screen.WriteString("Type a message and press Enter. /back exits, /help shows commands.\r\n")
	screen.WriteString("> ")
	screen.WriteString(draft)
	term.write(screen.String())
}

func (a *App) printMessages(term *terminal, channel string, limit int, includeDeleted bool) {
	messages := a.chat.RecentMessages(channel, limit, includeDeleted)
	if len(messages) == 0 {
		term.println("No messages.")
		return
	}
	for _, message := range messages {
		term.println(formatMessage(message))
	}
}

func formatMessage(message chat.Message) string {
	status := ""
	body := message.Body
	if message.Deleted {
		status = " [deleted]"
		body = fmt.Sprintf("<deleted by %s>", message.DeletedBy)
	}
	return fmt.Sprintf("[%d] %s %-16s %s%s",
		message.ID,
		message.CreatedAt.Format("01-02 15:04"),
		message.Author+":",
		body,
		status,
	)
}

func (a *App) adminMenu(term *terminal, user auth.User) {
	for {
		switch term.menu("Admin", []string{
			"List users",
			"Promote user",
			"Demote user",
			"Enable user",
			"Disable user",
			"Reset password",
			"Delete user",
			"Chat moderation",
			"Back",
		}) {
		case "1":
			a.listUsers(term)
		case "2":
			a.setRole(term, auth.RoleAdmin)
		case "3":
			a.setRole(term, auth.RoleUser)
		case "4":
			a.setActive(term, true)
		case "5":
			a.setActive(term, false)
		case "6":
			a.resetPassword(term)
		case "7":
			a.deleteUser(term)
		case "8":
			a.chatAdminMenu(term, user)
		case "9":
			return
		default:
			term.pause("Invalid choice.")
		}
	}
}

func (a *App) chatAdminMenu(term *terminal, user auth.User) {
	for {
		switch term.menu("Chat Moderation", []string{
			"View channel messages",
			"Delete message",
			"Clear channel",
			"Mute user",
			"Unmute user",
			"List muted users",
			"Back",
		}) {
		case "1":
			a.adminViewMessages(term)
		case "2":
			a.adminDeleteMessage(term, user)
		case "3":
			a.adminClearChannel(term, user)
		case "4":
			a.adminMuteUser(term, user)
		case "5":
			a.adminUnmuteUser(term)
		case "6":
			a.adminListMutes(term)
		case "7":
			return
		default:
			term.pause("Invalid choice.")
		}
	}
}

func (a *App) adminViewMessages(term *terminal) {
	term.clear()
	term.header("View Messages")
	channel := term.prompt("Channel")
	limit := parsePositiveInt(term.prompt("Limit"), 50)
	term.clear()
	term.header("#" + channel)
	a.printMessages(term, channel, limit, true)
	term.pause("")
}

func (a *App) adminDeleteMessage(term *terminal, user auth.User) {
	term.clear()
	term.header("Delete Message")
	id, err := strconv.ParseInt(term.prompt("Message ID"), 10, 64)
	if err != nil {
		term.pause("Invalid message ID.")
		return
	}
	if err := a.chat.DeleteMessage(id, user.Username); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("Message deleted.")
}

func (a *App) adminClearChannel(term *terminal, user auth.User) {
	term.clear()
	term.header("Clear Channel")
	channel := term.prompt("Channel")
	confirm := term.prompt(fmt.Sprintf("Type %q to confirm", channel))
	if strings.TrimSpace(confirm) != strings.TrimSpace(channel) {
		term.pause("Clear cancelled.")
		return
	}
	count, err := a.chat.ClearChannel(channel, user.Username)
	if err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause(fmt.Sprintf("Deleted %d messages.", count))
}

func (a *App) adminMuteUser(term *terminal, user auth.User) {
	term.clear()
	term.header("Mute User")
	username := term.prompt("Username")
	hours := parsePositiveInt(term.prompt("Hours"), 24)
	reason := term.prompt("Reason")
	if err := a.chat.MuteUser(username, hours, reason, user.Username); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("User muted.")
}

func (a *App) adminUnmuteUser(term *terminal) {
	term.clear()
	term.header("Unmute User")
	username := term.prompt("Username")
	if err := a.chat.UnmuteUser(username); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("User unmuted.")
}

func (a *App) adminListMutes(term *terminal) {
	term.clear()
	term.header("Muted Users")
	term.printf("%-24s %-20s %s\r\n", "USERNAME", "UNTIL", "REASON")
	term.println(strings.Repeat("-", 72))
	for _, mute := range a.chat.ListMutes() {
		term.printf("%-24s %-20s %s\r\n", mute.Username, mute.Until.Format("2006-01-02 15:04"), mute.Reason)
	}
	term.pause("")
}

func (a *App) listUsers(term *terminal) {
	term.clear()
	term.header("Users")
	term.printf("%-24s %-8s %-8s %s\r\n", "USERNAME", "ROLE", "ACTIVE", "CREATED")
	term.println(strings.Repeat("-", 68))
	for _, user := range a.store.ListUsers() {
		term.printf("%-24s %-8s %-8t %s\r\n",
			user.Username,
			user.Role,
			user.Active,
			user.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	term.pause("")
}

func (a *App) setRole(term *terminal, role auth.Role) {
	term.clear()
	term.header("Set Role")
	username := term.prompt("Username")
	if err := a.store.SetRole(username, role); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("Role updated.")
}

func (a *App) setActive(term *terminal, active bool) {
	term.clear()
	term.header("Set Active")
	username := term.prompt("Username")
	if err := a.store.SetActive(username, active); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("User updated.")
}

func (a *App) resetPassword(term *terminal) {
	term.clear()
	term.header("Reset Password")
	username := term.prompt("Username")
	password := term.password("New password")
	if err := a.store.ResetPassword(username, password); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("Password reset.")
}

func (a *App) deleteUser(term *terminal) {
	term.clear()
	term.header("Delete User")
	username := term.prompt("Username")
	confirm := term.prompt(fmt.Sprintf("Type %q to confirm", username))
	if strings.TrimSpace(confirm) != strings.TrimSpace(username) {
		term.pause("Delete cancelled.")
		return
	}
	if err := a.store.DeleteUser(username); err != nil {
		term.pause(fmt.Sprintf("Failed: %v", err))
		return
	}
	term.pause("User deleted.")
}

func (t *terminal) menu(title string, options []string) string {
	t.clear()
	t.header(title)
	for i, option := range options {
		t.printf("%d. %s\r\n", i+1, option)
	}
	t.println("")
	return strings.ToLower(t.prompt("Select"))
}

func (t *terminal) header(title string) {
	t.printf("\x1b[1m%s\x1b[0m\r\n", title)
	t.println(strings.Repeat("=", len(title)))
}

func (t *terminal) prompt(label string) string {
	t.printf("%s: ", label)
	line, err := t.readLine(false)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func (t *terminal) password(label string) string {
	t.printf("%s: ", label)
	line, err := t.readLine(true)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func (t *terminal) readLine(secret bool) (string, error) {
	var b strings.Builder
	for {
		select {
		case <-t.ctx.Done():
			return "", t.ctx.Err()
		default:
		}

		ch, err := t.in.ReadByte()
		if err != nil {
			return "", err
		}
		switch ch {
		case '\r', '\n':
			t.println("")
			return b.String(), nil
		case 3, 4:
			return "", io.EOF
		case 8, 127:
			if b.Len() > 0 {
				text := b.String()
				_, size := lastRune(text)
				b.Reset()
				b.WriteString(text[:len(text)-size])
				t.printf("\b \b")
			}
		default:
			if ch < 32 {
				continue
			}
			b.WriteByte(ch)
			if secret {
				t.printf("*")
			} else {
				t.printf("%c", ch)
			}
		}
	}
}

func lastRune(s string) (rune, int) {
	if s == "" {
		return 0, 0
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i]&0xc0 != 0x80 {
			return rune(s[i]), len(s) - i
		}
	}
	return 0, 1
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

type liveDraft struct {
	mu    sync.RWMutex
	value string
}

func (d *liveDraft) String() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.value
}

func (d *liveDraft) appendByte(ch byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.value += string([]byte{ch})
}

func (d *liveDraft) backspace() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.value == "" {
		return
	}
	_, size := lastRune(d.value)
	d.value = d.value[:len(d.value)-size]
}

func (d *liveDraft) clear() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	value := d.value
	d.value = ""
	return value
}

func readLiveLine(term *terminal, draft *liveDraft, changed chan<- struct{}) (string, error) {
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}

	for {
		select {
		case <-term.ctx.Done():
			return "", term.ctx.Err()
		default:
		}

		ch, err := term.in.ReadByte()
		if err != nil {
			return "", err
		}
		switch ch {
		case '\r', '\n':
			line := draft.clear()
			notify()
			return line, nil
		case 3, 4:
			return "", io.EOF
		case 8, 127:
			draft.backspace()
			notify()
		default:
			if ch < 32 {
				continue
			}
			draft.appendByte(ch)
			notify()
		}
	}
}

func (t *terminal) pause(message string) {
	if message != "" {
		t.println("")
		t.println(message)
	}
	t.println("")
	t.prompt("Press Enter to continue")
}

func (t *terminal) clear() {
	t.printf("\x1b[2J\x1b[H")
}

func (t *terminal) println(text string) {
	t.printf("%s\r\n", text)
}

func (t *terminal) printf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.out, format, args...)
}

func (t *terminal) write(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprint(t.out, text)
}
