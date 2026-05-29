package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/michaelpeterswa/bigpictures.company/internal/pipeline"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	phaseStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// ProgressModel renders two progress bars (tile + upload), a spinner, and the
// current phase. Pipeline work happens in a parent goroutine that uses
// tea.Program.Send to feed messages into the model.
type ProgressModel struct {
	title       string
	tile        progress.Model
	upload      progress.Model
	spinner     spinner.Model
	phase       pipeline.Phase
	tilePct     int
	upDone      int
	upTotal     int
	finished    bool
	finalResult *pipeline.Result
	finalErr    error
}

func NewProgressModel(title string) ProgressModel {
	t := progress.New(progress.WithDefaultGradient(), progress.WithWidth(40))
	u := progress.New(progress.WithDefaultGradient(), progress.WithWidth(40))
	s := spinner.New()
	s.Spinner = spinner.Dot
	return ProgressModel{
		title:   title,
		tile:    t,
		upload:  u,
		spinner: s,
		phase:   pipeline.PhaseValidate,
	}
}

func (m ProgressModel) Init() tea.Cmd { return m.spinner.Tick }

// Finished reports whether the model has reached a terminal state. The
// caller's bubbletea program should be quit by sending tea.Quit once Finished
// returns true; we don't auto-quit here so the caller can decide how to wrap
// up (print summary, exit code, etc.).
func (m ProgressModel) Finished() bool          { return m.finished }
func (m ProgressModel) Result() *pipeline.Result { return m.finalResult }
func (m ProgressModel) Err() error               { return m.finalErr }

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.finished = true
			m.finalErr = fmt.Errorf("interrupted")
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case PhaseMsg:
		m.phase = msg.Phase
		return m, nil
	case TileProgressMsg:
		m.tilePct = msg.Progress.Percent
		cmd := m.tile.SetPercent(float64(m.tilePct) / 100.0)
		return m, cmd
	case UploadProgressMsg:
		m.upDone = msg.Progress.UploadedFiles
		m.upTotal = msg.Progress.TotalFiles
		pct := 0.0
		if m.upTotal > 0 {
			pct = float64(m.upDone) / float64(m.upTotal)
		}
		cmd := m.upload.SetPercent(pct)
		return m, cmd
	case DoneMsg:
		m.finished = true
		m.finalResult = msg.Result
		return m, tea.Quit
	case ErrMsg:
		m.finished = true
		m.finalErr = msg.Err
		return m, tea.Quit
	case progress.FrameMsg:
		// Forward animation frames to both bars.
		t, cmd1 := m.tile.Update(msg)
		u, cmd2 := m.upload.Update(msg)
		m.tile = t.(progress.Model)
		m.upload = u.(progress.Model)
		return m, tea.Batch(cmd1, cmd2)
	}
	return m, nil
}

func (m ProgressModel) View() string {
	header := titleStyle.Render(m.title)
	if m.finished {
		if m.finalErr != nil {
			return header + "\n" + errorStyle.Render("error: "+m.finalErr.Error()) + "\n"
		}
		return header + "\n" + doneStyle.Render("done") + "\n"
	}
	phase := fmt.Sprintf("%s %s",
		m.spinner.View(),
		phaseStyle.Render(string(m.phase)),
	)
	tileRow := fmt.Sprintf("  tile  %s %s",
		m.tile.View(),
		dimStyle.Render(fmt.Sprintf("%3d%%", m.tilePct)),
	)
	upRow := fmt.Sprintf("  ship  %s %s",
		m.upload.View(),
		dimStyle.Render(fmt.Sprintf("%d/%d", m.upDone, m.upTotal)),
	)
	return header + "\n" + phase + "\n" + tileRow + "\n" + upRow + "\n"
}
