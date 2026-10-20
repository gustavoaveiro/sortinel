// Command sortinel deep-scans an input directory tree, copies every file
// it finds into an output directory organized by file type, and writes an
// .xlsx report of what happened to each file.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"golang.org/x/term"

	"github.com/gustavoaveiro/sortinel/internal/app"
	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/progress"
)

// version is set at build time via -ldflags "-X main.version=...";
// goreleaser injects the release tag when publishing.
var version = "dev"

func main() {
	var (
		in         = flag.String("in", "", "source directory to scan (required unless -gui)")
		out        = flag.String("out", "", "destination directory for the organized backup (required unless -gui)")
		workers    = flag.Int("workers", runtime.NumCPU(), "number of concurrent copy workers")
		reportPath = flag.String("report", "", "path for the .xlsx report (default: <out>/backup_report_<timestamp>.xlsx)")
		dryRun     = flag.Bool("dry-run", false, "scan and classify files without copying them")
		useGUI     = flag.Bool("gui", false, "launch the graphical interface instead of running from flags")
		showVer    = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("sortinel", version)
		return
	}

	if *useGUI {
		runGUI()
		return
	}

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "sortinel: -in and -out are required (or pass -gui)")
		flag.Usage()
		os.Exit(2)
	}

	tracker := progress.NewTracker()
	stopRender, renderDone := startProgressRendering(tracker)

	summary, err := app.Run(app.Config{
		InDir:      *in,
		OutDir:     *out,
		Workers:    *workers,
		DryRun:     *dryRun,
		ReportPath: *reportPath,
		Progress:   tracker,
	})

	close(stopRender)
	<-renderDone

	if err != nil {
		fmt.Fprintln(os.Stderr, "sortinel:", err)
		os.Exit(1)
	}

	fmt.Printf("Processed %d files\n", summary.Total)
	for _, status := range []model.Status{
		model.StatusSuccess, model.StatusRenamed, model.StatusAlreadyBackedUp,
		model.StatusSkippedSymlink, model.StatusFailed,
	} {
		if n := summary.Counts[status]; n > 0 {
			fmt.Printf("  %-22s %d\n", status, n)
		}
	}
	fmt.Println("Report written to", summary.ReportPath)
}

// startProgressRendering prints a live progress bar to stderr (so it never
// mixes with stdout output) every 250ms, until stop is closed. It's a
// no-op when stderr isn't a terminal, since overwriting a line with \r
// only makes sense interactively. The returned done channel is closed once
// rendering has fully stopped (including the final 100% line).
func startProgressRendering(t *progress.Tracker) (stop chan struct{}, done chan struct{}) {
	stop = make(chan struct{})
	done = make(chan struct{})

	if !term.IsTerminal(int(os.Stderr.Fd())) {
		close(done)
		return stop, done
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				fmt.Fprint(os.Stderr, "\r"+progress.RenderBar(t.Snapshot(), 30)+"\n")
				return
			case <-ticker.C:
				fmt.Fprint(os.Stderr, "\r"+progress.RenderBar(t.Snapshot(), 30))
			}
		}
	}()
	return stop, done
}
