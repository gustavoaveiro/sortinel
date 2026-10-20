package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
)

func drain(t *testing.T, root string) []Entry {
	t.Helper()
	var entries []Entry
	for e := range Walk(root) {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func TestWalk_FindsNestedRegularFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "top.txt"), "top")
	mustMkdir(t, filepath.Join(root, "sub", "deeper"))
	mustWriteFile(t, filepath.Join(root, "sub", "deeper", "nested.txt"), "nested")

	entries := drain(t, root)

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e.Kind != KindFile {
			t.Errorf("entry %q kind = %v, want KindFile", e.Path, e.Kind)
		}
		if e.Info == nil {
			t.Errorf("entry %q has nil Info", e.Path)
		}
	}
}

func TestWalk_SkipsSymlinksWithoutFollowing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	mustWriteFile(t, target, "real content")

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// A symlink to a directory must not be recursed into.
	linkedDir := filepath.Join(root, "dirlink")
	realDir := filepath.Join(root, "realdir")
	mustMkdir(t, realDir)
	mustWriteFile(t, filepath.Join(realDir, "inside.txt"), "inside")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatalf("Symlink dir: %v", err)
	}

	entries := drain(t, root)

	var files, symlinks int
	for _, e := range entries {
		switch e.Kind {
		case KindFile:
			files++
		case KindSkippedSymlink:
			symlinks++
		}
	}

	// real.txt, inside.txt (via realdir) = 2 files; link.txt, dirlink = 2 symlinks.
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
	if symlinks != 2 {
		t.Errorf("symlinks = %d, want 2", symlinks)
	}
}

func TestWalk_EmptyDirectoryProducesNoEntries(t *testing.T) {
	root := t.TempDir()
	entries := drain(t, root)
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0: %+v", len(entries), entries)
	}
}

func TestWalk_UnreadableDirectoryProducesErrorEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	mustMkdir(t, blocked)
	mustWriteFile(t, filepath.Join(blocked, "secret.txt"), "secret")

	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(blocked, 0o755)

	entries := drain(t, root)

	var sawError bool
	for _, e := range entries {
		if e.Kind == KindError {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("expected at least one KindError entry, got %+v", entries)
	}
}

func TestWalk_SkipsNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on windows")
	}

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "regular.txt"), "regular")

	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	entries := drain(t, root)

	var files int
	for _, e := range entries {
		if e.Kind == KindFile {
			files++
		}
	}
	if files != 1 {
		t.Errorf("files = %d, want 1 (fifo must be skipped): %+v", files, entries)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}
