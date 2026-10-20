package report

import (
	"fmt"
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

// fakeExcelFile implements excelFile and fails exactly the one call named
// by failOn, succeeding at everything else. It exists to exercise
// writeWorkbook's error-propagation branches, which can't be triggered
// through the real *excelize.File with the fixed, always-valid arguments
// writeWorkbook passes it.
type fakeExcelFile struct {
	failOn string
}

func (f *fakeExcelFile) fail(method string) error {
	if f.failOn == method {
		return fmt.Errorf("simulated %s failure", method)
	}
	return nil
}

func (f *fakeExcelFile) SetSheetName(source, target string) error   { return f.fail("SetSheetName") }
func (f *fakeExcelFile) SetCellStr(sheet, cell, value string) error { return f.fail("SetCellStr") }
func (f *fakeExcelFile) SetCellValue(sheet, cell string, value interface{}) error {
	return f.fail("SetCellValue")
}
func (f *fakeExcelFile) NewStyle(style *excelize.Style) (int, error) {
	if err := f.fail("NewStyle"); err != nil {
		return 0, err
	}
	return 1, nil
}
func (f *fakeExcelFile) SetCellStyle(sheet, topLeftCell, bottomRightCell string, styleID int) error {
	return f.fail("SetCellStyle")
}
func (f *fakeExcelFile) AutoFilter(sheet, rangeRef string, opts []excelize.AutoFilterOptions) error {
	return f.fail("AutoFilter")
}
func (f *fakeExcelFile) SaveAs(name string, opts ...excelize.Options) error { return f.fail("SaveAs") }
func (f *fakeExcelFile) Close() error                                       { return nil }

func TestWriteWorkbook_PropagatesEachUnderlyingError(t *testing.T) {
	// One Failed-status record so the SetCellStyle (highlight) path runs too.
	records := []model.Record{{Status: model.StatusFailed}}

	for _, method := range []string{
		"SetSheetName", "SetCellStr", "NewStyle", "SetCellValue", "SetCellStyle", "AutoFilter",
	} {
		t.Run(method, func(t *testing.T) {
			err := writeWorkbook(&fakeExcelFile{failOn: method}, records)
			if err == nil {
				t.Fatalf("expected an error when %s fails, got nil", method)
			}
		})
	}
}

func TestWriteWorkbook_SucceedsWhenNothingFails(t *testing.T) {
	if err := writeWorkbook(&fakeExcelFile{}, sampleRecords()); err != nil {
		t.Fatalf("writeWorkbook: %v", err)
	}
}

func TestWriteFile_PropagatesWriteWorkbookError(t *testing.T) {
	err := writeFile(&fakeExcelFile{failOn: "SetSheetName"}, "ignored.xlsx", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestWriteFile_PropagatesSaveAsError(t *testing.T) {
	err := writeFile(&fakeExcelFile{failOn: "SaveAs"}, "ignored.xlsx", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
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
