package copier

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/scanner"
)

func statInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	return info
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

func fileEntry(t *testing.T, path string) scanner.Entry {
	t.Helper()
	return scanner.Entry{Path: path, Info: statInfo(t, path), Kind: scanner.KindFile}
}

func readDest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(b)
}

func TestRun_CopiesClassifiedFile(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "hello world")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 2})

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.Status != model.StatusSuccess {
		t.Errorf("status = %q, want Success (reason: %s)", rec.Status, rec.Reason)
	}
	if rec.Category != model.CategoryText {
		t.Errorf("category = %q, want Text", rec.Category)
	}
	wantDest := filepath.Join(out, string(model.CategoryText), "notes.txt")
	if rec.DestPath != wantDest {
		t.Errorf("destPath = %q, want %q", rec.DestPath, wantDest)
	}
	if got := readDest(t, wantDest); got != "hello world" {
		t.Errorf("copied content = %q, want %q", got, "hello world")
	}
	if rec.SHA256 == "" {
		t.Error("expected a non-empty SHA256")
	}
}

func TestRun_DryRunDoesNotCopy(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "hello world")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1, DryRun: true})

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusSuccess {
		t.Errorf("status = %q, want Success", records[0].Status)
	}
	if _, err := os.Stat(records[0].DestPath); !os.IsNotExist(err) {
		t.Errorf("expected dest %q not to exist in dry-run mode", records[0].DestPath)
	}
}

func TestRun_NameCollisionIsRenamedWithReason(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	src1 := filepath.Join(in, "a", "notes.txt")
	writeFile(t, src1, "content A")
	src2 := filepath.Join(in, "b", "notes.txt")
	writeFile(t, src2, "content B - different")

	entries := make(chan scanner.Entry, 2)
	entries <- fileEntry(t, src1)
	entries <- fileEntry(t, src2)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	byDest := map[string]model.Record{}
	for _, r := range records {
		byDest[r.DestPath] = r
	}

	wantOriginal := filepath.Join(out, string(model.CategoryText), "notes.txt")
	wantRenamed := filepath.Join(out, string(model.CategoryText), "notes_1.txt")

	original, ok := byDest[wantOriginal]
	if !ok {
		t.Fatalf("expected a record with dest %q, got %+v", wantOriginal, records)
	}
	if original.Status != model.StatusSuccess {
		t.Errorf("original status = %q, want Success", original.Status)
	}

	renamed, ok := byDest[wantRenamed]
	if !ok {
		t.Fatalf("expected a record with dest %q, got %+v", wantRenamed, records)
	}
	if renamed.Status != model.StatusRenamed {
		t.Errorf("renamed status = %q, want Renamed", renamed.Status)
	}
	if renamed.Reason == "" {
		t.Error("expected a non-empty reason for the renamed file")
	}
}

func TestRun_IdenticalFileIsRecognizedAsAlreadyBackedUp(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "same content twice")

	destDir := filepath.Join(out, string(model.CategoryText))
	writeFile(t, filepath.Join(destDir, "notes.txt"), "same content twice")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusAlreadyBackedUp {
		t.Errorf("status = %q, want Already Backed Up (reason: %s)", records[0].Status, records[0].Reason)
	}
}

func TestRun_SkippedSymlinkProducesSkippedRecord(t *testing.T) {
	entries := make(chan scanner.Entry, 1)
	entries <- scanner.Entry{Path: "/some/link.txt", Kind: scanner.KindSkippedSymlink}
	close(entries)

	records := Run(entries, Options{OutDir: t.TempDir(), Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusSkippedSymlink {
		t.Errorf("status = %q, want Skipped - Symlink", records[0].Status)
	}
}

func TestRun_ScanErrorEntryProducesFailedRecord(t *testing.T) {
	entries := make(chan scanner.Entry, 1)
	entries <- scanner.Entry{Path: "/no/such/path", Kind: scanner.KindError, Err: errors.New("permission denied")}
	close(entries)

	records := Run(entries, Options{OutDir: t.TempDir(), Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed", records[0].Status)
	}
	if records[0].Reason != "permission denied" {
		t.Errorf("reason = %q, want %q", records[0].Reason, "permission denied")
	}
}

func TestRun_UnreadableSourceProducesFailedRecord(t *testing.T) {
	in := t.TempDir()
	missing := filepath.Join(in, "gone.txt")

	entry := scanner.Entry{Path: missing, Info: nil, Kind: scanner.KindFile}
	// Give it a fake Info since classifyAndHash only needs a size; use the
	// parent dir's info as a stand-in so Entry construction stays simple.
	entry.Info = statInfo(t, in)

	entries := make(chan scanner.Entry, 1)
	entries <- entry
	close(entries)

	records := Run(entries, Options{OutDir: t.TempDir(), Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed", records[0].Status)
	}
}

func TestRun_EmptyFileIsHandled(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "empty.txt")
	writeFile(t, src, "")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusSuccess {
		t.Errorf("status = %q, want Success", records[0].Status)
	}
}

func TestRun_ResolvePlacementErrorProducesFailedRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "hello")

	categoryDir := filepath.Join(out, string(model.CategoryText))
	if err := os.MkdirAll(categoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(categoryDir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(categoryDir, 0o755)

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed (reason: %s)", records[0].Status, records[0].Reason)
	}
}

func TestRun_CopyFileErrorProducesFailedRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "hello")

	// Lookups inside out still succeed (execute bit set), but creating the
	// category subdirectory is denied (no write bit) - resolvePlacement
	// succeeds while copyFile's MkdirAll fails.
	if err := os.Chmod(out, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(out, 0o755)

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed (reason: %s)", records[0].Status, records[0].Reason)
	}
}

func TestCopyFile_MissingSource(t *testing.T) {
	err := copyFile(filepath.Join(t.TempDir(), "nope.txt"), filepath.Join(t.TempDir(), "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCopyFile_MkdirAllFailsWhenAncestorIsAFile(t *testing.T) {
	out := t.TempDir()
	src := filepath.Join(out, "src.txt")
	writeFile(t, src, "content")

	// "blocker" exists as a regular file, so MkdirAll("blocker/sub") must fail.
	blocker := filepath.Join(out, "blocker")
	writeFile(t, blocker, "I am a file, not a directory")

	err := copyFile(src, filepath.Join(blocker, "sub", "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCopyFile_CreateDestFailsWhenDirNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	out := t.TempDir()
	src := filepath.Join(out, "src.txt")
	writeFile(t, src, "content")

	dir := filepath.Join(out, "readonly")
	if err := os.MkdirAll(dir, 0o555); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	err := copyFile(src, filepath.Join(dir, "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCopyFile_ReadingFromADirectoryFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file behaves differently on windows")
	}

	out := t.TempDir()
	srcDir := filepath.Join(out, "imasrcdir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := copyFile(srcDir, filepath.Join(out, "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestClassifyAndHash_SourceBecameADirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file behaves differently on windows")
	}

	dir := t.TempDir()
	rec := model.Record{SizeBytes: 10}
	err := classifyAndHash(dir, &rec)
	if err == nil {
		t.Fatal("expected an error reading the header of a directory, got nil")
	}
}

func TestRun_DefaultsToOneWorkerWhenUnset(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "hello")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 0})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
}
