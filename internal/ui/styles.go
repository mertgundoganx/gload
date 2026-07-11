package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("#7C3AED")
	colorSecondary = lipgloss.Color("#06B6D4")
	colorSuccess   = lipgloss.Color("#10B981")
	colorError     = lipgloss.Color("#EF4444")
	colorWarning   = lipgloss.Color("#F59E0B")
	colorMuted     = lipgloss.Color("#6B7280")
	colorText      = lipgloss.Color("#F9FAFB")
	colorDim       = lipgloss.Color("#9CA3AF")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(14)

	valueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSuccess)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorError)

	warningStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning)

	barFilledStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	barEmptyStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true)
)
