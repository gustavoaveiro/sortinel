// Package copier classifies and copies files discovered by the scanner into
// per-category folders under an output directory, producing one model.Record
// per source file.
package copier

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gustavoaveiro/sortinel/internal/classifier"
	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/scanner"
)

// Options configures a copier Run.
type Options struct {
	OutDir  string
	Workers int
	// DryRun classifies and hashes every file but never writes to OutDir.
	DryRun bool
}

// Run consumes entries until the channel is closed, processing them with a
// pool of opts.Workers goroutines, and returns one Record per entry. Order
// of the returned records is not guaranteed to match the scan order.
func Run(entries <-chan scanner.Entry, opts Options) []model.Record {
	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}

	results := make(chan model.Record)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range entries {
				results <- process(e, opts)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var records []model.Record
	for r := range results {
		records = append(records, r)
	}
	return records
}

func process(e scanner.Entry, opts Options) model.Record {
	rec := model.Record{
		SourcePath: e.Path,
		FileName:   filepath.Base(e.Path),
		Timestamp:  time.Now().UTC(),
	}

	switch e.Kind {
	case scanner.KindSkippedSymlink:
		rec.Status = model.StatusSkippedSymlink
		rec.Reason = "symbolic link - not followed"
		return rec
	case scanner.KindError:
		rec.Status = model.StatusFailed
		rec.Reason = e.Err.Error()
		return rec
	}

	rec.SizeBytes = e.Info.Size()

	if err := classifyAndHash(e.Path, &rec); err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = err.Error()
		return rec
	}

	if opts.DryRun {
		rec.Status = model.StatusSuccess
		rec.Reason = "dry run - not copied"
		rec.DestPath = filepath.Join(opts.OutDir, string(rec.Category), rec.FileName)
		return rec
	}

	place, err := resolvePlacement(opts.OutDir, rec.Category, rec.FileName, rec.SizeBytes, rec.SHA256)
	if err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = fmt.Sprintf("resolve destination: %v", err)
		return rec
	}

	rec.DestPath = place.path
	rec.Status = place.status
	rec.Reason = place.reason

	if place.status == model.StatusAlreadyBackedUp {
		return rec
	}

	if err := copyFile(e.Path, place.path); err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = fmt.Sprintf("copy: %v", err)
	}
	return rec
}

// classifyAndHash reads src once, filling in the record's DetectedType,
// Category and SHA256 fields.
func classifyAndHash(path string, rec *model.Record) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 4096)
	peeked, err := br.Peek(min(4096, int(rec.SizeBytes)))
	if err != nil && err != io.EOF {
		return fmt.Errorf("read header: %w", err)
	}

	mimeType, category, err := classifier.Classify(bytes.NewReader(peeked), rec.FileName)
	if err != nil {
		return fmt.Errorf("classify: %w", err)
	}
	rec.DetectedType = mimeType
	rec.Category = category

	hash, err := hashReader(br)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	rec.SHA256 = hash
	return nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create category dir: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	return out.Close()
}
