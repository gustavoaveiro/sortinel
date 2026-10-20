// Package model holds the shared types describing a scanned file and its
// outcome, independent of how the file was discovered, classified or copied.
package model

import "time"

// Status describes the outcome of processing a single source file.
type Status string

const (
	StatusSuccess         Status = "Success"
	StatusRenamed         Status = "Success (Renamed)"
	StatusAlreadyBackedUp Status = "Already Backed Up"
	StatusFailed          Status = "Failed"
	StatusSkippedSymlink  Status = "Skipped - Symlink"
)

// Category is a top-level bucket in the output directory.
type Category string

const (
	CategoryImages     Category = "Images"
	CategoryVideos     Category = "Videos"
	CategoryAudio      Category = "Audio"
	CategoryWord       Category = "Documents/Word"
	CategoryPDF        Category = "Documents/PDF"
	CategoryExcel      Category = "Documents/Excel"
	CategoryPowerPoint Category = "Documents/PowerPoint"
	CategoryText       Category = "Documents/Text"
	CategoryArchives   Category = "Archives"
	CategoryCode       Category = "Code"
	CategoryOther      Category = "Other"
)

// Record is one row of the final report: a single source file and what
// happened to it.
type Record struct {
	SourcePath   string
	DestPath     string
	FileName     string
	DetectedType string // MIME type, e.g. "application/pdf"
	Category     Category
	Status       Status
	Reason       string
	SizeBytes    int64
	SHA256       string
	Timestamp    time.Time
}
