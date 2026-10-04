package tui

import (
	"math/big"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent = lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"}
	muted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B95A5"}
	danger = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}

	accentStyle = lipgloss.NewStyle().Foreground(accent)
	boldStyle   = lipgloss.NewStyle().Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(muted)
	errorStyle  = lipgloss.NewStyle().Foreground(danger)
	inputBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
)

const banner = ` ▄▀█ █▀▀ █▀▀ █▀█ █▀▀ █▀▄
 █▀█ █▄▄ █▄▄ █▀▄ ██▄ █▄▀`

// trimDecimal shortens a decimal string to at most places digits after the point.
func trimDecimal(value string, places int) string {
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return value
	}
	s := r.FloatString(places)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}
