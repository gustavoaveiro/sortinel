// Package classifier determines the Category of a file by inspecting its
// actual content (magic bytes) rather than trusting its extension.
package classifier

import (
	"io"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

// Classify reads the start of r (mimetype only needs a small header) and
// returns the detected MIME type together with the Category it maps to.
func Classify(r io.Reader, fileName string) (mimeType string, category model.Category, err error) {
	mt, err := mimetype.DetectReader(r)
	if err != nil {
		return "", "", err
	}
	return mt.String(), categoryFor(mt, fileName), nil
}

func categoryFor(mt *mimetype.MIME, fileName string) model.Category {
	for m := mt; m != nil; m = m.Parent() {
		if cat, ok := byExactMIME[m.String()]; ok {
			return cat
		}
	}

	switch {
	case strings.HasPrefix(mt.String(), "image/"):
		return model.CategoryImages
	case strings.HasPrefix(mt.String(), "video/"):
		return model.CategoryVideos
	case strings.HasPrefix(mt.String(), "audio/"):
		return model.CategoryAudio
	case strings.HasPrefix(mt.String(), "text/"):
		return categoryForCodeOrText(fileName)
	default:
		return model.CategoryOther
	}
}

// byExactMIME covers formats whose MIME type doesn't carry enough signal on
// its own (e.g. all Office Open XML formats are ZIP-based) or that shouldn't
// fall into the generic image/video/audio/text buckets.
var byExactMIME = map[string]model.Category{
	"application/pdf": model.CategoryPDF,

	"application/msword": model.CategoryWord,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": model.CategoryWord,

	"application/vnd.ms-excel": model.CategoryExcel,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": model.CategoryExcel,
	"text/csv": model.CategoryExcel,

	"application/vnd.ms-powerpoint":                                             model.CategoryPowerPoint,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": model.CategoryPowerPoint,

	"application/zip":              model.CategoryArchives,
	"application/x-tar":            model.CategoryArchives,
	"application/gzip":             model.CategoryArchives,
	"application/x-bzip2":          model.CategoryArchives,
	"application/x-7z-compressed":  model.CategoryArchives,
	"application/x-rar-compressed": model.CategoryArchives,
	"application/x-rar":            model.CategoryArchives,
}

// codeExtensions are source-code file extensions. mimetype reports these as
// generic "text/plain", so the extension is the only signal available.
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".java": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true,
	".rb": true, ".php": true, ".rs": true, ".swift": true, ".kt": true, ".scala": true,
	".sh": true, ".bash": true, ".zsh": true, ".sql": true, ".html": true, ".css": true,
	".scss": true, ".json": true, ".yaml": true, ".yml": true, ".xml": true, ".toml": true,
}

func categoryForCodeOrText(fileName string) model.Category {
	ext := strings.ToLower(extOf(fileName))
	if codeExtensions[ext] {
		return model.CategoryCode
	}
	return model.CategoryText
}

func extOf(fileName string) string {
	if i := strings.LastIndexByte(fileName, '.'); i >= 0 {
		return fileName[i:]
	}
	return ""
}
