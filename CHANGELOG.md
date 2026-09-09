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

### Fixed
- Review 01: verified plaintext transfers, create-only destinations, explicit overwrite permission, self-transfer rejection, protection metadata/tag preservation, partial failure reporting, and byte-preserving file/stdin inputs.
- Review 02: AWS context precedence and explicit default-profile selection; configuration duration decoding, bounds validation, and atomic saves; clipboard ownership cleanup, fixed masking, escaped terminal output, and corrected IAM examples.
- Review 03: atomic cache transactions, tag freshness/completeness, partial inventory tracking, consistent glob matching, metadata-only inventory, and `list --tags` output.
- Review 04: stale asynchronous detail results, history/terminal bounds, tree navigation, editor terminal handoff, refresh lifecycle/status, mutation reconciliation, and lazy paginated history.

### Changed
- Browse detail starts masked; reveal/copy retrieves plaintext explicitly.
- `put` positional input is literal. Use `--file` or `--stdin` for exact file/input bytes; implicit filename detection and whitespace trimming are removed.
- `get --value` emits exact bytes without an added newline.
- Copy/move create destinations unless `--overwrite` is supplied; `--force` bypasses move confirmation independently of output format.
- The local `make release` target was removed; release archives are now produced exclusively by GoReleaser in CI.

### Security
- Every GitHub Action pinned to a full commit SHA (with the major version in a trailing comment); Dependabot will keep the SHAs current.

[Unreleased]: https://github.com/yachiko/clerk/compare/HEAD...HEAD
