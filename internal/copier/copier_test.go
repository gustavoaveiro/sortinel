package copier

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestRun_FastPathCopyAndHashErrorProducesFailedRecord(t *testing.T) {
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
	// category subdirectory is denied (no write bit) - the natural
	// destination doesn't exist (fast path), so copyAndHash's own MkdirAll
	// is what fails.
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

func TestRun_NaturalDestinationStatErrorProducesFailedRecord(t *testing.T) {
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

func TestRun_SlowPathResolvePlacementErrorProducesFailedRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores file permissions")
	}

	in := t.TempDir()
	out := t.TempDir()
	content := "same size, unreadable existing file"
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, content)

	// An existing file of the same name and size, so resolvePlacement's
	// size check passes and it tries to hash the existing file to compare
	// - which fails because that file isn't readable.
	existing := filepath.Join(out, string(model.CategoryText), "notes.txt")
	writeFile(t, existing, content)
	if err := os.Chmod(existing, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(existing, 0o644)

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

func TestRun_SlowPathCopyFileErrorProducesFailedRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, "notes.txt")
	writeFile(t, src, "new content, different from what's already there")

	categoryDir := filepath.Join(out, string(model.CategoryText))
	// A same-named file with different content, forcing a rename to
	// notes_1.txt - which copyFile then can't create once the directory
	// is made read-only.
	writeFile(t, filepath.Join(categoryDir, "notes.txt"), "old content")
	if err := os.Chmod(categoryDir, 0o555); err != nil {
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

func TestRun_DryRunSourceBecameADirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file behaves differently on windows")
	}

	in := t.TempDir()
	out := t.TempDir()
	srcDir := filepath.Join(in, "imasrcdir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, srcDir)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1, DryRun: true})
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

func TestCopyAndHash_Success(t *testing.T) {
	out := t.TempDir()
	dst := filepath.Join(out, "sub", "notes.txt")

	hash, err := copyAndHash(strings.NewReader("hello world"), dst)
	if err != nil {
		t.Fatalf("copyAndHash: %v", err)
	}
	if got := readDest(t, dst); got != "hello world" {
		t.Errorf("copied content = %q, want %q", got, "hello world")
	}
	wantHash, _ := hashFile(dst)
	if hash != wantHash {
		t.Errorf("hash = %q, want %q", hash, wantHash)
	}
}

func TestCopyAndHash_MkdirAllFailsWhenAncestorIsAFile(t *testing.T) {
	out := t.TempDir()
	blocker := filepath.Join(out, "blocker")
	writeFile(t, blocker, "I am a file, not a directory")

	_, err := copyAndHash(strings.NewReader("content"), filepath.Join(blocker, "sub", "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCopyAndHash_CreateDestFailsWhenDirNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	out := t.TempDir()
	dir := filepath.Join(out, "readonly")
	if err := os.MkdirAll(dir, 0o555); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	_, err := copyAndHash(strings.NewReader("content"), filepath.Join(dir, "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCopyAndHash_ReadErrorPropagates(t *testing.T) {
	out := t.TempDir()
	_, err := copyAndHash(erroringReader{}, filepath.Join(out, "dst.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestClassifyAndHash_PeekErrorPropagates(t *testing.T) {
	rec := model.Record{SizeBytes: 10}
	err := classifyAndHash(erroringReader{}, &rec)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// headerThenErrorReader serves a fixed header successfully, then fails on
// every subsequent read - used to make Peek succeed while the later full
// read (used for hashing) fails.
type headerThenErrorReader struct {
	header []byte
	err    error
}

func (r *headerThenErrorReader) Read(p []byte) (int, error) {
	if len(r.header) > 0 {
		n := copy(p, r.header)
		r.header = r.header[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *headerThenErrorReader) Close() error { return nil }

func TestRun_SlowPathHashErrorProducesFailedRecord(t *testing.T) {
	out := t.TempDir()
	// Something already at the natural destination, so process() takes
	// the slow path and needs to fully hash the "source" before deciding
	// whether it's a duplicate or a real collision.
	writeFile(t, filepath.Join(out, string(model.CategoryText), "notes.txt"), "existing")

	// Plain ASCII header so ClassifyBytes lands this in Documents/Text,
	// matching the existing file set up above.
	header := make([]byte, 4096)
	for i := range header {
		header[i] = 'a'
	}

	original := openSource
	openSource = func(string) (io.ReadCloser, error) {
		return &headerThenErrorReader{
			header: header,
			err:    errors.New("simulated read failure"),
		}, nil
	}
	defer func() { openSource = original }()

	entries := make(chan scanner.Entry, 1)
	entries <- scanner.Entry{
		Path: "/irrelevant/notes.txt",
		Info: fakeFileInfo{name: "notes.txt", size: 4096 + 10},
		Kind: scanner.KindFile,
	}
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed (reason: %s)", records[0].Status, records[0].Reason)
	}
}

// fakeFileInfo is a minimal os.FileInfo for entries whose Path doesn't
// correspond to anything openSource actually reads from disk.
type fakeFileInfo struct {
	name string
	size int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }

func TestClassifyAndHash_HashErrorAfterSuccessfulPeek(t *testing.T) {
	header := make([]byte, 4096)
	r := &headerThenErrorReader{header: header, err: errors.New("simulated read failure")}
	rec := model.Record{SizeBytes: int64(len(header) + 10)}

	err := classifyAndHash(r, &rec)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestClassifyAndHash_SourceBecameADirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file behaves differently on windows")
	}

	in := t.TempDir()
	out := t.TempDir()
	srcDir := filepath.Join(in, "imasrcdir")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, srcDir)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed (reason: %s)", records[0].Status, records[0].Reason)
	}
}

// fakeProgressSink records calls made to it, safe for concurrent use since
// Run's workers call FileDone from multiple goroutines.
type fakeProgressSink struct {
	mu            sync.Mutex
	totalFiles    int
	totalBytes    int64
	fileDoneCalls int
	bytesDone     int64
}

func (f *fakeProgressSink) SetTotal(files int, bytes int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.totalFiles, f.totalBytes = files, bytes
}

func (f *fakeProgressSink) FileDone(size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fileDoneCalls++
	f.bytesDone += size
}

func TestRun_ReportsProgressForEveryEntry(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	writeFile(t, filepath.Join(in, "a.txt"), "hello")
	writeFile(t, filepath.Join(in, "b.txt"), "world!")

	sink := &fakeProgressSink{}
	entries := make(chan scanner.Entry, 2)
	entries <- fileEntry(t, filepath.Join(in, "a.txt"))
	entries <- fileEntry(t, filepath.Join(in, "b.txt"))
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 2, Progress: sink})

	if sink.fileDoneCalls != len(records) {
		t.Errorf("fileDoneCalls = %d, want %d", sink.fileDoneCalls, len(records))
	}
	var wantBytes int64
	for _, r := range records {
		wantBytes += r.SizeBytes
	}
	if sink.bytesDone != wantBytes {
		t.Errorf("bytesDone = %d, want %d", sink.bytesDone, wantBytes)
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
