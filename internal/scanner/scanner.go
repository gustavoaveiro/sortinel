// Package scanner walks a source directory tree and streams back every
// regular file it finds. Symbolic links are never followed - they are
// reported back as skipped entries instead.
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Kind identifies what a scanner Entry represents.
type Kind int

const (
	// KindFile is a regular, readable file ready to be classified/copied.
	KindFile Kind = iota
	// KindSkippedSymlink is a symbolic link that was not followed.
	KindSkippedSymlink
	// KindError is a path the walk could not stat/read (e.g. permission denied).
	KindError
)

// Entry is one item produced while walking the source tree.
type Entry struct {
	Path string
	Info fs.FileInfo // nil for KindError
	Kind Kind
	Err  error // set for KindError
}

// Walk recursively walks root and sends one Entry per file, symlink or
// unreadable path found. The returned channel is closed once the walk is
// complete. Directories themselves never produce an Entry.
func Walk(root string) <-chan Entry {
	out := make(chan Entry)

	go func() {
		defer close(out)
		walkFS(os.DirFS(root), root, out)
	}()

	return out
}

// Count walks root and returns the number of entries found and the total
// size of regular files among them. It's meant for a fast, metadata-only
// pre-scan so callers (e.g. a progress reporter) know the total amount of
// work before the real pass over the same tree begins.
func Count(root string) (files int, totalBytes int64) {
	for e := range Walk(root) {
		files++
		if e.Kind == KindFile {
			totalBytes += e.Info.Size()
		}
	}
	return files, totalBytes
}

// walkFS is the injectable core of Walk: fsys supplies directory entries,
// while root is only used to reconstruct absolute paths for Entry.Path.
// Separating it from Walk lets tests exercise fs.DirEntry failure modes
// that can't happen with the real OS filesystem but are part of the
// io/fs.DirEntry contract.
func walkFS(fsys fs.FS, root string, out chan<- Entry) {
	_ = fs.WalkDir(fsys, ".", func(relPath string, d fs.DirEntry, err error) error {
		path := filepath.Join(root, relPath)

		if err != nil {
			out <- Entry{Path: path, Kind: KindError, Err: err}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			out <- Entry{Path: path, Kind: KindError, Err: infoErr}
			return nil
		}

		if info.Mode()&fs.ModeSymlink != 0 {
			out <- Entry{Path: path, Info: info, Kind: KindSkippedSymlink}
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		out <- Entry{Path: path, Info: info, Kind: KindFile}
		return nil
	})
}
