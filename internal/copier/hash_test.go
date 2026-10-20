package copier

import (
	"errors"
	"path/filepath"
	"testing"
)

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

func TestHashReader_PropagatesReadError(t *testing.T) {
	_, err := hashReader(erroringReader{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestHashFile_MissingFile(t *testing.T) {
	_, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestHashFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeFile(t, path, "abc")

	hash, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	const wantSHA256OfABC = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if hash != wantSHA256OfABC {
		t.Errorf("hash = %q, want %q", hash, wantSHA256OfABC)
	}
}
