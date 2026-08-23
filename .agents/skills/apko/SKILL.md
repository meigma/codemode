---
name: apko
description: >
  Assemble the runtime OCI image for template-go with apko (`apko.yaml`) — the
  Dockerfile-free, multi-arch, nonroot image built from the melange-produced apk plus a
  minimal Wolfi base. Use when changing image contents, the `image-local` mise task, the
  `apko build`/`apko publish` steps in `.github/workflows/release.yml` or
  `security-scan.yml`, runtime packages, the nonroot user, OCI annotations, or the
  per-build SBOM. Pairs with the `melange` skill (the apk) and the `mise` skill (the CLI).
---

# apko

apko owns exactly one step in this repo: turn the melange-built apk plus a small set of
Wolfi base packages into the runtime image. It is the modern, distroless-equivalent
replacement for `gcr.io/distroless/static-debian12:nonroot` — no Dockerfile, no `RUN`, no
shell. The entire image is declared in `apko.yaml`; everything else (signing, attestation,
the apk itself) belongs to adjacent tools.

## Verified against

- apko `v1.2.19` (pinned in `mise.toml` as `aqua:chainguard-dev/apko`, locked in `mise.lock`).
- Grounded in the local `apko --help` for this version and the repo files
  (`apko.yaml`, `mise.toml`, `.github/workflows/release.yml`,
  `.github/workflows/security-scan.yml`), not from memory.
- Run apko through mise so the pinned binary is used: `mise exec -- apko <sub> --help`.

## Use this skill when

- Adding or removing a runtime dependency (a Wolfi package in `contents.packages`).
- Editing the nonroot account, entrypoint, archs, or OCI annotations in `apko.yaml`.
- Touching `apko build`/`apko publish` in the release or security-scan workflows, or the
  `image-local` mise task.
- Debugging the published image (wrong arch set, missing apk, untrusted `@local` package,
  SBOM directory errors).

## apko's lane (do not cross it)

1. The image is defined **only** in `apko.yaml`. There is no Dockerfile and there must not
   be one. Never add `RUN`, `apt`, `apk add`, shell steps, or a base-image `FROM`.
2. Add a runtime dependency by adding a **Wolfi package** to `contents.packages` — nothing
   else. Keep the set minimal; every package is CVE surface in a distroless image.
3. apko consumes the apk; it never builds it. Source → apk is the `melange` skill's job. If
   the app changed, rebuild the apk first, then apko.
4. apko does not sign or attest. `cosign sign` and the `attest.yml` provenance are separate
   release steps. Do not fold them into apko invocations.
5. Keep it nonroot. The image runs as uid/gid 65532 with no shell; do not add a shell,
   package manager, or root entrypoint for convenience.
6. Do not reintroduce `apko.lock.json` / `apko lock`. The Wolfi base floats by design (see
   below). Pinning is recorded in the per-build SBOM + provenance, not a committed lockfile.

## How the image is wired (`apko.yaml` anatomy)

Read `apko.yaml` before changing anything. The load-bearing parts:

- `contents.repositories`: the Wolfi os repo **and** `@local ./packages`. The `@local`
  repository is melange's output directory — that is how the just-built apk is found.
- `contents.keyring`: the Wolfi signing key URL. The **ephemeral melange public key(s)** are
  not listed here; they are appended at build/publish time with `--keyring-append` and are
  never committed. Without the matching pub key in the keyring, apko refuses the `@local`
  apk as unsigned.
- `contents.packages`: `wolfi-baselayout`, `ca-certificates-bundle`, `tzdata`, and
  `template-go@local`. The `@local` suffix pins the package to the `@local` repository,
  i.e. the apk melange just built (not anything from the Wolfi index).
- `accounts`: defines group+user `nonroot` (gid/uid **65532**) and `run-as: 65532`. Wolfi has
  no `nonroot` package, so the user is created here. This mirrors `distroless:nonroot`.
- `entrypoint.command: /usr/bin/template-go` — where the `go/build` melange pipeline
  installs the binary.
- `archs: [amd64, arm64]` — the index architectures.
- `annotations`: OCI labels. `org.opencontainers.image.version` carries the
  `# x-release-please-version` marker; release-please bumps it. Do not hand-edit it.

## Local build (the `image-local` mise task)

`mise run image-local` builds a single host-arch image and loads it into Docker as
`template-go:dev` for local running/inspection. The apko step is:

```bash
apko build apko.yaml template-go:dev image.tar \
  --arch "$arch" \
  --keyring-append ./melange.rsa.pub
docker load < image.tar
docker tag "template-go:dev-$arch" template-go:dev
```

Non-obvious points:

- **The retag is required, not cosmetic.** A single-arch `apko build` loads into Docker under
  an arch-suffixed tag (`template-go:dev-amd64` / `-arm64`, using the Go arch name from
  `go env GOARCH`). The task retags to the plain `template-go:dev` so it is easy to
  `docker run`. `security-scan.yml` does the same (`...:security-scan-amd64` → `...:security-scan`).
