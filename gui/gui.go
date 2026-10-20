// Package gui provides the desktop interface for sortinel, built with
// Fyne so the same binary runs natively on Windows, macOS and Linux.
package gui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	coreapp "github.com/gustavoaveiro/sortinel/internal/app"
)

// Run opens the main window and blocks until the user closes it.
func Run() {
	a := app.New()
	w := a.NewWindow("Sortinel")

	inEntry := widget.NewEntry()
	inEntry.SetPlaceHolder("Folder to back up")
	outEntry := widget.NewEntry()
	outEntry.SetPlaceHolder("Folder to write the organized backup into")

	workersEntry := widget.NewEntry()
	workersEntry.SetText(strconv.Itoa(runtime.NumCPU()))

	dryRunCheck := widget.NewCheck("Dry run (classify only, don't copy)", nil)

	status := widget.NewLabel("")
	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	openReportBtn := widget.NewButton("Open report", nil)
	openReportBtn.Disable()

	browse := func(target *widget.Entry) *widget.Button {
		return widget.NewButton("Browse…", func() {
			dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
				if err != nil || u == nil {
					return
				}
				target.SetText(u.Path())
			}, w)
		})
	}

	var runBtn *widget.Button
	runBtn = widget.NewButton("Run backup", func() {
		workers, err := strconv.Atoi(workersEntry.Text)
		if err != nil || workers < 1 {
			dialog.ShowError(fmt.Errorf("workers must be a positive number"), w)
			return
		}

		cfg := coreapp.Config{
			InDir:   inEntry.Text,
			OutDir:  outEntry.Text,
			Workers: workers,
			DryRun:  dryRunCheck.Checked,
		}

		runBtn.Disable()
		openReportBtn.Disable()
		status.SetText("Running…")
		progress.Show()

		go func() {
			summary, err := coreapp.Run(cfg)

			fyne.Do(func() {
				progress.Hide()
				runBtn.Enable()
				if err != nil {
					status.SetText("Failed: " + err.Error())
					dialog.ShowError(err, w)
					return
				}
				status.SetText(fmt.Sprintf("Done: %d files processed. Report: %s", summary.Total, summary.ReportPath))
				openReportBtn.OnTapped = func() { openInDefaultApp(summary.ReportPath) }
				openReportBtn.Enable()
			})
		}()
	})

	form := container.NewVBox(
		widget.NewLabel("Input folder"),
		container.NewBorder(nil, nil, nil, browse(inEntry), inEntry),
		widget.NewLabel("Output folder"),
		container.NewBorder(nil, nil, nil, browse(outEntry), outEntry),
		widget.NewLabel("Worker count"),
		workersEntry,
		dryRunCheck,
		runBtn,
		progress,
		status,
		openReportBtn,
	)

	w.SetContent(container.NewPadded(form))
	w.Resize(fyne.NewSize(480, 420))
	w.ShowAndRun()
}

func openInDefaultApp(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}
