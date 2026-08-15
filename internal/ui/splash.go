package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// bannerSmall is the startup splash wordmark (figlet "small" font).
const bannerSmall = ` _                   _                 _
| |_ _ _ ___ ___ ___| |_ _ _ _  _ _ _ | |__
|  _| '_/ -_) -_)___|  _| '_| || | ' \| / /
 \__|_| \___\___|    \__|_|  \_,_|_||_|_\_\`

// renderSplash draws the centered startup banner shown while the first scan
// runs. It is dismissed as soon as the first repo appears (or the scan
// finishes with no repos).
func (m appModel) renderSplash() string {
	banner := lipgloss.NewStyle().Bold(true).Foreground(g.accentColor).Render(bannerSmall)
	sub := g.dim.Render(m.spinner.View() + " finding your repos…")

	content := lipgloss.JoinVertical(lipgloss.Center, banner, "", sub)
	if m.width <= 0 || m.height <= 0 {
		return content
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
