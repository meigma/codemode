# template-go

`template-go` is the reusable Go repository starter for Meigma projects.
It includes a small Go CLI skeleton, Moon tasks, pinned CI, Dependabot, baseline repository security settings, and Release Please plus the reusable `meigma/release` release unit.

## Local Bootstrap

Prerequisites:

- [mise](https://mise.jdx.dev) — provisions every pinned tool from `mise.toml` +
  `mise.lock`: Go, Moon, Python + uv, `golangci-lint`, GoReleaser, GitHub CLI,
  Syft, Cosign, Melange, and apko. Run `mise install` once; there is nothing
  else to install by hand.

Tool versions live in `mise.toml`; `mise.lock` records a per-platform download URL
and checksum for each (and, for the aqua-backed CLIs, cosign/SLSA/GitHub-attestation
verification). `mise install` runs with `locked = true`, so it **fails closed** if a
tool lacks a pre-resolved, checksummed entry for the current platform. Moon runs every
task against these tools as `system` binaries on PATH and manages no toolchain itself.
To bump a tool, edit its version in `mise.toml`, run
`mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64`, and commit
`mise.toml` + `mise.lock`.

After creating a new repository from this template, replace the placeholder names before doing feature work:

```sh
go mod edit -module github.com/meigma/YOUR_REPO
mv cmd/template-go cmd/YOUR_BINARY
```

Then update `template-go` references in the Moon tasks, GoReleaser, Melange and apko configurations, README, and package docs.

## Common Tasks

Moon is the standard task front door:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
moon run root:check
```

CI runs the same aggregate check:

```sh
moon ci --summary minimal
```

The starter CLI is intentionally small:

```sh
go run ./cmd/template-go --version
go run ./cmd/template-go --message "hello from cobra"
go test ./...
```

The CLI entrypoint uses Cobra and Viper in the same shape as other Meigma CLIs: `cmd/template-go` stays thin, `internal/cli` owns command construction, and Viper-backed flags can also be supplied through `TEMPLATE_GO_*` environment variables.

## Container Image

The image is built without a Dockerfile. GoReleaser produces the canonical Linux
binaries. [Melange](https://github.com/chainguard-dev/melange) packages those
exact bytes into signed [Wolfi](https://github.com/wolfi-dev) APKs, and
[apko](https://github.com/chainguard-dev/apko) assembles a minimal,
multi-architecture, nonroot image. The runtime uses UID/GID 65532 and includes
CA certificates and timezone data.

Build and run a host-architecture image locally with Docker:

```sh
mise run image-local
docker run --rm template-go:dev --version
docker run --rm template-go:dev --message "hello from container"
```

The Wolfi base resolves current packages during each build. The generated SBOM
and provenance record the resolved contents. GoReleaser stamps `version`,
`commit`, and `date` into the release binary; `mise run image-local` uses
development values.

## CI and Security

The default CI workflow keeps permissions minimal, pins external actions, disables checkout credential persistence, and delegates checks to Moon.
It uses GitHub-hosted dependency caches for Go, golangci-lint, and uv download artifacts while leaving Moon remote caching as an optional follow-up for repositories that need a shared task-output cache.
The docs workflow builds the MkDocs site on pull requests and deploys `docs/build` to GitHub Pages from the default branch.
The scheduled security scan workflow builds the local container image weekly, scans it for high/critical fixed vulnerabilities, and uploads SARIF results to GitHub code scanning.
Dependabot covers GitHub Actions, the root Go module, and the docs uv project.

Repository settings live in `.github/repository-settings.toml`.
They default to immutable releases, private vulnerability reporting, signed commits, squash-only merges, GitHub Pages workflow publishing, and protected tags.

## Release Layer

Release automation is enabled for the template application so the template
proves the binary and OCI paths before generated projects inherit them.

The release path is:

- Release Please maintains the release PR and creates a stable `vMAJOR.MINOR.PATCH`
  tag plus a draft GitHub Release after merge.
- `.github/workflows/release.yml` calls one immutable
  [`meigma/release`](https://github.com/meigma/release) release unit pinned by
  full commit SHA.
- The release unit builds and verifies archives, native packages, checksums,
  SBOMs, and the checksum Sigstore bundle.
- The same canonical Linux binaries become a signed, attested
  `ghcr.io/meigma/template-go` image for `amd64` and `arm64`.
- The release unit uploads the verified assets and publishes the GitHub Release
  only after the OCI path succeeds.

Generated repositories must update names and metadata, pin one reviewed
`meigma/release` revision across every workflow and signer reference, and
rehearse with both publication inputs disabled before their first public
release. See [`DELETE_ME.md`](DELETE_ME.md) for the setup checklist.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines, local setup expectations, and pull request workflow.

## Security

See [SECURITY.md](SECURITY.md) for supported versions and the private vulnerability reporting path.

## License

Add the repository license before publishing a project generated from this template.
