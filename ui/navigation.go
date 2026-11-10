package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Keep whole shortcuts together and center every row independently. Very narrow
// terminals may need more than two rows; no shortcut is silently dropped.
func renderHints(width int, pairs ...[2]string) string {
	width = max(1, width)
	sep := helpSepSty.Render(" • ")
	rows := []string{}
	line := ""
	for _, p := range pairs {
		part := helpKeySty.Render(p[0]) + " " + helpDescSty.Render(p[1])
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(part) > width {
			rows = append(rows, line)
			line = ""
		}
		if line != "" {
			line += sep
		}
		if lipgloss.Width(part) > width {
			if line != "" {
				rows = append(rows, line)
				line = ""
			}
			rows = append(rows, strings.Split(ansi.Hardwrap(part, width, true), "\n")...)
		} else {
			line += part
		}
	}
	if line != "" {
		rows = append(rows, line)
	}
	for i, row := range rows {
		rows[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, row)
	}
	return strings.Join(rows, "\n")
}
func contentWidth(width int) int {
	h, _ := docStyle.GetFrameSize()
	if width <= 0 {
		return 100
	}
	return max(1, width-h)
}

func pageHeader(width int, title, subtitle string) string {
	w := contentWidth(width)
	header := lipgloss.PlaceHorizontal(w, lipgloss.Center, headerBarSty.Render(ansi.Truncate(title, max(1, w-4), "…")))
	if subtitle != "" {
		for _, line := range strings.Split(ansi.Wrap(subtitle, w, ""), "\n") {
			header += "\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, line)
		}
	}
	return header + "\n"
}
func pageBodyHeight(width, height int, subtitle, hints string) int {
	_, frame := docStyle.GetFrameSize()
	return max(1, height-frame-lipgloss.Height(pageHeader(width, "", subtitle))-lipgloss.Height(hints)-1)
}

// A bounded content area keeps navigation at the bottom at any window size.
func pageView(width, height int, title, subtitle, body, hints string) tea.View {
	w := contentWidth(width)
	header := pageHeader(width, title, subtitle)
	room := pageBodyHeight(width, height, subtitle, hints)
	lines := strings.Split(body, "\n")
	if len(lines) > room && room > 1 {
		hidden := len(lines) - (room - 1)
		lines = append(lines[:room-1], mutedSty.Render(fmt.Sprintf("… %d more — scroll or grow the terminal", hidden)))
	} else if len(lines) > room {
		lines = lines[:room]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "…")
	}
	body = strings.Join(lines, "\n")
	_, frame := docStyle.GetFrameSize()
	spacer := max(1, height-frame-lipgloss.Height(header)-lipgloss.Height(body)-lipgloss.Height(hints))
	v := tea.NewView(docStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, strings.Repeat("\n", spacer-1), hints)))
	v.AltScreen = true
	return v
}
