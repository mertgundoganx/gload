package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mertgundoganx/gload/internal/metrics"
	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/pkg/config"
)

type tickMsg time.Time
type doneMsg struct{}

type model struct {
	cfg      *config.Config
	runner   *runner.Runner
	snapshot metrics.Snapshot
	done     bool
	cancel   context.CancelFunc
}

func NewModel(cfg *config.Config, r *runner.Runner, cancel context.CancelFunc) model {
	return model{
		cfg:    cfg,
		runner: r,
		cancel: cancel,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), waitForDone(m.runner))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}

	case tickMsg:
		m.snapshot = m.runner.Metrics.Snapshot()
		return m, tickCmd()

	case doneMsg:
		m.snapshot = m.runner.Metrics.Snapshot()
		m.done = true
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	return renderView(m)
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForDone(r *runner.Runner) tea.Cmd {
	return func() tea.Msg {
		<-r.Done
		return doneMsg{}
	}
}
