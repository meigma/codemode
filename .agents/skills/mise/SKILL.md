---
name: mise
description: >
  Operate mise as the single source of truth for tool versions and integrity in
  template-go. Use when touching mise.toml or mise.lock, bumping or adding a pinned
  tool (go, python, uv, golangci-lint, moon, melange, apko, cosign), resolving
  "command not found"/PATH problems, fixing locked/trust failures, or wiring mise
  into moon, the CI workflow, or the local container task.
---

# mise

mise owns the lifecycle of every pinned tool and the project's tool-related env in
this repo. It replaced Proto (`.prototools`, `.moon/proto/*`). Treat `mise.toml` +
`mise.lock` as the only place a toolchain version is declared; everything else
(moon, CI, the container build) consumes what mise puts on PATH.

## Verified against

- `mise 2026.6.14` (`macos-arm64`), grounded in the captured `--help` for
  `install/use/ls/lock/exec/run/trust/outdated/upgrade/settings/current/activate/env/which`
  and `mise doctor --help`, plus this repo's `mise.toml`, `mise.lock`, `moon.yml`,
  `.moon/toolchains.yml`, `.github/workflows/ci.yml`, and `README.md`.
- Advice is grounded in the local CLI and these files, not memory. Re-verify on a
  mise minor/major bump.

## Use this skill when

- Bumping or adding a tool, or reviewing a diff that touches `mise.toml`/`mise.lock`.
- A tool is missing from PATH, or `mise install` fails closed under `locked`.
- mise prompts for trust (commonly inside a `.wt/` worktree that nests under the repo).
- Explaining how moon, `ci.yml`, or `mise run image-local` get their binaries.

## mise's lane (non-negotiables)

mise manages **tool + env lifecycle only**, plus the one local container task below.
State these as rules:

1. mise is **not the task runner and not the CI gate** — that is moon. Do not move
   build/lint/test into mise tasks.
2. **Every tool an engineer needs goes through mise.** Never `go install`,
   `go tool`, `brew install`, `apt`, `npm -g`, `pipx`, `cargo install`, or a manual
   download for project tooling. Add it to `[tools]` and `mise lock` instead.
3. **Force the verifying backend.** Pin CLIs with an explicit `aqua:` (or `github:`)
   ref, e.g. `"aqua:golangci/golangci-lint" = "2.12.2"`. A bare/asdf/npm/cargo/pipx
   backend resolves without a recorded checksum — never let a tool land that way.
4. **Bump = edit `mise.toml`, then `mise lock`, then commit both together.** Never
   hand-edit `mise.lock` (`# @generated`) and never commit one without the other.
5. The only mise *task* in this repo is `image-local` (a local container convenience).
   Do not add general-purpose tasks here.

## How mise is wired here

`mise.toml`:

- `[tools]`: `go = "1.26.4"`, `python = "3.14.3"` (core backends), and six CLIs
  pinned via explicit `aqua:` refs (`golangci/golangci-lint`, `astral-sh/uv`,
  `moonrepo/moon`, `chainguard-dev/melange`, `chainguard-dev/apko`, `sigstore/cosign`).
- `[env] GOTOOLCHAIN = "local"`: never auto-download a Go toolchain other than the
  pinned one; matches `go.mod`'s `go 1.26.4`. mise `[env]` is **not** carried by the
  CI action's shims, so `ci.yml` also sets `GOTOOLCHAIN: local` at job level — keep
  both in sync.
- `[settings] lockfile = true` (read/write `mise.lock`) and `locked = true` (the
  integrity gate; equivalent to the `--locked` flag / `MISE_LOCKED=1`).

moon consumes mise, it does not duplicate it: `.moon/toolchains.yml` declares no
language toolchain and `moon.yml` sets `toolchains.default: system`, so every moon
task command is a bare binary (`go`, `golangci-lint`) resolved from PATH. `moon.yml`
also lists `mise.toml` + `mise.lock` as inputs (via the `goSources`/`lintConfig`
groups) of build/format/lint/test, so a tool bump re-triggers those tasks and
invalidates the result cache of the cacheable one (build; format/lint/test already
run with `cache: false`). See the `worktrunk` skill for worktree mechanics and the
`melange`/`apko` skills for the container build those pinned tools feed.

