package copier

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

func TestResolvePlacement_FreshFile(t *testing.T) {
	out := t.TempDir()
	hash, _ := hashFileContent(t, "content")

	place, err := resolvePlacement(out, model.CategoryText, "notes.txt", int64(len("content")), hash)
	if err != nil {
		t.Fatalf("resolvePlacement: %v", err)
	}
	if place.status != model.StatusSuccess {
		t.Errorf("status = %q, want Success", place.status)
	}
	want := filepath.Join(out, string(model.CategoryText), "notes.txt")
	if place.path != want {
		t.Errorf("path = %q, want %q", place.path, want)
	}
}

func TestResolvePlacement_CollisionThenAlreadyBackedUp(t *testing.T) {
	out := t.TempDir()
	dir := filepath.Join(out, string(model.CategoryText))
	writeFile(t, filepath.Join(dir, "notes.txt"), "different content")
	writeFile(t, filepath.Join(dir, "notes_1.txt"), "target content")

	hash, size := hashFileContent(t, "target content")

	place, err := resolvePlacement(out, model.CategoryText, "notes.txt", size, hash)
	if err != nil {
		t.Fatalf("resolvePlacement: %v", err)
	}
	if place.status != model.StatusAlreadyBackedUp {
		t.Errorf("status = %q, want Already Backed Up", place.status)
	}
	if place.path != filepath.Join(dir, "notes_1.txt") {
		t.Errorf("path = %q, want notes_1.txt", place.path)
	}
}

func TestResolvePlacement_StatErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions")
	}

	out := t.TempDir()
	dir := filepath.Join(out, string(model.CategoryText))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	_, err := resolvePlacement(out, model.CategoryText, "notes.txt", 5, "deadbeef")
	if err == nil {
		t.Fatal("expected a stat error, got nil")
	}
}

func TestResolvePlacement_HashFileErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores file permissions")
	}

	out := t.TempDir()
	dir := filepath.Join(out, string(model.CategoryText))
	existing := filepath.Join(dir, "notes.txt")
	writeFile(t, existing, "same size!")
	if err := os.Chmod(existing, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(existing, 0o644)

	_, err := resolvePlacement(out, model.CategoryText, "notes.txt", int64(len("same size!")), "deadbeef")
	if err == nil {
		t.Fatal("expected a hashFile error, got nil")
	}
}

func TestResolvePlacement_ExhaustsCollisionAttempts(t *testing.T) {
	original := maxCollisionAttempts
	maxCollisionAttempts = 2
	defer func() { maxCollisionAttempts = original }()

	out := t.TempDir()
	dir := filepath.Join(out, string(model.CategoryText))
	writeFile(t, filepath.Join(dir, "notes.txt"), "AAAA")
	writeFile(t, filepath.Join(dir, "notes_1.txt"), "BBBB")
	writeFile(t, filepath.Join(dir, "notes_2.txt"), "CCCC")

	// Target content/size differs from all three existing files, so every
	// slot up to maxCollisionAttempts is a genuine (non-matching) collision.
	_, err := resolvePlacement(out, model.CategoryText, "notes.txt", 5, "deadbeef")
	if err == nil {
		t.Fatal("expected an exhausted-collisions error, got nil")
	}
}

func hashFileContent(t *testing.T, content string) (hash string, size int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.tmp")
	writeFile(t, path, content)
	h, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	return h, int64(len(content))
}
