// Package tui contains the Bubble Tea models, lipgloss styles, and huh forms
// shown by the `pano` CLI when stdout/stderr is a TTY. Non-TTY callers get
// plain text fallbacks; nothing in this package should run when piping output
// into a file or another command.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/michaelpeterswa/bigpictures.company/internal/pipeline"
	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
)

// PhaseMsg announces a stage transition.
type PhaseMsg struct{ Phase pipeline.Phase }

// TileProgressMsg is a tiling % event.
type TileProgressMsg struct{ Progress tile.Progress }

// UploadProgressMsg reports cumulative directory-upload progress.
type UploadProgressMsg struct{ Progress pipeline.UploadProgress }

// DoneMsg fires when the pipeline returns successfully.
type DoneMsg struct{ Result *pipeline.Result }

// ErrMsg fires on pipeline failure.
type ErrMsg struct{ Err error }

// SendPhase returns a tea.Cmd that emits a PhaseMsg.
func SendPhase(p pipeline.Phase) tea.Cmd      { return func() tea.Msg { return PhaseMsg{Phase: p} } }
func SendTile(p tile.Progress) tea.Cmd        { return func() tea.Msg { return TileProgressMsg{Progress: p} } }
func SendUpload(p pipeline.UploadProgress) tea.Cmd { return func() tea.Msg { return UploadProgressMsg{Progress: p} } }
func SendDone(r *pipeline.Result) tea.Cmd     { return func() tea.Msg { return DoneMsg{Result: r} } }
func SendErr(err error) tea.Cmd               { return func() tea.Msg { return ErrMsg{Err: err} } }
