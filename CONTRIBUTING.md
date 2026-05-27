# Contributing to Clerk

Thanks for your interest in improving Clerk. This guide covers what you need to develop, test, and submit changes.

## Prerequisites

| Tool   | Version | Why                                                       |
| ------ | ------- | --------------------------------------------------------- |
| Go     | 1.25+   | Matches `go.mod`.                                         |
| Docker | any     | Spins up [moto](https://github.com/getmoto/moto) for integration tests. |
| Make   | any     | Driver for all common tasks.                              |
| AWS CLI| any     | Optional — handy for verifying Parameter Store state.     |

## Local Development Workflow

```bash
make build              # Build ./bin/clerk
make dev                # Build for the current OS/arch with no optimizations
make test               # Unit tests (-race -short)
make test-verbose       # Unit tests with per-test output
make coverage           # Unit tests with HTML coverage report
make lint               # golangci-lint
make vet                # go vet ./...
make fmt                # gofmt -s -w .
make fmt-check          # Fail if any file is not gofmt-clean
make deps               # go mod download && go mod tidy
make clean              # Remove bin/, dist/, coverage artifacts
```

Integration tests spin up a local moto server:

```bash
make test-integration         # Builds, starts moto, runs integration suite, stops moto
make test-integration-large   # The large-scale subset (hundreds of parameters)
make bench-integration        # Integration benchmarks
```

## Commit Style

We use [Conventional Commits](https://www.conventionalcommits.org/). Common types:

- `feat:` — new functionality
- `fix:` — bug fix
- `chore:` — maintenance, dependency bumps, repo hygiene
- `docs:` — documentation only
- `ci:` — workflow or release-tooling changes
- `refactor:` — internal cleanup with no behavior change
- `perf:` — performance improvement
- `test:` — adding or refactoring tests

Scope where it adds clarity: `feat(browse): …`, `fix(cache): …`, `feat(cli): …`.

Keep commits small and focused — one logical change per commit. Branch from `main`.

## Pull Request Checklist

- [ ] `make lint test build` is clean.
- [ ] `make test-integration` is clean if you touched the SSM client or browse cache.
- [ ] User-facing changes have a `CHANGELOG.md` entry under `## [Unreleased]`.
- [ ] Behavior changes that affect documented commands or flags update the matching section in `README.md`.

## Releasing

Releases are tag-driven. To cut the next patch:

```bash
make tag        # Computes vX.Y.(Z+1) from the latest git tag, tags HEAD, pushes.
```

The push triggers `.github/workflows/release.yml`, which runs GoReleaser, signs the checksum file with keyless cosign, and uploads a SLSA v1.0 provenance attestation.

## Reporting Security Issues

Please don't open a public issue for security vulnerabilities. See `SECURITY.md` for the disclosure process.

## Code of Conduct

Be respectful. Disagreement is fine; personal attacks are not.

## License

By contributing you agree your contributions are licensed under the project's [MIT License](LICENSE).
