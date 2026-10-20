package classifier

import (
	"errors"
	"strings"
	"testing"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

func TestClassify_PropagatesReadError(t *testing.T) {
	_, _, err := Classify(erroringReader{}, "whatever.bin")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestClassify(t *testing.T) {
	pdfHeader := "%PDF-1.4\n%fake pdf content for detection\n"
	pngHeader := string([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) + "rest"
	zipHeader := string([]byte{'P', 'K', 0x03, 0x04}) + "rest of a zip-like file"
	flvHeader := string([]byte{0x46, 0x4C, 0x56, 0x01}) + "rest of a flash video"
	wavHeader := "RIFF" + "\x24\x00\x00\x00" + "WAVE" + "fmt rest of a wav file"

	tests := []struct {
		name     string
		content  string
		fileName string
		want     model.Category
	}{
		{"pdf", pdfHeader, "report.pdf", model.CategoryPDF},
		{"png image", pngHeader, "photo.png", model.CategoryImages},
		{"zip archive", zipHeader, "bundle.zip", model.CategoryArchives},
		{"flv video", flvHeader, "clip.flv", model.CategoryVideos},
		{"wav audio", wavHeader, "sound.wav", model.CategoryAudio},
		{"plain text", "just some plain text content", "notes.txt", model.CategoryText},
		{"go source as text", "package main\n\nfunc main() {}\n", "main.go", model.CategoryCode},
		{"csv", "a,b,c\n1,2,3\n", "data.csv", model.CategoryExcel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mimeType, category, err := Classify(strings.NewReader(tc.content), tc.fileName)
			if err != nil {
				t.Fatalf("Classify returned error: %v", err)
			}
			if mimeType == "" {
				t.Error("expected a non-empty detected MIME type")
			}
			if category != tc.want {
				t.Errorf("category = %q, want %q (mime=%s)", category, tc.want, mimeType)
			}
		})
	}
}

func TestClassifyBytes(t *testing.T) {
	mimeType, category := ClassifyBytes([]byte("%PDF-1.4\nfake pdf\n"), "report.pdf")
	if category != model.CategoryPDF {
		t.Errorf("category = %q, want %q", category, model.CategoryPDF)
	}
	if mimeType == "" {
		t.Error("expected a non-empty detected MIME type")
	}
}

func TestClassify_UnknownBinaryFallsBackToOther(t *testing.T) {
	// Bytes that don't match any known signature.
	content := string([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	_, category, err := Classify(strings.NewReader(content), "mystery.bin")
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if category != model.CategoryOther {
		t.Errorf("category = %q, want %q", category, model.CategoryOther)
	}
}

func TestExtOf(t *testing.T) {
	cases := map[string]string{
		"file.txt":       ".txt",
		"archive.tar.gz": ".gz",
		"noext":          "",
		"":               "",
	}
	for in, want := range cases {
		if got := extOf(in); got != want {
			t.Errorf("extOf(%q) = %q, want %q", in, got, want)
		}
	}
}
