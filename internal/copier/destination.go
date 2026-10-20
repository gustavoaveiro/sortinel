package copier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

// maxCollisionAttempts bounds how many "name_N.ext" variants are tried
// before giving up on a destination slot for a file. Declared as a var
// (rather than const) so tests can shrink it to exercise the exhaustion path
// without creating thousands of files.
var maxCollisionAttempts = 10000

// placement describes where a file ended up (or why it didn't move) after
// resolving name collisions against files already present in outDir.
type placement struct {
	path   string
	status model.Status
	reason string
}

// resolvePlacement finds the destination path for a source file of the
// given size/hash under outDir/category/fileName, auto-renaming on name
// collisions and recognizing files that were already backed up in a
// previous run (same name, same content).
func resolvePlacement(outDir string, category model.Category, fileName string, size int64, hash string) (placement, error) {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	if base == "" {
		// A dotfile with no further dots (e.g. ".hidden"): filepath.Ext
		// treats the whole name as the "extension", which would otherwise
		// rename a collision to "_1.hidden" instead of ".hidden_1". Treat
		// the full name as the base instead, so the dotfile's identity is
		// preserved and the suffix lands where a human would expect it.
		base = fileName
		ext = ""
	}
	dir := filepath.Join(outDir, string(category))

	for n := 0; n <= maxCollisionAttempts; n++ {
		candidateName := fileName
		if n > 0 {
			candidateName = fmt.Sprintf("%s_%d%s", base, n, ext)
		}
		candidate := filepath.Join(dir, candidateName)

		info, err := os.Stat(candidate)
		switch {
		case os.IsNotExist(err):
			if n == 0 {
				return placement{path: candidate, status: model.StatusSuccess}, nil
			}
			return placement{
				path:   candidate,
				status: model.StatusRenamed,
				reason: fmt.Sprintf("name collision with an existing, different file at %s", filepath.Join(dir, fileName)),
			}, nil
		case err != nil:
			return placement{}, err
		}

		if info.Size() == size {
			existingHash, err := hashFile(candidate)
			if err != nil {
				return placement{}, err
			}
			if existingHash == hash {
				return placement{
					path:   candidate,
					status: model.StatusAlreadyBackedUp,
					reason: fmt.Sprintf("identical file already backed up at %s", candidate),
				}, nil
			}
		}
		// Same name, different content: try the next suffix.
	}

	return placement{}, fmt.Errorf("could not find a free destination name after %d attempts", maxCollisionAttempts)
}
