package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

// fakeProgressSink records calls made to it for assertions. It must be
// concurrency-safe: Run's copier workers call FileDone from multiple
// goroutines.
type fakeProgressSink struct {
	mu         sync.Mutex
	totalFiles int
	totalBytes int64
	fileDones  int
	bytesDone  int64
}

func (f *fakeProgressSink) SetTotal(files int, bytes int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.totalFiles, f.totalBytes = files, bytes
}

func (f *fakeProgressSink) FileDone(size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fileDones++
	f.bytesDone += size
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestRun_EndToEnd(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	writeFile(t, filepath.Join(in, "notes.txt"), "hello")
	writeFile(t, filepath.Join(in, "sub", "report.pdf"), "%PDF-1.4 fake pdf bytes for detection")

	summary, err := Run(Config{InDir: in, OutDir: out})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if summary.Total != 2 {
		t.Errorf("Total = %d, want 2", summary.Total)
	}
	if summary.Counts[model.StatusSuccess] != 2 {
		t.Errorf("Success count = %d, want 2", summary.Counts[model.StatusSuccess])
	}
	if _, err := os.Stat(summary.ReportPath); err != nil {
		t.Errorf("expected report at %q: %v", summary.ReportPath, err)
	}

	f, err := excelize.OpenFile(summary.ReportPath)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", summary.ReportPath, err)
	}
	defer f.Close()
}

func TestRun_UsesExplicitReportPath(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "notes.txt"), "hello")

	reportPath := filepath.Join(t.TempDir(), "custom.xlsx")
	summary, err := Run(Config{InDir: in, OutDir: out, ReportPath: reportPath})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.ReportPath != reportPath {
		t.Errorf("ReportPath = %q, want %q", summary.ReportPath, reportPath)
	}
}

func TestRun_DryRunDoesNotCreateFiles(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "notes.txt"), "hello")

	summary, err := Run(Config{InDir: in, OutDir: out, DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Counts[model.StatusSuccess] != 1 {
		t.Errorf("Success count = %d, want 1", summary.Counts[model.StatusSuccess])
	}
	textDir := filepath.Join(out, string(model.CategoryText))
	if _, err := os.Stat(textDir); !os.IsNotExist(err) {
		t.Errorf("expected %q not to exist in dry-run mode", textDir)
	}
}

func TestRun_MissingInOrOutIsRejected(t *testing.T) {
	if _, err := Run(Config{InDir: "", OutDir: "x"}); err == nil {
		t.Error("expected an error when InDir is empty")
	}
	if _, err := Run(Config{InDir: "x", OutDir: ""}); err == nil {
		t.Error("expected an error when OutDir is empty")
	}
}

func TestRun_NonExistentInDirIsRejected(t *testing.T) {
	_, err := Run(Config{InDir: filepath.Join(t.TempDir(), "does-not-exist"), OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestRun_InDirThatIsAFileIsRejected(t *testing.T) {
	in := t.TempDir()
	filePath := filepath.Join(in, "notafile.txt")
	writeFile(t, filePath, "x")

	_, err := Run(Config{InDir: filePath, OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestRun_CannotCreateOutDir(t *testing.T) {
	in := t.TempDir()
	writeFile(t, filepath.Join(in, "notes.txt"), "hello")

	// OutDir's parent is a file, so MkdirAll must fail.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	writeFile(t, blocker, "I am a file")

	_, err := Run(Config{InDir: in, OutDir: filepath.Join(blocker, "sub")})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestRun_ReportWriteFailure(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "notes.txt"), "hello")

	// A directory can't be overwritten by the report writer's SaveAs.
	badReportPath := t.TempDir()

	_, err := Run(Config{InDir: in, OutDir: out, ReportPath: badReportPath})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestRun_DefaultWorkerCount(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "notes.txt"), "hello")

	// Workers left at zero should fall back to a sane default rather than
	// deadlocking or erroring.
	summary, err := Run(Config{InDir: in, OutDir: out, Workers: 0})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Total != 1 {
		t.Errorf("Total = %d, want 1", summary.Total)
	}
}

func TestRun_ReportsProgressWhenSinkProvided(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "a.txt"), "hello")
	writeFile(t, filepath.Join(in, "b.txt"), "world!")

	sink := &fakeProgressSink{}
	summary, err := Run(Config{InDir: in, OutDir: out, Progress: sink})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if sink.totalFiles != 2 {
		t.Errorf("totalFiles = %d, want 2", sink.totalFiles)
	}
	if sink.totalBytes != 11 { // "hello" (5) + "world!" (6)
		t.Errorf("totalBytes = %d, want 11", sink.totalBytes)
	}
	if sink.fileDones != summary.Total {
		t.Errorf("fileDones = %d, want %d", sink.fileDones, summary.Total)
	}
	if sink.bytesDone != sink.totalBytes {
		t.Errorf("bytesDone = %d, want %d", sink.bytesDone, sink.totalBytes)
	}
}
