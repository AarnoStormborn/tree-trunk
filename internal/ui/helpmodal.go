package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// helpModal is the filterable keybinding cheatsheet (04-tui-layout.md §3:
// lazygit `?` + herdr filterable cheatsheet). Typing filters; `/` focuses
// the filter; `?`/esc closes.
type helpModal struct {
	filter textinput.Model
	rows   []helpRow // pre-rendered binding lines
}

type helpRow struct {
	line string
}

func (h helpModal) render(width int) string {
	query := h.filter.Value()
	var b strings.Builder
	b.WriteString(g.title.Render("keybindings") + "\n\n")
	count := 0
	for _, r := range h.rows {
		if query == "" || strings.Contains(strings.ToLower(r.line), strings.ToLower(query)) {
			b.WriteString(r.line)
			b.WriteString("\n")
			count++
		}
	}
	if count == 0 {
		b.WriteString(g.dim.Render("no matches"))
		b.WriteString("\n")
	}
	b.WriteString("\n" + g.dim.Render("/ filter · ?/esc close"))
	// Filter prompt line.
	prompt := "filter: "
	if h.filter.Focused() {
		prompt += h.filter.View()
	} else {
		prompt += g.dim.Render(h.filter.Value() + " (press / to type)")
	}
	b.WriteString("\n\n" + prompt)

	return lipglossBox(b.String(), width)
}

func newHelpModal(bindings [][]key.Binding) *helpModal {
	f := textinput.New()
	f.Placeholder = "filter bindings…"
	h := &helpModal{filter: f}
	seen := map[string]bool{}
	for _, group := range bindings {
		for _, kb := range group {
			if kb.Help().Key == "" {
				continue
			}
			line := kb.Help().Key + "  " + g.dim.Render(kb.Help().Desc)
			if !seen[line] {
				seen[line] = true
				h.rows = append(h.rows, helpRow{line: line})
			}
		}
	}
	sort.Slice(h.rows, func(i, j int) bool { return h.rows[i].line < h.rows[j].line })
	return h
}

// handleKey processes cheatsheet keys: typing edits the filter, / focuses it,
// ?/esc closes, enter closes.
func (h *helpModal) handleKey(msg tea.KeyMsg) (close bool) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "?"))):
		return true
	case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
		h.filter.Focus()
		return false
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return true
	default:
		if h.filter.Focused() {
			var cmd tea.Cmd
			h.filter, cmd = h.filter.Update(msg)
			_ = cmd
		}
		return false
	}
}

func lipglossBox(content string, width int) string {
	if width <= 0 {
		width = 72
	}
	return newBorder().Width(width).Render(content)
}

var _ = tea.KeyMsg{}
