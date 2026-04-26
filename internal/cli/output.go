package cli

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// IsTTY reports whether stdout is an interactive terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Styles holds the lipgloss styles used across commands.
var Styles = struct {
	// Category indicators
	Good    lipgloss.Style
	Warning lipgloss.Style
	Danger  lipgloss.Style
	Dim     lipgloss.Style
	Bold    lipgloss.Style

	// Table elements
	Header    lipgloss.Style
	Cell      lipgloss.Style
	SizeCell  lipgloss.Style
	RightCell lipgloss.Style

	// Misc
	Title   lipgloss.Style
	Section lipgloss.Style
}{
	Good:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")), // green
	Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // yellow
	Danger:  lipgloss.NewStyle().Foreground(lipgloss.Color("1")), // red
	Dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")), // dark gray
	Bold:    lipgloss.NewStyle().Bold(true),

	Header:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
	Cell:      lipgloss.NewStyle(),
	SizeCell:  lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	RightCell: lipgloss.NewStyle().Align(lipgloss.Right),

	Title:   lipgloss.NewStyle().Bold(true).Underline(true),
	Section: lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
}

// CategoryStyle returns a style appropriate for a given category string.
func CategoryStyle(cat string) lipgloss.Style {
	switch cat {
	case "library_seeding":
		return Styles.Good
	case "library_only":
		return Styles.Good
	case "seeding_only":
		return Styles.Warning
	case "orphan":
		return Styles.Danger
	default:
		return Styles.Cell
	}
}

// CategoryIndicator returns a single unicode indicator for a category.
func CategoryIndicator(cat string) string {
	switch cat {
	case "library_seeding", "library_only":
		return "✓"
	case "seeding_only":
		return "↑"
	case "orphan":
		return "⚠"
	default:
		return "?"
	}
}

// CategoryLabel returns a display-friendly label for a category.
func CategoryLabel(cat string) string {
	switch cat {
	case "library_seeding":
		return "library+seeding"
	case "library_only":
		return "library-only"
	case "seeding_only":
		return "seeding-only"
	case "orphan":
		return "orphan"
	default:
		return cat
	}
}

// CategoryDescription returns a short description shown in the report summary.
func CategoryDescription(cat string) string {
	switch cat {
	case "library_seeding":
		return "doing double duty"
	case "library_only":
		return "in library"
	case "seeding_only":
		return "active torrents"
	case "orphan":
		return "candidates for deletion"
	default:
		return ""
	}
}
