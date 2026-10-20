// Package app wires the scanner, copier and report packages into the single
// Run entry point shared by the CLI and the GUI.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gustavoaveiro/sortinel/internal/copier"
	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/progress"
	"github.com/gustavoaveiro/sortinel/internal/report"
	"github.com/gustavoaveiro/sortinel/internal/scanner"
)

// Config describes one backup run.
type Config struct {
	InDir  string
	OutDir string
	// Workers is the number of concurrent copy workers. Defaults to 4 if < 1.
	Workers int
	// DryRun classifies and hashes files without copying them.
	DryRun bool
	// ReportPath is where the .xlsx report is written. If empty, a
	// timestamped file is created inside OutDir.
	ReportPath string
	// Progress, if non-nil, receives a SetTotal call after a metadata-only
	// pre-scan of InDir, then a FileDone call per entry as work proceeds.
	Progress progress.Sink
	// Excludes are additional glob patterns (matched against a file or
	// directory's base name) to skip, on top of scanner.DefaultExcludes
	// unless NoDefaultExcludes is set.
	Excludes []string
	// NoDefaultExcludes disables scanner.DefaultExcludes, using only
	// Excludes.
	NoDefaultExcludes bool
}

// ParseExcludePatterns splits a comma-separated list of glob patterns
// (as taken from a CLI flag or GUI text field) into a slice, trimming
// whitespace and dropping empty entries.
func ParseExcludePatterns(s string) []string {
	var patterns []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// Summary is returned after a run completes.
type Summary struct {
	ReportPath string
	Total      int
	Counts     map[model.Status]int
}

// Run scans cfg.InDir, copies/classifies every file into cfg.OutDir and
// writes the .xlsx report, returning a Summary of what happened.
func Run(cfg Config) (Summary, error) {
	if cfg.InDir == "" || cfg.OutDir == "" {
		return Summary{}, fmt.Errorf("both input and output directories are required")
	}
	info, err := os.Stat(cfg.InDir)
	if err != nil {
		return Summary{}, fmt.Errorf("input directory: %w", err)
	}
	if !info.IsDir() {
		return Summary{}, fmt.Errorf("input path %q is not a directory", cfg.InDir)
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return Summary{}, fmt.Errorf("create output directory: %w", err)
	}

	workers := cfg.Workers
	if workers < 1 {
		workers = 4
	}

	excludes := cfg.Excludes
	if !cfg.NoDefaultExcludes {
		excludes = append(append([]string{}, scanner.DefaultExcludes...), excludes...)
	}
	if err := scanner.ValidatePatterns(excludes); err != nil {
		return Summary{}, err
	}

	if cfg.Progress != nil {
		files, bytes := scanner.Count(cfg.InDir, excludes)
		cfg.Progress.SetTotal(files, bytes)
	}

	entries := scanner.Walk(cfg.InDir, excludes)
	records := copier.Run(entries, copier.Options{
		OutDir:   cfg.OutDir,
		Workers:  workers,
		DryRun:   cfg.DryRun,
		Progress: cfg.Progress,
	})

	reportPath := cfg.ReportPath
	if reportPath == "" {
		reportPath = filepath.Join(cfg.OutDir, fmt.Sprintf("backup_report_%s.xlsx", time.Now().Format("20060102_150405")))
	}
	if err := report.Write(reportPath, records); err != nil {
		return Summary{}, fmt.Errorf("write report: %w", err)
	}

	summary := Summary{ReportPath: reportPath, Total: len(records), Counts: map[model.Status]int{}}
	for _, r := range records {
		summary.Counts[r.Status]++
	}
	return summary, nil
}
