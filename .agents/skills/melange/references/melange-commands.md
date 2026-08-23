# Melange Command Map

Curated operator reference for `melange v0.54.0`. Prefer local `melange --help` /
`melange <cmd> --help` output and the official docs (https://github.com/chainguard-dev/melange)
if anything here drifts. Every flag below is copied from the pinned version's `--help`; flags
not used by this repo are marked so.

## Global flags

Apply across all `melange` commands:

- `--log-level string`: log level — `debug`, `info`, `warn`, `error` (default `INFO`).
- `-h`, `--help`: help for the command.

## `melange build`

Purpose: build a package (the signed apk) from a YAML configuration file. This is the repo's
core operation.

Usage:

```bash
melange build [config.yaml] [flags]
```

Flags that matter here:

- `--arch strings`: architectures to build for (e.g. `x86_64,arm64`). The repo passes a single
  Go-style arch per invocation (`amd64` or `arm64`); default is all arches in the config.
- `--runner string`: runner used to execute build steps — `bubblewrap`, `docker`, or `qemu`.
  The repo always passes `docker` (required on macOS; needs Docker running).
- `--signing-key string`: key used to sign the produced apk.
- `--source-dir string`: directory of included sources mounted into the build (the repo passes
  `.` so the module source compiles in).
- `--vars-file string`: file of preloaded build vars; overrides `vars:` in the config. The repo
  injects `version` / `commit` / `date` this way.
- `-k`, `--keyring-append strings`: extra keys to include in the build environment keyring (for
  pulling from signed repositories; the apko keyring handoff is a separate concern).
- `--out-dir string`: directory packages are written to (default `./packages/`). The apk lands
  in `<out-dir>/<wolfi-arch>/`, where the Wolfi arch is `x86_64`/`aarch64`, not `amd64`/`arm64`.
- `-r`, `--repository-append strings`: extra repositories for the build environment.
- `--package-append strings`: extra packages to install into the build environment.
- `--namespace string`: namespace for package URLs in the generated apk SBOM (default `unknown`).
- `--generate-index`: whether to generate `APKINDEX.tar.gz` (default `true`).
- `--cache-dir string`: cached inputs directory (default `./melange-cache/`).
- `--debug`: enable debug logging of build pipelines.
- `--debug-runner`: keep the builder container after success/failure for inspection.
- `-i`, `--interactive`: attach a tty to the builder pod on failure.

Not used by this repo (do not add without reason):

- `--generate-provenance`: emits SLSA provenance as a separate `.attest.tar.gz` next to the apk.
  This repo produces image-level provenance via `attest.yml` instead, so this stays off.
- `--build-date string`: timestamp for files inside the image (reproducibility). Distinct from
  the ldflag `date`, which comes from `--vars-file`. The repo leaves this at default.

Notes:

- Canonical repo invocation: `melange build melange.yaml --arch <arch> --runner docker
  --signing-key <key>.rsa --source-dir . --vars-file <vars>.yaml`.
- The output public key (the `.rsa.pub` matching `--signing-key`) is handed to `apko build` /
  `apko publish` via `--keyring-append` so apko trusts the `@local` apk.

## `melange keygen`

Purpose: generate an RSA keypair for package signing.

Usage:

```bash
melange keygen [key.rsa] [flags]
```

Flags:

- `--key-size int`: size of the RSA key in bits (default `4096`).

Notes:

- Writes `<name>.rsa` (private) and `<name>.rsa.pub` (public). In this repo keys are ephemeral
  and gitignored; CI uses per-arch names (`melange-amd64.rsa`, `melange-arm64.rsa`).

## `melange sign`

Purpose: sign an existing `.apk` on disk in place with the provided key.

Usage:

```bash
melange sign [--signing-key=key.rsa] package.apk
melange sign [--signing-key=key.rsa] *.apk
```

Flags:

- `-k`, `--signing-key string`: signing key (default `local-melange.rsa`).

Notes:

- The repo signs during `build` via `--signing-key`, so this standalone command is rarely
  needed. Use it only to re-sign a prebuilt apk.

## `melange sign-index`

Purpose: sign an APK repository index (`APKINDEX.tar.gz`).

Usage:

```bash
melange sign-index [--signing-key=key.rsa] <APKINDEX.tar.gz>
melange sign-index [--signing-key=key.rsa] <APKINDEX.tar.gz> --force
```

Flags:

- `-f`, `--force`: overwrite the existing index with a newly signed one.
- `--signing-key string`: signing key (default `melange.rsa`).

Note: the default key name here (`melange.rsa`) differs from `sign`'s default
(`local-melange.rsa`). Not part of the repo's build flow.

## `melange package-version`

Purpose: print the target package id for a config, i.e.
`{{ .Package.Name }}-{{ .Package.Version }}-r{{ .Package.Epoch }}`.

Usage:

```bash
melange package-version [config.yaml]
```

Notes:

- Read-only; useful for scripting the expected apk filename. It is sugar over `melange query`.

## `melange bump`

Purpose: update a melange YAML to a new package version.

Notes:

- Do NOT use in this repo. `package.version` carries `# x-release-please-version` and is owned
  by release-please. Flags are not captured here; run `melange bump --help` if ever needed
  outside this repo.

## Other subcommands (present, unused here)

`compile`, `index`, `initramfs`, `license-check`, `lint` (experimental), `query`, `scan`,
`source`, `test`, `update-cache`, `version`, `completion`. None are part of the repo's build
path; consult `melange <cmd> --help` before relying on any of them.