CI (`.github/workflows/ci.yml`) installs via
`jdx/mise-action@… with: version: 2026.6.14, cache: true`. The action installs
every tool from `mise.toml` honoring `mise.lock` (locked → fail closed), including
`moon`, and prepends the shim dir to PATH so moon's `system` tasks find the
binaries. CI uses mise-action, **not** `moonrepo/setup-toolchain`. (The
publishing pipeline — `release.yml`'s mise-action steps — sets `cache: false` so a
poisoned Actions cache can't influence the toolchain that builds/signs artifacts.)

## The lockfile, precisely

`mise.lock` is `# @generated`. Per tool it records a `[[tools."<ref>"]]` block
(`version`, `backend`) and one `[tools."<ref>"."platforms.<plat>"]` table for each
of the four platforms: `linux-x64`, `linux-arm64`, `macos-x64`, `macos-arm64`.

- Every platform entry carries a `url`. **`locked = true` requires a pre-resolved
  `url` per platform** and fails closed otherwise (per `mise install --help`: it
  prevents API calls to GitHub/aqua at install time).
- Every entry in this repo also carries `checksum = "sha256:…"`, which is enforced.
  (Tools that publish no upstream checksum — e.g. sqlc — would carry a `url` but no
  `checksum`; none of this repo's pinned tools are in that situation, so a missing
  checksum here would be unexpected, not normal.)
- A subset additionally records a `provenance` field, reflecting the verification
  the aqua registry applies for that tool: `provenance = "github-attestations"` on
  `uv`, `golangci-lint`, and `python`; `provenance = "cosign"` on `cosign`. The
  remaining tools (`go`, `melange`, `apko`, `moon`) carry no `provenance` field. Do
  **not** claim every tool is attestation-verified; the always-on guarantees are the
  pinned `url` and the `checksum`.

## Bumping a tool (the canonical operation)

```bash
# 1. edit the version in mise.toml (keep the aqua: ref)
# 2. re-resolve url/checksum for all four platforms
mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64
# 3. commit mise.toml + mise.lock together
```

- `mise outdated` (add `--bump` to see latest across major lines, `-J` for JSON)
  shows what could move before you decide.
- `mise upgrade <tool> --bump` is the one-shot equivalent (edits `mise.toml` and
  re-locks), but the repo's committed convention is the explicit edit + `mise lock`
  so the version change is a reviewable diff.
- After locking, confirm all four platform tables are present for each changed tool
  before committing; do not ship a partial lock entry.

## Adding a tool

1. Add `"aqua:<owner>/<repo>" = "<version>"` (or another verifying backend) to
   `[tools]` in `mise.toml`.
2. `mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64` to populate
   url/checksum for all platforms.
3. If a moon task uses it, add it to that task's input fileGroup as appropriate;
   `mise.toml`/`mise.lock` are already inputs of the main task groups.
4. `mise install` locally to materialize it, then commit both files.

## Worktree trust gotcha

`.wt/` worktrees nest **under** the repo, so mise's upward config search loads both
the worktree's config and the parent repo's `mise.toml`. When mise prompts, trust
both:

```bash
mise trust --all      # trust this dir and its parents
mise trust --show     # inspect trust status without changing it
```

The main checkout `/Users/josh/code/meigma/template-go` is already trusted.

## Inspection / read-only ops

```bash
mise ls                 # installed + active tool versions (-J for JSON)
mise current            # active versions only, script-friendly
mise which golangci-lint   # resolved bin path; --version for just the version
mise outdated           # what could bump
mise doctor             # diagnose install/PATH problems (doctor path prints PATH)
mise exec -- golangci-lint version   # run a pinned tool ad hoc, no shell activation
```

## Gotchas

- `mise install` installs but does **not** activate — tools are not on PATH until
  `mise activate` runs in the shell, or you go through `mise exec` / `mise run` /
  shims. CI relies on mise-action prepending the shim dir; locally use
  `eval "$(mise activate zsh)"` once, or prefix one-off commands with `mise exec --`.
- `mise.local.toml` / `.mise.local.toml` are gitignored per-developer overrides.
  Never commit them and never put project pins there — project pins belong in the
  committed `mise.toml`.
- `mise.toml` and `mise.lock` are committed and authoritative; the gitignored
  `melange*.rsa*`, `melange-vars.yaml`, `.melange-vars.local.yaml`, and the
  `packages/`/`image.tar` artifacts come from `mise run image-local` and must stay
  uncommitted (see the `melange`/`apko` skills).
- `[tasks.image-local]` passes `--runner docker` to melange (melange needs a Linux
  build sandbox) and loads/retags the host-arch image into Docker as
  `template-go:dev`. Docker must be running on macOS.

## Command reference

See [references/mise-commands.md](references/mise-commands.md) for the version-stamped
command and flag map.
