package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/sshserver"
)

type App struct {
	store *auth.Store
}

type terminal struct {
	ctx context.Context
	in  *bufio.Reader
	out io.Writer
}

func New(store *auth.Store) *App {
	return &App{store: store}
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

		options := []string{"Profile", "Logout"}
		if user.Role == auth.RoleAdmin {
			options = []string{"Profile", "Admin", "Logout"}
		}

		choice := term.menu("User Menu", options)
		switch {
		case choice == "1":
			a.profile(term, user)
		case user.Role == auth.RoleAdmin && choice == "2":
			a.adminMenu(term)
		case (user.Role == auth.RoleAdmin && choice == "3") || (user.Role != auth.RoleAdmin && choice == "2"):
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

func (a *App) adminMenu(term *terminal) {
	for {
		switch term.menu("Admin", []string{
			"List users",
			"Promote user",
			"Demote user",
			"Enable user",
			"Disable user",
			"Reset password",
			"Delete user",
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
			return
		default:
			term.pause("Invalid choice.")
		}
	}
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
	fmt.Fprintf(t.out, format, args...)
}
