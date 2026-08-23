---
name: melange
description: >
  Build the project's signed Wolfi apk with melange. Use when editing melange.yaml or its
  go/build pipeline, changing how version/commit/date are stamped into the binary, adding a
  build-time package, signing the apk, or debugging `mise run image-local` or the release
  `melange-build` job. This is the source-to-signed-apk step that apko later turns into the image.
---

# Melange

melange has exactly one job in this repo: compile the Go binary into a signed
[Wolfi](https://github.com/wolfi-dev) apk described by `melange.yaml`. That apk is the only
artifact apko assembles into the runtime image. There is no Dockerfile. Ground every command
in `--help` and the repo files below, not memory.

## Verified against

- melange `v0.54.0` (GitCommit `7fb1d6a`), pinned in `mise.toml` as
  `aqua:chainguard-dev/melange = "0.54.0"` and locked per-platform in `mise.lock`. Run it via
  mise (`mise exec -- melange ...`) or an activated shell; do not install it any other way.
- Grounded in local `melange --help` / `melange build --help` and the repo files: `melange.yaml`,
  `mise.toml` (`[tasks.image-local]`), `.github/workflows/release.yml`, `release-dry-run.yml`,
  and `security-scan.yml`.
- Sibling skills: `mise` provisions melange and puts it on PATH; `apko` consumes the apk this
  step produces. See those skills for their lanes.

## Use this skill when

- Editing `melange.yaml` or the `go/build` pipeline.
- Changing how `version` / `commit` / `date` reach the binary.
- Adding a build-time package or toolchain to the apk.
- Signing an apk or reasoning about the melange-to-apko key handoff.
- Debugging a failed `mise run image-local` or the release/dry-run melange build job.

## melange's lane (non-negotiable)

1. melange does ONE thing: source to signed Wolfi apk. It does not build the image (that is
   apko) and it does not push anything anywhere.
2. The apk is the single artifact. apko reads it from `./packages` via `@local`; nothing else
   consumes, tags, or distributes it.
3. Never hand-edit `package.version` in `melange.yaml`. It carries `# x-release-please-version`
   and release-please owns it. Do not run `melange bump` in this repo.
4. Inject `version` / `commit` / `date` ONLY through `--vars-file`. Never edit the `ldflags`
   line per build and never add date math inside the sandbox. The defaults under `vars:`
   (`0.0.0` / `none` / `unknown`) exist only so a bare build does not fail.
5. Never commit signing keys. `melange*.rsa`, `melange*.rsa.pub`, `melange-vars.yaml`,
   `.melange-vars.local.yaml`, and `/packages/` are gitignored. Keys are ephemeral, minted per
   build; the private key never leaves the machine that built the apk.
6. No Dockerfile, no `RUN`, no `apt`. Build-time tools come from
   `environment.contents.packages` (the `go-1.26` Wolfi package). Runtime dependencies belong
   in `apko.yaml`, not here.
7. Always pass `--runner docker`. Do not rely on the platform default runner.

## melange.yaml anatomy

- `package`: `name: template-go`, `version: "0.1.1"` (release-please marker), `epoch: 0`.
- `environment.contents`: the Wolfi `os` repository + its signing keyring, plus
  `packages: [go-1.26]` (the build toolchain). `environment.environment.CGO_ENABLED: "0"`.
- `vars`: build-time defaults overridden by `--vars-file`.
- `pipeline: - uses: go/build` with `packages: ./cmd/template-go`,
  `output: template-go`, `go-package: go-1.26`, `modroot: .`, `strip: "-s -w"`, the
  `ldflags` that stamp `main.version/commit/date` from `${{vars.*}}`, and
  `extra-args: "-mod=readonly -buildvcs=false"`. The `go/build` builtin auto-adds `-trimpath`
  and installs to `/usr/bin/<output>` — apko's entrypoint `/usr/bin/template-go` depends on
  that path. The module source mounts in from `--source-dir`.

## Build the apk locally

`mise run image-local` is the supported local path; it runs melange then apko. The melange
portion is:

```bash
melange keygen melange.rsa
printf 'version: "dev"\ncommit: "%s"\ndate: "%s"\n' \
  "$(git rev-parse --short HEAD)" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > .melange-vars.local.yaml
melange build melange.yaml \
  --arch "$(go env GOARCH)" \
  --runner docker \
  --signing-key melange.rsa \
  --source-dir . \
  --vars-file .melange-vars.local.yaml
```

This drops a signed apk under `./packages/<apkdir>/` (default `--out-dir` is `./packages/`).
`<apkdir>` is the Wolfi arch name, not the Go arch: `amd64` to `x86_64`, `arm64` to `aarch64`.
apko then reads the whole `./packages` directory as the `@local` repository.

## How the release / CI build differs

`release.yml` (`melange-build`) and `release-dry-run.yml` run the same `melange build`
invocation under a matrix, one arch per NATIVE runner — `amd64` on `ubuntu-24.04`, `arm64` on
`ubuntu-24.04-arm`. No QEMU. Differences from local:

- Each runner mints its own ephemeral key with a distinct name: `melange keygen melange-<arch>.rsa`.
- The vars file (`melange-vars.yaml`) carries the real release `version`, the full
  `git rev-parse HEAD`, and the committer date `git show -s --format=%cI HEAD`.
- Each runner uploads `packages/<apkdir>/**` plus its `melange-<arch>.rsa.pub`. The private key
  never leaves the runner; apko later trusts the apk via the uploaded public keys.

`security-scan.yml` builds only `amd64` for the weekly Trivy scan — the same
`melange build`, but it mints a single `melange.rsa` key (not the per-arch
`melange-<arch>.rsa` naming above).

## Signing and the apko handoff

`--signing-key` signs the apk during the build. apko must trust that signature to install the
`@local` apk, so the matching public key is appended to apko's keyring with
`--keyring-append`. Locally that is `--keyring-append ./melange.rsa.pub`; in the release job apko
appends both arches' keys (`./melange-amd64.rsa.pub`, `./melange-arm64.rsa.pub`). Omit the public
key and apko rejects the apk as untrusted. See the `apko` skill.

## Gotchas

- `--runner docker` is required wherever melange runs: it needs a Linux build sandbox, and on
  macOS/Docker Desktop the Docker runner provides it. Docker must be running. Valid runners are
  `bubblewrap`, `docker`, `qemu`; this repo always uses `docker`.
- Wolfi arch name is not the Go arch in output paths: `amd64`→`x86_64`, `arm64`→`aarch64`. Match
  these when globbing `packages/<apkdir>/**`.
- melange produces an SBOM for the apk (`--namespace` sets its package-URL namespace). The
  image-level SBOM and SLSA provenance are produced later by syft + `attest.yml`, NOT by melange.
  Do not add `--generate-provenance` to chase that; the repo does not use it.
- Build metadata flows version/commit/date through `--vars-file` into `ldflags` only. Neither
  local nor CI uses melange's `--build-date` flag (that controls in-image file timestamps for
  reproducibility, a different concern from the ldflag date).
- Do not override `output:` in the pipeline without updating `apko.yaml`'s `entrypoint.command`
  and `contents.packages` — the binary path `/usr/bin/template-go` is contractual.
- To add a build-time tool or a different Go toolchain, edit `environment.contents.packages`
  (Wolfi package names) in `melange.yaml`. Do not install inside the sandbox.

## Command reference

See [references/melange-commands.md](references/melange-commands.md) for the version-stamped
command and flag reference.
