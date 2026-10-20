package copier

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gustavoaveiro/sortinel/internal/model"
	"github.com/gustavoaveiro/sortinel/internal/scanner"
)

// Stress tests for the kind of real-world filenames that break naive file
// tools: unicode/emoji, names near the filesystem's length limit, special
// characters, and permission-denied source files.

func TestRun_UnicodeAndEmojiFilenames(t *testing.T) {
	names := []string{
		"café_notes.txt",
		"日本語のファイル名.txt",
		"emoji😀📸🎉.txt",
		"файл.txt", // Cyrillic
		"naïve_résumé.txt",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			in := t.TempDir()
			out := t.TempDir()
			src := filepath.Join(in, name)
			writeFile(t, src, "some plain text content")

			entries := make(chan scanner.Entry, 1)
			entries <- fileEntry(t, src)
			close(entries)

			records := Run(entries, Options{OutDir: out, Workers: 1})
			if len(records) != 1 {
				t.Fatalf("got %d records, want 1", len(records))
			}
			rec := records[0]
			if rec.Status != model.StatusSuccess {
				t.Fatalf("status = %q, want Success (reason: %s)", rec.Status, rec.Reason)
			}
			if rec.FileName != name {
				t.Errorf("FileName = %q, want %q", rec.FileName, name)
			}
			if _, err := os.Stat(rec.DestPath); err != nil {
				t.Errorf("destination missing: %v", err)
			}
			if !strings.HasSuffix(rec.DestPath, name) {
				t.Errorf("DestPath = %q, want it to end with %q", rec.DestPath, name)
			}
		})
	}
}

func TestRun_FilenameWithSpacesAndSpecialCharacters(t *testing.T) {
	// Parens, brackets, quotes, ampersands, semicolons - our code never
	// builds a shell string from a filename (os.Open/os.Create take the
	// path directly, no shell involved), so this should just work.
	name := `My Notes (2024) [final] - "copy" & more; v2.txt`

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, name)
	writeFile(t, src, "content")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	rec := records[0]
	if rec.Status != model.StatusSuccess {
		t.Fatalf("status = %q, want Success (reason: %s)", rec.Status, rec.Reason)
	}
	if _, err := os.Stat(rec.DestPath); err != nil {
		t.Errorf("destination missing: %v", err)
	}
}

func TestRun_VeryLongFilenameNearFilesystemLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has its own (different, often shorter) path length rules")
	}

	// Most Linux filesystems cap a single path component at 255 bytes.
	longBase := strings.Repeat("a", 251)
	name := longBase + ".txt"
	if len(name) != 255 {
		t.Fatalf("test setup: name is %d bytes, want 255", len(name))
	}

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, name)
	writeFile(t, src, "content")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	rec := records[0]
	if rec.Status != model.StatusSuccess {
		t.Fatalf("status = %q, want Success (reason: %s)", rec.Status, rec.Reason)
	}
	if _, err := os.Stat(rec.DestPath); err != nil {
		t.Errorf("destination missing: %v", err)
	}
}

func TestRun_CollisionRenameOverflowingFilesystemLimitFailsGracefully(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has its own (different, often shorter) path length rules")
	}

	// A 255-byte name already at the limit: appending "_1" during a
	// collision rename would overflow it. Must fail cleanly, not panic.
	longBase := strings.Repeat("a", 251)
	name := longBase + ".txt"

	in := t.TempDir()
	out := t.TempDir()
	src := filepath.Join(in, name)
	writeFile(t, src, "new content, different from what's there")

	// Pre-create a different-content file of the same name, forcing a
	// rename attempt rather than an Already-Backed-Up match.
	existing := filepath.Join(out, string(model.CategoryText), name)
	writeFile(t, existing, "old content")

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: out, Workers: 1})
	rec := records[0]
	if rec.Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed (an overflowing rename must fail cleanly, not panic)", rec.Status)
	}
	if rec.Reason == "" {
		t.Error("expected a non-empty failure reason")
	}
}

func TestRun_UnreadableSourceFileProducesFailedRecord_WeirdName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores file permissions")
	}

	in := t.TempDir()
	src := filepath.Join(in, "秘密のファイル😀.txt")
	writeFile(t, src, "secret")
	if err := os.Chmod(src, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(src, 0o644)

	entries := make(chan scanner.Entry, 1)
	entries <- fileEntry(t, src)
	close(entries)

	records := Run(entries, Options{OutDir: t.TempDir(), Workers: 1})
	rec := records[0]
	if rec.Status != model.StatusFailed {
		t.Errorf("status = %q, want Failed", rec.Status)
	}
}
