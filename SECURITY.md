# Security Policy

## Supported Versions

Clerk is pre-1.0. Only the latest `0.x` release receives security fixes.

| Version | Supported |
| ------- | --------- |
| 0.x     | ✅        |
| < 0.1.0 | ❌        |

## Reporting a Vulnerability

Please report security vulnerabilities **privately** via GitHub Security Advisories:

1. Open the project's [Security tab](https://github.com/yachiko/clerk/security/advisories).
2. Click **Report a vulnerability**.
3. Provide a clear description, reproduction steps, and the impact you observed.

Do not open a public issue or PR for suspected vulnerabilities.

## What to Expect

- **Acknowledgement** within 5 business days.
- **Initial assessment** (severity, scope, affected versions) within 10 business days.
- **Fix or mitigation timeline** communicated once the assessment is complete.
- **Public disclosure** coordinated with the reporter; advisory and patched release published together.

## Threat Model

Clerk is a local CLI / TUI that reads and writes AWS Systems Manager Parameter Store entries using the credentials in your standard AWS SDK credential chain. The relevant threat surfaces are:

- **AWS credentials.** Clerk inherits whatever the SDK resolves (`~/.aws/credentials`, IMDS, env vars). It never persists credentials and never logs them.
- **Local cache.** The browse cache lives under `~/.clerk/cache.json` and respects `cache_ttl`. Secret values are stored in memory only; the cache holds parameter names and metadata, not plaintext values.
- **Clipboard.** When a value is copied, the clipboard is cleared after `clipboard_timeout` (default 60s).
- **Config file.** `~/.clerk/config.json` stores user preferences (region, profile, defaults). It never stores secrets.

Clerk does not open network listeners and does not perform authentication on its own; it relies entirely on AWS IAM.

## Out of Scope

- Compromise of the host machine.
- Misconfigured IAM policies that grant overly broad SSM access.
- Bugs in the AWS SDK or upstream dependencies after Dependabot has had a chance to update them.
- Side-channel observation of values briefly held in the terminal scrollback.

## Defensive Measures

- The binary is statically built (`CGO_ENABLED=0`) and shipped with `-s -w` stripped symbols.
- Every release archive is accompanied by a `checksums.txt`, a keyless cosign signature, and a SLSA v1.0 provenance attestation.
- CodeQL `security-and-quality` query pack runs on every push/PR and weekly.
- All GitHub Actions in workflows are pinned to commit SHAs; Dependabot keeps them current.

## Verifying a Release

Every tagged release ships:

- Platform archives (`.tar.gz` / `.zip`) for linux/darwin × amd64/arm64 and windows/amd64.
- A `checksums.txt` listing the SHA-256 of every archive.
- A keyless [cosign](https://github.com/sigstore/cosign) signature of `checksums.txt` (`checksums.txt.sig` + `checksums.txt.pem`), recorded in the Rekor transparency log.
- A [SLSA v1.0](https://slsa.dev/) build provenance attestation (`*.intoto.jsonl`) covering all artifacts.

### Verify with cosign

```bash
TAG=v0.1.0   # adjust
curl -sLO https://github.com/yachiko/clerk/releases/download/${TAG}/checksums.txt
curl -sLO https://github.com/yachiko/clerk/releases/download/${TAG}/checksums.txt.sig
curl -sLO https://github.com/yachiko/clerk/releases/download/${TAG}/checksums.txt.pem

cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp "https://github.com/yachiko/clerk/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Then verify the archive you downloaded against `checksums.txt`:

```bash
sha256sum -c checksums.txt --ignore-missing
```

### Verify SLSA provenance

```bash
# Install slsa-verifier: https://github.com/slsa-framework/slsa-verifier
slsa-verifier verify-artifact \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/yachiko/clerk \
  --source-tag ${TAG} \
  clerk_0.1.0_linux_amd64.tar.gz
```
