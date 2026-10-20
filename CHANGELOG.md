# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial implementation of Sortinel: a CLI (and Fyne-based GUI, built with
  `-tags gui`) that deep-scans a source directory, classifies every file by
  content (magic bytes, not extension), copies it into a type-organized
  output directory, and writes an `.xlsx` report of every file processed.
- Content-based classification into Images, Videos, Audio, Archives, Code,
  and Documents/{Word,PDF,Excel,PowerPoint,Text}, with an `Other` fallback.
- Symlinks are detected and skipped (never followed), and reported as such.
- Name-collision handling: conflicting destination names are auto-renamed,
  with the reason recorded in the report.
- Resumable runs: files already present at the destination with matching
  SHA-256 content are recognized as `Already Backed Up` and not re-copied.
- Parallel copying via a configurable worker pool, and a `-dry-run` mode
  that classifies and hashes without copying.
- GitHub Actions CI that runs `gofmt`, `go vet`, the full test suite with
  the race detector, and cross-compiles the CLI for linux/darwin/windows on
  every push and pull request.
- 100% unit test coverage across all internal packages, enforced as a CI
  gate that fails the build if coverage drops.
- MIT license.
- Release automation: pushing a `v*` tag builds CLI binaries for
  linux/darwin (amd64+arm64) and windows/amd64 via GoReleaser and publishes
  them as a GitHub Release with checksums and auto-generated notes.

[Unreleased]: https://github.com/gustavoaveiro/sortinel/commits/master
