package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opzc35/tuikit/internal/auth"
)

type checkinModel struct {
	store      *auth.Store
	user       auth.User
	result     auth.CheckInResult
	ranking    []auth.RankingEntry
	checkedIn  bool
	width      int
	height     int
}

func newCheckIn(store *auth.Store) checkinModel {
	return checkinModel{
		store: store,
	}
}

func (m checkinModel) Init() tea.Cmd {
	return nil
}

func (m checkinModel) Update(msg tea.Msg) (checkinModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case userLoginMsg:
		m.user = auth.User(msg)
		m.ranking = m.store.GetCheckInRanking(10)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.checkedIn {
				result, err := m.store.CheckIn(m.user.Username)
				if err == nil {
					m.result = result
					m.checkedIn = result.Success
					if updated, ok := m.store.GetUser(m.user.Username); ok {
						m.user = updated
					}
					m.ranking = m.store.GetCheckInRanking(10)
				}
			}
			return m, nil
		case "esc", "q":
			return m, func() tea.Msg { return navigateMsg(screenDashboard) }
		}
	}

	return m, nil
}

func (m checkinModel) View() string {
	boxWidth := m.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	var content string

	if m.checkedIn {
		content = lipgloss.JoinVertical(lipgloss.Left,
			successStyle.Render("✓ "+m.result.Message),
			"",
			fmt.Sprintf("%s %d", labelStyle.Render("总积分:"), m.result.TotalPoints),
			fmt.Sprintf("%s %d 天", labelStyle.Render("连续签到:"), m.result.ConsecutiveDays),
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("每日签到"),
			"",
			fmt.Sprintf("%s %d", labelStyle.Render("当前积分:"), m.user.CheckInPoints),
			"",
			dimStyle.Render("签到规则:"),
			"  • 基础积分: 10 分",
			"  • 连续签到奖励: 每天 +2 分",
			"  • 每日最高: 30 分",
			"",
			"",
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 2).
				Render("按 Enter 签到"),
		)
	}

	if m.result.Message != "" && !m.checkedIn {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			warningStyle.Render(m.result.Message),
		)
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		"",
		titleStyle.Render("积分排行榜"),
		"",
	)

	if len(m.ranking) == 0 {
		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			dimStyle.Render("  暂无签到记录"),
		)
	} else {
		for _, entry := range m.ranking {
			var rankStyle lipgloss.Style
			switch entry.Rank {
			case 1:
				rankStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
			case 2:
				rankStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C0C0C0")).Bold(true)
			case 3:
				rankStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#CD7F32")).Bold(true)
			default:
				rankStyle = dimStyle
			}

			line := fmt.Sprintf("  %s %-15s %d 分",
				rankStyle.Render(fmt.Sprintf("#%d", entry.Rank)),
				entry.Username,
				entry.Points,
			)

			if entry.Username == m.user.Username {
				line = primaryColorStyle.Render(line)
			}

			content = lipgloss.JoinVertical(lipgloss.Left, content, line)
		}
	}

	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		dimStyle.Render("按 Esc 或 q 返回"),
	)

	return Box("签到", content, boxWidth)
}