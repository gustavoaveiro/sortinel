// Package copier classifies and copies files discovered by the scanner into
// per-category folders under an output directory, producing one model.Record
// per source file.
package copier

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gustavoaveiro/sortinel/internal/classifier"
	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/progress"
	"github.com/gustavoaveiro/sortinel/internal/scanner"
)

// openSource opens path for reading. It's a package variable so tests can
// substitute a reader that fails partway through - simulating a real I/O
// error mid-read, which isn't practical to trigger deterministically
// against an actual filesystem.
var openSource = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// Options configures a copier Run.
type Options struct {
	OutDir  string
	Workers int
	// DryRun classifies and hashes every file but never writes to OutDir.
	DryRun bool
	// Progress, if non-nil, is notified once per entry as it finishes.
	Progress progress.Sink
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
				rec := process(e, opts)
				if opts.Progress != nil {
					opts.Progress.FileDone(rec.SizeBytes)
				}
				results <- rec
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
	case scanner.KindExcluded:
		rec.Status = model.StatusExcluded
		rec.Reason = fmt.Sprintf("matched exclude pattern %q", e.MatchedPattern)
		return rec
	case scanner.KindError:
		rec.Status = model.StatusFailed
		rec.Reason = e.Err.Error()
		return rec
	}

	rec.SizeBytes = e.Info.Size()

	src, err := openSource(e.Path)
	if err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = fmt.Sprintf("open source: %v", err)
		return rec
	}
	defer src.Close()

	if opts.DryRun {
		if err := classifyAndHash(src, &rec); err != nil {
			rec.Status = model.StatusFailed
			rec.Reason = err.Error()
			return rec
		}
		rec.Status = model.StatusSuccess
		rec.Reason = "dry run - not copied"
		rec.DestPath = filepath.Join(opts.OutDir, string(rec.Category), rec.FileName)
		return rec
	}

	br, err := peekAndClassify(src, &rec)
	if err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = err.Error()
		return rec
	}

	// Common case: nothing is at the natural destination name yet, so
	// there's no need to know the content hash before deciding where to
	// write - copy and hash in a single pass over the source instead of
	// reading it twice.
	naturalDest := filepath.Join(opts.OutDir, string(rec.Category), rec.FileName)
	_, statErr := os.Stat(naturalDest)
	switch {
	case os.IsNotExist(statErr):
		hash, err := copyAndHash(br, naturalDest)
		if err != nil {
			rec.Status = model.StatusFailed
			rec.Reason = fmt.Sprintf("copy: %v", err)
			return rec
		}
		rec.SHA256 = hash
		rec.DestPath = naturalDest
		rec.Status = model.StatusSuccess
		return rec
	case statErr != nil:
		rec.Status = model.StatusFailed
		rec.Reason = fmt.Sprintf("resolve destination: %v", statErr)
		return rec
	}

	// Something already exists at the natural name: the source must be
	// fully hashed before we can tell a genuine duplicate apart from an
	// unrelated collision, so this path does read the source twice.
	hash, err := hashReader(br)
	if err != nil {
		rec.Status = model.StatusFailed
		rec.Reason = fmt.Sprintf("hash: %v", err)
		return rec
	}
	rec.SHA256 = hash

	place, err := resolvePlacement(opts.OutDir, rec.Category, rec.FileName, rec.SizeBytes, hash)
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

// peekAndClassify reads enough of r to classify the file (without
// consuming those bytes from the returned reader) and fills in the
// record's DetectedType and Category.
func peekAndClassify(r io.Reader, rec *model.Record) (*bufio.Reader, error) {
	br := bufio.NewReaderSize(r, 4096)
	peeked, err := br.Peek(min(4096, int(rec.SizeBytes)))
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read header: %w", err)
	}

	mimeType, category := classifier.ClassifyBytes(peeked, rec.FileName)
	rec.DetectedType = mimeType
	rec.Category = category
	return br, nil
}

// classifyAndHash reads r once, filling in the record's DetectedType,
// Category and SHA256 fields, without writing anything (used for -dry-run,
// where there's no destination to stream into).
func classifyAndHash(r io.Reader, rec *model.Record) error {
	br, err := peekAndClassify(r, rec)
	if err != nil {
		return err
	}

	hash, err := hashReader(br)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	rec.SHA256 = hash
	return nil
}

// copyAndHash streams r into a newly created file at dst (creating parent
// category directories as needed), computing its SHA-256 in the same pass
// rather than reading the source a second time.
func copyAndHash(r io.Reader, dst string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("create category dir: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), r); err != nil {
		return "", fmt.Errorf("write destination: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), out.Close()
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
