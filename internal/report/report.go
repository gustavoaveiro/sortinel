// Package report writes a slice of model.Record as a .xlsx spreadsheet that
// opens in Excel, Google Sheets and LibreOffice.
package report

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/gustavoaveiro/sortinel/internal/model"
)

const sheetName = "Report"

var columns = []string{
	"Source Path", "Destination Path", "File Name", "Detected Type",
	"Category", "Status", "Reason", "Size (bytes)", "SHA-256", "Timestamp",
}

// Write renders records into a new .xlsx file at path, failing rows are
// highlighted in red.
func Write(path string, records []model.Record) error {
	f := excelize.NewFile()
	defer f.Close()

	if err := f.SetSheetName("Sheet1", sheetName); err != nil {
		return err
	}

	for i, col := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheetName, cell, col); err != nil {
			return err
		}
	}

	failedStyle, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#F8CBAD"}, Pattern: 1},
	})
	if err != nil {
		return err
	}

	for rowIdx, rec := range records {
		row := rowIdx + 2 // 1 header row, 1-indexed cells
		values := []interface{}{
			rec.SourcePath, rec.DestPath, rec.FileName, rec.DetectedType,
			string(rec.Category), string(rec.Status), rec.Reason, rec.SizeBytes,
			rec.SHA256, rec.Timestamp.Format("2006-01-02 15:04:05"),
		}
		for colIdx, v := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, row)
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return err
			}
		}
		if rec.Status == model.StatusFailed {
			firstCell, _ := excelize.CoordinatesToCellName(1, row)
			lastCell, _ := excelize.CoordinatesToCellName(len(columns), row)
			if err := f.SetCellStyle(sheetName, firstCell, lastCell, failedStyle); err != nil {
				return err
			}
		}
	}

	if err := f.AutoFilter(sheetName, fmt.Sprintf("A1:%s1", mustCol(len(columns))), nil); err != nil {
		return err
	}

	return f.SaveAs(path)
}

func mustCol(n int) string {
	col, _ := excelize.ColumnNumberToName(n)
	return col
}
