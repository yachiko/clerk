# Changelog

All notable changes to Clerk are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `CONTRIBUTING.md`, `CHANGELOG.md`, `SECURITY.md`, `LICENSE` (MIT).
- Dependabot config covering Go modules and GitHub Actions.
- CI workflow (`go vet`, `golangci-lint`, `go test -race`, `go build`) running on push and PR.
- CodeQL workflow (`security-and-quality` query pack) running on push, PR, and a weekly schedule.
- GoReleaser-based release workflow producing archives for linux/darwin (amd64 + arm64) and windows/amd64 on tag push.
- Keyless cosign signing of `checksums.txt` on every release.
- SLSA v1.0 build provenance attestation for every release artifact.
- `make tag` target that creates and pushes the next patch version tag.

### Changed
- The local `make release` target was removed; release archives are now produced exclusively by GoReleaser in CI.

### Security
- Every GitHub Action pinned to a full commit SHA (with the major version in a trailing comment); Dependabot will keep the SHAs current.

[Unreleased]: https://github.com/yachiko/clerk/compare/HEAD...HEAD
