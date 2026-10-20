# Sortinel

**Sortinel** deep-scans a directory tree, identifies every file by its actual
content (not its extension), and copies it into a tidy, type-organized
backup — producing a spreadsheet report of exactly what happened to every
single file along the way.

Point it at a messy folder (an old hard drive, a Downloads directory, a
decade of "Backup (2) (final) (copy)" folders) and get back:

```
backup_out/
├── Images/
├── Videos/
├── Audio/
├── Archives/
├── Code/
├── Documents/
│   ├── Word/
│   ├── PDF/
│   ├── Excel/
│   ├── PowerPoint/
│   └── Text/
├── Other/
└── backup_report_20260101_120000.xlsx
```

## Features

- **Deep recursive scan** of the entire source tree, arbitrarily nested.
- **Content-based classification** — files are identified by magic
  bytes/MIME sniffing, so a `.jpg` that's secretly a PNG still ends up in
  `Images/`, and a mislabeled `report.doc` that's actually a PDF ends up in
  `Documents/PDF/`.
- **Never overwrites, never loses a file.** Name collisions are
  auto-renamed (`photo.jpg` → `photo_1.jpg`), and the report records *why*.
- **Resumable.** If a run is interrupted, re-running it recognizes files
  that were already copied (by SHA-256 content hash, not just name) and
  skips them instead of duplicating work.
- **Symlinks are never followed** — they're reported as skipped, so you
  can't accidentally loop forever or copy things from outside the source
  tree.
- **Parallel copying** with a configurable worker pool.
- **Live progress with ETA.** A metadata-only pre-scan sizes up the whole
  job first, then a progress bar (CLI) or progress widget (GUI) tracks
  bytes/files done and an estimated time to completion based on observed
  throughput.
- **One spreadsheet report per run** (`.xlsx`, opens natively in Excel,
  Google Sheets and LibreOffice) with one row per source file: where it
  came from, where it went, its detected type, its status, and — for
  anything that wasn't a plain success — why.
- **CLI and GUI in the same binary.** Script it, or point-and-click it.
- Single static binary, no runtime dependencies, builds for Windows, macOS
  and Linux.

## Installation

Download a prebuilt binary for Linux, macOS or Windows from the
[Releases page](https://github.com/gustavoaveiro/sortinel/releases) —
each release has a `.tar.gz`/`.zip` per OS/arch plus a `checksums.txt`.

Or, with Go installed:

```sh
go install github.com/gustavoaveiro/sortinel/cmd/sortinel@latest
```

Or build from source:

```sh
git clone https://github.com/gustavoaveiro/sortinel.git
cd sortinel
go build -o sortinel ./cmd/sortinel
```

This builds the **CLI only**. See [Building the GUI](#building-the-gui)
below for the desktop interface.

## Usage

```sh
sortinel -in /path/to/messy/folder -out /path/to/organized/backup
```

```
Usage of sortinel:
  -dry-run
        scan and classify files without copying them
  -gui
        launch the graphical interface instead of running from flags
  -in string
        source directory to scan (required unless -gui)
  -out string
        destination directory for the organized backup (required unless -gui)
  -report string
        path for the .xlsx report (default: <out>/backup_report_<timestamp>.xlsx)
  -version
        print the version and exit
  -workers int
        number of concurrent copy workers (default: number of CPUs)
```

### Examples

Back up and organize a folder, using 8 parallel workers:

```sh
sortinel -in ~/old-hard-drive -out ~/sorted-backup -workers 8
```

Preview what *would* happen — classify and hash every file, write the
report, but copy nothing:

```sh
sortinel -in ~/old-hard-drive -out ~/sorted-backup -dry-run
```

Re-run after an interruption — already-backed-up files (same content,
verified by checksum) are skipped automatically:

```sh
sortinel -in ~/old-hard-drive -out ~/sorted-backup
```

## The report

Every run produces an `.xlsx` file with one row per file encountered in the
source tree:

| Column | Meaning |
|---|---|
| Source Path | Where the file was found |
| Destination Path | Where it was copied to (blank if it wasn't) |
| File Name | Original file name |
| Detected Type | MIME type detected from content |
| Category | The folder it was sorted into |
| Status | `Success`, `Success (Renamed)`, `Already Backed Up`, `Skipped - Symlink`, or `Failed` |
| Reason | Why, for anything other than a plain `Success` |
| Size (bytes) | File size |
| SHA-256 | Content checksum |
| Timestamp | When it was processed |

Rows with `Status = Failed` are highlighted so problems are easy to spot at
a glance.

## Building the GUI

The desktop GUI is built with [Fyne](https://fyne.io) and is gated behind a
build tag so the CLI never pulls in a graphics toolchain:

```sh
go build -tags gui -o sortinel ./cmd/sortinel
./sortinel -gui
```

On Linux, Fyne needs OpenGL and X11/Wayland development headers installed
first, e.g. on Debian/Ubuntu:

```sh
sudo apt install libgl1-mesa-dev libxkbcommon-dev libwayland-dev wayland-protocols \
  libxrandr-dev libxinerama-dev libxcursor-dev libxi-dev libxxf86vm-dev
```

Windows and macOS builds generally work out of the box with a standard C
toolchain installed (MSVC/MinGW on Windows, Xcode command line tools on
macOS).

## How classification works

Files are identified by [gabriel-vasile/mimetype](https://github.com/gabriel-vasile/mimetype),
which inspects the actual bytes of each file (magic numbers, container
structure) rather than trusting its extension. This correctly tells apart
look-alike formats — e.g. `.docx`/`.xlsx`/`.pptx` are all ZIP containers
under the hood, but Sortinel reads far enough into each one to know which
Office format it actually is.

Source code files (`.go`, `.py`, `.ts`, etc.) and plain text are both
reported as `text/*` by content sniffing alone, so those are split by
extension into `Code/` vs `Documents/Text/`.

## Development

```sh
go test ./...          # run the test suite
go build ./cmd/sortinel # build the CLI
gofmt -l .              # check formatting
```

## License

[MIT](LICENSE)
