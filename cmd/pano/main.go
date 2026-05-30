// Command pano tiles, uploads, and manages panoramas for bigpictures.company.
//
// The exit path is:
//   - errors that wrap *rfc9457.RFC9457 are rendered as pretty text on a TTY,
//     or as one-line JSON on stderr otherwise. Exit code 1.
//   - any other error is rendered as plain text. Exit code 1.
//   - success exits 0.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"alpineworks.io/rfc9457"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/version"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := newRootCmd()
	if err := root.ExecuteContext(ctx); err != nil {
		renderError(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pano",
		Short:         "Tile, upload, and manage panoramas for bigpictures.company",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", version.Version, version.Commit, version.Date),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newConfigCmd(),
		newMigrateCmd(),
		newDBCmd(),
		newTileCmd(),
		newUploadTilesCmd(),
		newExifCmd(),
		newUploadCmd(),
		newListCmd(),
		newShowCmd(),
		newDeleteCmd(),
		newReprocessCmd(),
	)
	return cmd
}

// renderError writes err to w. If err (or anything in its chain) is a
// *rfc9457.RFC9457, the Problem is rendered as pretty text when w is a TTY
// and as compact JSON otherwise.
func renderError(w io.Writer, err error) {
	var p *rfc9457.RFC9457
	if errors.As(err, &p) {
		if isTTY(w) {
			renderProblemText(w, p)
			return
		}
		renderProblemJSON(w, p)
		return
	}
	fmt.Fprintf(w, "error: %s\n", err.Error())
}

func renderProblemText(w io.Writer, p *rfc9457.RFC9457) {
	if p.Title != "" {
		fmt.Fprintf(w, "error: %s\n", p.Title)
	} else {
		fmt.Fprintln(w, "error: problem")
	}
	if p.Detail != "" {
		fmt.Fprintf(w, "  %s\n", p.Detail)
	}
	if p.Type != "" && p.Type != "about:blank" {
		fmt.Fprintf(w, "  type: %s\n", p.Type)
	}
	for k, v := range p.Extensions {
		fmt.Fprintf(w, "  %s: %v\n", k, v)
	}
}

func renderProblemJSON(w io.Writer, p *rfc9457.RFC9457) {
	body, err := p.ToJSON()
	if err != nil {
		fmt.Fprintf(w, "error: %s\n", p.Error())
		return
	}
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}

// isTTY reports whether w points at an interactive terminal. Avoids a separate
// dependency on go-isatty; falls back to false for non-*os.File writers.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
