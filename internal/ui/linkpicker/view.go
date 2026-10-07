package linkpicker

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

// ViewOverlay renders the picker centered on a dimmed copy of
// background. Returns background unchanged when not visible.
func (m *Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	m.termHeight = termHeight
	box := m.renderBox(termWidth)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

func (m *Model) renderBox(termWidth int) string {
	overlayWidth := termWidth * 6 / 10
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 80 {
		overlayWidth = 80
	}
	if overlayWidth > termWidth-2 {
		overlayWidth = termWidth - 2
	}
	overlayWidth = m.widenForRows(overlayWidth, termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	bg := styles.Background
	title := lipgloss.NewStyle().
		Bold(true).
		Background(bg).
		Foreground(styles.Primary).
		Render(m.title)
	title = m.withTitleStatus(title, innerWidth)

	badgeStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.Accent)
	mutedStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted)

	var rows []string
	for _, i := range m.window() {
		it := m.items[i]
		text := m.drawnText(it, innerWidth)
		badge := ""
		if it.InApp {
			badge = " [slk]"
		}
		budget := innerWidth - 1 - lipgloss.Width(badge) // 1 = indicator column
		budget -= lipgloss.Width(m.checkbox(i))
		side := m.sideColumn(it, innerWidth)
		budget -= lipgloss.Width(side)
		if budget < 1 {
			budget = 1
		}
		if lipgloss.Width(text) > budget {
			text = cellwidth.Cut(text, budget)
		}
		// Detail rides muted in whatever space the main text leaves;
		// dropped entirely when the row is too tight for it to help.
		detail := it.Detail
		detailBudget := budget - lipgloss.Width(text) - 2
		if detail != "" && detailBudget >= 4 {
			if lipgloss.Width(detail) > detailBudget {
				detail = cellwidth.Cut(detail, detailBudget)
			}
		} else {
			detail = ""
		}
		mainStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
		indicator := " "
		if i == m.selected {
			mainStyle = mainStyle.Foreground(styles.Primary).Bold(true)
			indicator = lipgloss.NewStyle().Background(bg).Foreground(styles.Accent).Render("\u258c")
		}
		indicator += m.checkbox(i)
		row := indicator + mainStyle.Render(text)
		used := lipgloss.Width(text)
		if detail != "" {
			row += mutedStyle.Render("  " + detail)
			used += 2 + lipgloss.Width(detail)
		}
		if used < budget {
			row += mainStyle.Render(strings.Repeat(" ", budget-used))
		}
		rows = append(rows, row+side+badgeStyle.Render(badge))
	}

	footer := lipgloss.NewStyle().
		Background(bg).
		Foreground(styles.TextMuted).
		Render(m.footerText(innerWidth))

	content := title + "\n" + m.filterLine() + "\n" + strings.Join(rows, "\n") + "\n\n" + footer
	content = messages.ReapplyBgAfterResets(content, messages.BgANSI()+messages.FgANSI())

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(bg).
		Background(bg).
		Padding(1, 1).
		Width(overlayWidth).
		Render(content)
}
