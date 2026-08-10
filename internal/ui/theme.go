package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is the color palette (04-tui-layout.md §8). All named styles are
// derived from it so `theme.overrides` and NO_COLOR apply everywhere.
type Theme struct {
	Normal        lipgloss.Color
	Dim           lipgloss.Color
	Accent        lipgloss.Color
	Dirty         lipgloss.Color
	Conflict      lipgloss.Color
	Clean         lipgloss.Color
	WorktreeChild lipgloss.Color
	Selection     lipgloss.Color // background for the focused row
	DiffHunk      lipgloss.Color // @@ hunk-header color
}

// DefaultTheme is the built-in palette (dark-first; light variant swaps a
// few hues for contrast).
func DefaultTheme() Theme {
	return Theme{
		Normal:        lipgloss.Color("252"),
		Dim:           lipgloss.Color("245"),
		Accent:        lipgloss.Color("205"),
		Dirty:         lipgloss.Color("220"),
		Conflict:      lipgloss.Color("196"),
		Clean:         lipgloss.Color("34"),
		WorktreeChild: lipgloss.Color("110"),
		Selection:     lipgloss.Color("237"),
		DiffHunk:      lipgloss.Color("44"),
	}
}

// LightTheme is used when theme.variant = "light".
func LightTheme() Theme {
	t := DefaultTheme()
	t.Normal = lipgloss.Color("16")
	t.Dim = lipgloss.Color("59")
	t.Conflict = lipgloss.Color("124")
	t.Clean = lipgloss.Color("28")
	t.Dirty = lipgloss.Color("94")
	t.Selection = lipgloss.Color("253")
	return t
}

// styles bundles the theme-derived styles used across views.
type styles struct {
	dim         lipgloss.Style
	accent      lipgloss.Style
	dimColor    lipgloss.Color
	accentColor lipgloss.Color
	borderColor lipgloss.Color
	tabActiveFg lipgloss.Color
	conflict    lipgloss.Style
	staged      lipgloss.Style
	unstaged    lipgloss.Style
	untracked   lipgloss.Style
	selColor    lipgloss.Color // selection background color
	title       lipgloss.Style

	diffAdd     lipgloss.Style
	diffDel     lipgloss.Style
	diffHunk    lipgloss.Style
	diffHeader  lipgloss.Style
	tabActive   lipgloss.Style
	tabInactive lipgloss.Style
	selBar      lipgloss.Style
}

// palette keys accepted in theme.overrides (09-config.md §1).
var overrideKeys = map[string]*lipgloss.Color{
	"normal":         nil,
	"dim":            nil,
	"accent":         nil,
	"dirty":          nil,
	"conflict":       nil,
	"clean":          nil,
	"worktree_child": nil,
}

// buildStyles derives the style set from a Theme. When noColor is set (NO_COLOR
// or a non-color terminal), lipgloss renders plain.
func buildStyles(t Theme, noColor bool) styles {
	mk := func(c lipgloss.Color) lipgloss.Style {
		s := lipgloss.NewStyle()
		if !noColor {
			s = s.Foreground(c)
		}
		return s
	}
	return styles{
		dim:         mk(t.Dim),
		accent:      mk(t.Accent),
		dimColor:    t.Dim,
		accentColor: t.Accent,
		borderColor: t.Dim,
		tabActiveFg: lipgloss.Color("0"),
		conflict:    mk(t.Conflict),
		staged:      mk(t.Clean),
		unstaged:    mk(t.Dirty),
		untracked:   mk(t.Dim),
		selColor:    t.Selection,
		title:       lipgloss.NewStyle().Bold(true),

		diffAdd:    mk(t.Clean),
		diffDel:    mk(t.Conflict),
		diffHunk:   mk(t.DiffHunk),
		diffHeader: lipgloss.NewStyle().Bold(true).Foreground(t.Dim),

		tabActive:   activeTab(t, noColor),
		tabInactive: mk(t.Dim).Padding(0, 1),
		selBar:      mk(t.Accent),
	}
}

// activeTab is the highlighted tab pill (accent background, dark bold text).
func activeTab(t Theme, noColor bool) lipgloss.Style {
	s := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	if !noColor {
		s = s.Background(t.Accent).Foreground(lipgloss.Color("0"))
	} else {
		s = s.Reverse(true)
	}
	return s
}

// applyOverrides mutates a theme from theme.overrides hex colors.
func applyOverrides(t *Theme, overrides map[string]string) {
	if overrides == nil {
		return
	}
	set := func(c *lipgloss.Color, key string) {
		if v, ok := overrides[key]; ok && v != "" {
			*c = lipgloss.Color(v)
		}
	}
	set(&t.Normal, "normal")
	set(&t.Dim, "dim")
	set(&t.Accent, "accent")
	set(&t.Dirty, "dirty")
	set(&t.Conflict, "conflict")
	set(&t.Clean, "clean")
	set(&t.WorktreeChild, "worktree_child")
}

// noColorEnv reports whether NO_COLOR is set (https://no-color.org).
func noColorEnv() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// globals holds the current theme-derived styles. Set once at app start via
// initStyles; views read through the accessor functions below.
var g styles = buildStyles(DefaultTheme(), false)

// initStyles builds the global style set from the config theme.
func initStyles(theme Theme, overrides map[string]string) {
	t := theme
	applyOverrides(&t, overrides)
	g = buildStyles(t, noColorEnv())
}

// newBorder returns a rounded-border style (shared by modals).
func newBorder() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
}

// hrule renders a horizontal rule of the given width (used between app
// sections; lipgloss borders need an explicit content width to render).
func hrule(width int) string {
	if width <= 0 {
		width = 40
	}
	return strings.Repeat("─", width)
}
