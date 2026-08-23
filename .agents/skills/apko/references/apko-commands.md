# apko Command Map

Curated operator reference for `apko v1.2.19` (pinned in `mise.toml` as
`aqua:chainguard-dev/apko`, locked in `mise.lock`). Prefer local `--help`
(`mise exec -- apko <sub> --help`) and the official docs if anything here drifts. Only the
flags this repo uses, plus the few an operator reaches for, are listed — every flag below is
real and correctly spelled/typed for this version.

## Global flags

Apply to every `apko` subcommand:

- `--log-level <string>`: `debug|info|warn|error|fatal|panic` (default `INFO`).
- `-C, --workdir <string>`: working directory (default is the current dir). apko searches for
  config, base image, and `@local` paths relative to this.
- `-h, --help`.

## `apko build`

Purpose: build an image from `apko.yaml` into a `docker load`-able tarball (or an
`oci-layout-dir/`). Used by the `image-local` mise task and `security-scan.yml`.

Usage:

```bash
apko build <config.yaml> <tag> <output.tar|oci-layout-dir/> [flags]
```

Flags that matter here:

- `--arch <strings>`: architectures to build (e.g. `amd64`, `x86_64`, or `amd64,arm64`).
  Accepts `host` for the host arch. Default is all archs in the config. The repo passes a
  single resolved arch for local/scan builds.
- `-k, --keyring-append <strings>`: extra public keys to trust. **Required** here to trust the
  ephemeral melange-signed `@local` apk (`--keyring-append ./melange.rsa.pub`). Repeatable.
- `--sbom`: generate SBOMs (default `true`; disable with `--sbom=false`).
- `--sbom-path <string>`: write SBOMs to this dir (default: the image dir). Must already exist.
- `--sbom-formats <strings>`: SBOM formats (default `[spdx]`).
- `--annotations <strings>`: OCI annotations as `key:value` (colon-separated). The repo
  declares annotations in `apko.yaml` instead.
- `-r, --repository-append <strings>`: extra package repositories.
- `-b, --build-repository-append <strings>`: extra repositories used only at build time.
- `-p, --package-append <strings>`: extra packages beyond `contents.packages`.
- `--build-date <string>`: timestamp (RFC3339) for files inside the image. Repo relies on the
  default; set only when you need a specific reproducible timestamp.
- `--offline`: do not fetch packages (cache must be pre-populated).
- `--cache-dir <string>`: apk/index cache directory.
- `--lockfile <string>`: constrain package versions to a `.lock.json`. Not used here (see the
  SKILL on why `apko lock` is avoided).
- `--include-paths <strings>`: extra paths to resolve input files from.
- `--ignore-signatures`: skip repository signature verification. Do not use; it defeats the
  `@local`/Wolfi signing checks.

Notes:

- A single-arch build loads into Docker under an **arch-suffixed tag** (`<tag>-<arch>`, Go arch
  name). Retag to the plain `<tag>` before running it.
- `build` has **no** `--local` or `--image-refs` flag — those are `publish`-only.

## `apko publish`

Purpose: build **and push** the (multi-arch) image to a registry, printing the published
refs/digest to stdout. Used by `release.yml` (`container-image-release`).

Usage:

```bash
apko publish <config.yaml> <tag...> [flags]
```

Flags that matter here:

- `--arch <strings>`: architectures for the index. Repo uses `--arch amd64,arm64`; must match
  the per-arch apks present under `packages/`.
- `-k, --keyring-append <strings>`: trust keys. Repo appends **both** ephemeral melange pub
  keys (`./melange-amd64.rsa.pub`, `./melange-arm64.rsa.pub`) — one per arch. Repeatable.
- `--sbom-path <string>`: write SBOMs here. **apko does not create this dir** — `mkdir -p`
  it first (real release bug otherwise).
- `--sbom` (default `true`; `--sbom=false` to disable), `--sbom-formats <strings>` (default `[spdx]`).
- `--image-refs <string>`: write the published refs to a file. Repo parses stdout instead.
- `--local`: publish only to the local Docker daemon (no registry push). Not used here.
- `--annotations <strings>`: OCI annotations `key:value`. Repo uses `apko.yaml`.
- `-r, --repository-append` / `-b, --build-repository-append` / `-p, --package-append`: as in
  `build`.
- `--build-date`, `--offline`, `--cache-dir`, `--lockfile`, `--ignore-signatures`: as in
  `build`.

Notes:

- Authenticates via the **Docker keychain** — run `docker login <registry>` first.
- Accepts multiple `<tag>` positionals; the repo passes one version tag.
- Pushes even while the GitHub release is a draft (GHCR has no draft state).

## `apko show-config`

Purpose: print the fully-derived config apko will act on (YAML). Read-only.

Usage:

```bash
apko show-config <config.yaml> [flags]
```

Flags: `-k, --keyring-append`, `-r, --repository-append`, `-b, --build-repository-append`,
`--offline`, `--cache-dir`. No `--arch`.

## `apko show-packages`

Purpose: resolve and print the exact packages + versions that would install, without building.
Best preview of what the floating Wolfi base pulls right now (CVE bumps, new packages).

Usage:

```bash
apko show-packages <config.yaml> [flags]
```

Flags that matter here:

- `--arch <strings>`: which arch's resolution to show (accepts `host`).
- `--format <string>`: predefined name (e.g. `name-version`, `name=version`, `packagelock`,
  `packagelock-source`) or a Go template over `.Name`, `.Version`, `.Source` (default
  `{{ .Name }} {{ .Version }}`). `packagelock`/`packagelock-source` emit a YAML-list-ready form.
- `-k, --keyring-append`, `-r, --repository-append`, `-b, --build-repository-append`,
  `--offline`, `--cache-dir`. Append the melange pub key if you need it to resolve `@local`.

## `apko lock`

Exists but intentionally **not used** in this repo (see the SKILL). Writes a `.lock.json` that
pins package versions.

Usage:

```bash
apko lock <config.yaml> [flags]
```

Flags: `--arch`, `--output <string>` (lockfile path), `-k, --keyring-append`,
`-r, --repository-append`, `-b, --build-repository-append`, `--cache-dir`,
`--include-paths`, `--ignore-signatures`. Do not add `apko.lock.json` to this repo.

## `apko version`

Purpose: print the apko version.

Usage:

```bash
apko version [--json]
```

## Other subcommands (not used here)

`build-minirootfs`, `clean`, `dot`, `install-keys`, `login`, `completion`, `help`. `apko login`
can authenticate to a registry, but the repo relies on `docker login` + the Docker keychain.
`apko dot` (dependency digraph) and `apko clean` (cache) are occasional debugging aids.
