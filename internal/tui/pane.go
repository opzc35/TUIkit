package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type paneID int

type Pane struct {
	id      paneID
	screen  screen
	focused bool
}

type splitDir int

const (
	splitHorizontal splitDir = iota
	splitVertical
)

// Layout is a tree: either a single pane or a split containing two sub-layouts.
type Layout struct {
	pane    *Pane      // leaf node
	dir     splitDir   // split direction (only if split)
	left    *Layout    // first sub-layout (only if split)
	right   *Layout    // second sub-layout (only if split)
	ratio   float64    // left/right size ratio, default 0.5
}

func singlePane(pane *Pane) *Layout {
	return &Layout{pane: pane}
}

func splitLayout(dir splitDir, left, right *Layout, ratio float64) *Layout {
	return &Layout{dir: dir, left: left, right: right, ratio: ratio}
}

// FindPane returns the pane with the given ID.
func (l *Layout) FindPane(id paneID) *Pane {
	if l.pane != nil {
		if l.pane.id == id {
			return l.pane
		}
		return nil
	}
	if p := l.left.FindPane(id); p != nil {
		return p
	}
	return l.right.FindPane(id)
}

// FocusedPane returns the currently focused pane.
func (l *Layout) FocusedPane() *Pane {
	if l.pane != nil {
		if l.pane.focused {
			return l.pane
		}
		return nil
	}
	if p := l.left.FocusedPane(); p != nil {
		return p
	}
	return l.right.FocusedPane()
}

// AllPanes returns all panes in the layout.
func (l *Layout) AllPanes() []*Pane {
	if l.pane != nil {
		return []*Pane{l.pane}
	}
	return append(l.left.AllPanes(), l.right.AllPanes()...)
}

// SetFocus sets only the pane with the given ID as focused.
func (l *Layout) SetFocus(id paneID) {
	for _, p := range l.AllPanes() {
		p.focused = p.id == id
	}
}

// RemovePane removes the pane with the given ID and collapses the layout.
// Returns the new layout (may be nil if last pane removed).
func (l *Layout) RemovePane(id paneID) *Layout {
	if l.pane != nil {
		if l.pane.id == id {
			return nil
		}
		return l
	}

	newLeft := l.left.RemovePane(id)
	newRight := l.right.RemovePane(id)

	if newLeft == nil && newRight == nil {
		return nil
	}
	if newLeft == nil {
		return newRight
	}
	if newRight == nil {
		return newLeft
	}
	return splitLayout(l.dir, newLeft, newRight, l.ratio)
}

// SplitFocused splits the focused pane, adding a new pane next to it.
func (l *Layout) SplitFocused(newPane *Pane, dir splitDir) {
	focused := l.FocusedPane()
	if focused == nil {
		return
	}

	l.splitPane(focused.id, newPane, dir)
}

func (l *Layout) splitPane(targetID paneID, newPane *Pane, dir splitDir) {
	if l.pane != nil {
		if l.pane.id == targetID {
			l.pane.focused = false
			newPane.focused = true
			left := singlePane(l.pane)
			right := singlePane(newPane)
			l.pane = nil
			l.dir = dir
			l.left = left
			l.right = right
			l.ratio = 0.5
		}
		return
	}
	l.left.splitPane(targetID, newPane, dir)
	l.right.splitPane(targetID, newPane, dir)
}

// NextPane returns the next pane ID in the layout order.
func (l *Layout) NextPane(current paneID) paneID {
	panes := l.AllPanes()
	for i, p := range panes {
		if p.id == current && i < len(panes)-1 {
			return panes[i+1].id
		}
	}
	return panes[0].id
}

// PrevPane returns the previous pane ID in the layout order.
func (l *Layout) PrevPane(current paneID) paneID {
	panes := l.AllPanes()
	for i, p := range panes {
		if p.id == current && i > 0 {
			return panes[i-1].id
		}
	}
	return panes[len(panes)-1].id
}

// NeighbourPanes returns panes adjacent to the focused one based on split direction.
func (l *Layout) NeighbourPanes(focusedID paneID, dir splitDir) []paneID {
	var neighbours []paneID
	l.collectNeighbours(focusedID, dir, &neighbours)
	return neighbours
}

