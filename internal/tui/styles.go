package tui

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	primaryColor = lipgloss.Color("#7aa2f7")
	successColor = lipgloss.Color("#9ece6a")
	warningColor = lipgloss.Color("#e0af68")
	errorColor   = lipgloss.Color("#f7768e")
	dimColor     = lipgloss.Color("#565f89")
	borderColor  = lipgloss.Color("#414868")

	// Base styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#414868")).
			Padding(0, 1)

	// Menu styles
	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#c0caf5"))

	selectedMenuItemStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(primaryColor).
				Bold(true)

	// Form styles
	labelStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	// Message styles
	messageAuthorStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	messageTimeStyle = lipgloss.NewStyle().
				Foreground(dimColor)

	messageBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#c0caf5"))

	deletedMessageStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Italic(true)

	// Status styles
	successStyle = lipgloss.NewStyle().
			Foreground(successColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)
)

// Box creates a styled box with title and content
func Box(title, content string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(width).
		Render(
			lipgloss.JoinVertical(lipgloss.Left,
				titleStyle.Render(title),
				content,
			),
		)
}

// SplitScreen creates a two-column layout
func SplitScreen(sidebar, content string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
}

// ClockBlock returns a styled time display block for sidebars
func ClockBlock() string {
	now := time.Now()
	return lipgloss.JoinVertical(lipgloss.Left,
		dimStyle.Render("Time"),
		"  "+now.Format("15:04:05"),
		"  "+now.Format("2006-01-02"),
	)
}