- `apko build` writes a tarball for `docker load`. The positional output can also be an
  `oci-layout-dir/`, but the repo uses a `.tar`.
- `--keyring-append ./melange.rsa.pub` makes apko trust the locally-signed `@local` apk. The
  local task mints one ephemeral key (`melange.rsa`); only its single pub key is appended.
- `--arch "$arch"` uses the explicit Go arch name. `host` is also accepted by apko, but the
  task passes the resolved value.
- melange must have run first (`packages/` must contain the apk). The task does this in order;
  if you run apko by hand, build the apk first — see the `melange` skill.

## CI publish (`release.yml` → multi-arch index)

The `container-image-release` job assembles and pushes the multi-arch image:

```bash
mkdir -p sbom            # apko does NOT create --sbom-path; it must pre-exist
apko publish apko.yaml "$IMAGE_TAG" \
  --arch amd64,arm64 \
  --keyring-append ./melange-amd64.rsa.pub \
  --keyring-append ./melange-arm64.rsa.pub \
  --sbom-path ./sbom \
  | tee apko-out.txt
```

Non-obvious points:

- **`--sbom-path` must pre-exist.** apko moves SBOMs into the directory but does not create
  it; a missing `sbom/` is a real release failure (baked-in `mkdir -p sbom` guards it).
- **Two keyring keys.** Each arch's apk was signed on its own native runner with its own
  ephemeral key, so both `melange-amd64.rsa.pub` and `melange-arm64.rsa.pub` must be appended;
  apko verifies each per-arch apk against the matching key.
- **`docker login` is a precondition.** `apko publish` authenticates via the Docker keychain.
  release.yml runs `docker/login-action` against `ghcr.io` first; without it, the push fails.
- **The digest is parsed from stdout.** apko prints the published refs/digest; release.yml
  greps `sha256:[0-9a-f]{64}` and takes the last match. `--image-refs <file>` can write the
  refs to a file instead, but the repo parses stdout.
- The image is pushed even while the GitHub release is still a draft — GHCR has no draft
  state. That is expected.
- apko emits its own SBOM (`--sbom-path ./sbom`, spdx by default). The **attested** image SBOM
  is generated separately by `syft <ref> -o spdx-json` and attached with `actions/attest-sbom`.
  Two different SBOMs; do not conflate them.

After publish, release.yml (adjacent steps, not apko's job): verifies the manifest is exactly
`linux/amd64,linux/arm64` via `docker buildx imagetools inspect`, smoke-tests `--version` and
`--message`, then `cosign sign --yes` (keyless), syft SBOM attestation, and finally the
isolated `attest.yml` SLSA-L3 provenance.

## Multi-arch model

apko does not emulate. It assembles a 2-arch index from per-arch apks that **melange already
built natively** (amd64 on `ubuntu-24.04`, arm64 on `ubuntu-24.04-arm`, no QEMU). The
`archs:` in `apko.yaml` and `--arch amd64,arm64` must line up with the apks present under
`packages/` (Wolfi arch dirs `x86_64`/`aarch64`). If an arch's apk is missing, publish fails.

## Read-only inspection

Use these to reason about the image without building it:

```bash
apko show-config apko.yaml      # the fully-derived config apko will act on
apko show-packages apko.yaml    # exact packages + versions that would install
```

`show-packages` resolves the live Wolfi index, so it shows what the floating base would pull
right now — the right tool to preview a CVE bump or confirm a new package resolves. Both
accept `--keyring-append`/`--repository-append` if you need them to see the `@local` apk.

## Why `apko lock` is deliberately unused

`apko lock` exists (it writes a `.lock.json` of pinned package versions) but the repo does not
use it, on purpose:

- The app package is a per-build `@local` apk; a committed lock would pin a stale/foreign
  checksum for it.
- The Wolfi base (`ca-certificates-bundle`, `tzdata`, …) is meant to float to latest for a
  fresh CA bundle/timezones and low CVE surface. Pinning fights that model.
- Reproducibility comes from recording the exact resolved versions in the per-build SBOM +
  provenance attestation, not from a lockfile. Do not add `apko.lock.json`.

## Gotchas

- Single-arch `apko build` Docker-loads under an **arch-suffixed tag**; retag before using it.
  (The suffix is the value passed to `--arch`; the repo passes the Go arch name, e.g.
  `-amd64`, not the Wolfi `-x86_64`.)
- `--sbom-path` directory must already exist.
- `--keyring-append` is mandatory for the `@local` apk and must cover **every** arch being
  published.
- `apko publish` needs a prior `docker login`; `apko build` (tarball) does not.
- `org.opencontainers.image.version` in `apko.yaml` is release-please-owned — never hand-edit.
- Adding a runtime dep means a Wolfi package in `contents.packages`, then rebuild the apk and
  the image. There is no Dockerfile to edit.
- apko is pinned by mise; invoke it via `mise exec -- apko …` (or a mise task / shimmed PATH),
  not a system install. Bumping apko is a `mise.toml` + `mise lock` change — see the `mise`
  skill.

See [references/apko-commands.md](references/apko-commands.md) for the version-stamped command
and flag reference.
