package report

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

func sampleRecords() []model.Record {
	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	return []model.Record{
		{
			SourcePath: "/in/a.pdf", DestPath: "/out/Documents/PDF/a.pdf", FileName: "a.pdf",
			DetectedType: "application/pdf", Category: model.CategoryPDF, Status: model.StatusSuccess,
			SizeBytes: 123, SHA256: "abc123", Timestamp: ts,
		},
		{
			SourcePath: "/in/b.bin", DestPath: "", FileName: "b.bin",
			Status: model.StatusFailed, Reason: "permission denied", Timestamp: ts,
		},
	}
}

func TestWrite_ProducesReadableWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.xlsx")

	if err := Write(path, sampleRecords()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	header, err := f.GetRows(sheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(header) != 3 { // 1 header + 2 data rows
		t.Fatalf("got %d rows, want 3: %+v", len(header), header)
	}
	if header[0][0] != "Source Path" {
		t.Errorf("header[0][0] = %q, want %q", header[0][0], "Source Path")
	}
	if header[1][2] != "a.pdf" {
		t.Errorf("row 1 FileName = %q, want %q", header[1][2], "a.pdf")
	}
	if header[2][5] != string(model.StatusFailed) {
		t.Errorf("row 2 Status = %q, want %q", header[2][5], model.StatusFailed)
	}
}

func TestWrite_EmptyRecordsProducesHeaderOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xlsx")

	if err := Write(path, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1 (header only)", len(rows))
	}
}

func TestWrite_InvalidPathFails(t *testing.T) {
	// A directory can't be overwritten by SaveAs.
	dir := t.TempDir()
	err := Write(dir, sampleRecords())
	if err == nil {
		t.Fatal("expected an error when writing to a directory path, got nil")
	}
}

func TestMustCol(t *testing.T) {
	if got := mustCol(1); got != "A" {
		t.Errorf("mustCol(1) = %q, want %q", got, "A")
	}
	if got := mustCol(27); got != "AA" {
		t.Errorf("mustCol(27) = %q, want %q", got, "AA")
	}
}