func (l *Layout) collectNeighbours(focusedID paneID, dir splitDir, result *[]paneID) {
	if l.pane != nil {
		return
	}
	if l.dir == dir {
		// In a matching split, panes on the other side are neighbours
		if l.left.Contains(focusedID) {
			for _, p := range l.right.AllPanes() {
				*result = append(*result, p.id)
			}
		} else if l.right.Contains(focusedID) {
			for _, p := range l.left.AllPanes() {
				*result = append(*result, p.id)
			}
		} else {
			l.left.collectNeighbours(focusedID, dir, result)
			l.right.collectNeighbours(focusedID, dir, result)
		}
	} else {
		l.left.collectNeighbours(focusedID, dir, result)
		l.right.collectNeighbours(focusedID, dir, result)
	}
}

func (l *Layout) Contains(id paneID) bool {
	return l.FindPane(id) != nil
}

// Render renders the layout tree into a string.
func (l *Layout) Render(width, height int, views map[paneID]string) string {
	if l.pane != nil {
		view := views[l.pane.id]
		borderStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
		if l.pane.focused {
			borderStyle = borderStyle.BorderForeground(primaryColor)
		} else {
			borderStyle = borderStyle.BorderForeground(borderColor)
		}
		return borderStyle.Width(width - 2).Height(height - 2).Render(view)
	}

	if l.dir == splitHorizontal {
		leftW := int(float64(width) * l.ratio)
		rightW := width - leftW
		leftView := l.left.Render(leftW, height, views)
		rightView := l.right.Render(rightW, height, views)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	}

	topH := int(float64(height) * l.ratio)
	bottomH := height - topH
	topView := l.left.Render(width, topH, views)
	bottomView := l.right.Render(width, bottomH, views)
	return lipgloss.JoinVertical(lipgloss.Left, topView, bottomView)
}

// PaneDimensions returns the allocated (width, height) for each pane.
func (l *Layout) PaneDimensions(width, height int) map[paneID][2]int {
	if l.pane != nil {
		return map[paneID][2]int{l.pane.id: {width, height}}
	}

	dims := map[paneID][2]int{}
	if l.dir == splitHorizontal {
		leftW := int(float64(width) * l.ratio)
		rightW := width - leftW
		for id, dim := range l.left.PaneDimensions(leftW, height) {
			dims[id] = dim
		}
		for id, dim := range l.right.PaneDimensions(rightW, height) {
			dims[id] = dim
		}
	} else {
		topH := int(float64(height) * l.ratio)
		bottomH := height - topH
		for id, dim := range l.left.PaneDimensions(width, topH) {
			dims[id] = dim
		}
		for id, dim := range l.right.PaneDimensions(width, bottomH) {
			dims[id] = dim
		}
	}
	return dims
}

// PaneCount returns the number of panes.
func (l *Layout) PaneCount() int {
	if l.pane != nil {
		return 1
	}
	return l.left.PaneCount() + l.right.PaneCount()
}

// ReplacePane replaces the pane with the given ID with a new pane.
func (l *Layout) ReplacePane(targetID paneID, newPane *Pane) {
	if l.pane != nil {
		if l.pane.id == targetID {
			l.pane = newPane
		}
		return
	}
	l.left.ReplacePane(targetID, newPane)
	l.right.ReplacePane(targetID, newPane)
}

// StatusLine returns a status line showing all panes.
func (l *Layout) StatusLine() string {
	panes := l.AllPanes()
	var parts []string
	for _, p := range panes {
		label := fmt.Sprintf("%d:%s", p.id, screenLabel(p.screen))
		if p.focused {
			parts = append(parts, primaryColorStyle.Render("["+label+"]"))
		} else {
			parts = append(parts, dimStyle.Render("["+label+"]"))
		}
	}
	return strings.Join(parts, " ")
}

var primaryColorStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)

func screenLabel(s screen) string {
	switch s {
	case screenMainMenu:
		return "Menu"
	case screenLogin:
		return "Login"
	case screenRegister:
		return "Register"
	case screenDashboard:
		return "Dashboard"
	case screenChat:
		return "Chat"
	case screenProfile:
		return "Profile"
	case screenAdmin:
		return "Admin"
	case screenCheckIn:
		return "CheckIn"
	case screenAPIUser:
		return "API"
	default:
		return "?"
	}
}