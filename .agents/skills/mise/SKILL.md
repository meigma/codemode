---
name: mise
description: >
  Operate mise as the single source of truth for tool versions and integrity in
  CodeMode. Use when touching mise.toml or mise.lock, bumping or adding a pinned
  tool (go, python, golangci-lint, mockery, uv, moon, gh), resolving "command not
  found"/PATH problems, fixing locked/trust failures, or wiring mise into moon and
  the CI workflow.
---

# mise

mise owns the lifecycle of every pinned tool and the project's tool-related env in
this repo. Treat `mise.toml` + `mise.lock` as the only place a toolchain version is
declared; everything else (moon, CI) consumes what mise puts on PATH.

## Verified against

- `mise 2026.7.17` (`macos-arm64`) for the lock/trust workflow below, and this
  repo's `mise.toml`, `mise.lock`, `moon.yml`, `.moon/toolchains.yml`, and
  `.github/workflows/ci.yml`.
- The command map in `references/mise-commands.md` is a `--help` capture from
  `mise 2026.6.14`, the version CI installs. Re-verify on a mise minor/major bump.

## Use this skill when

- Bumping or adding a tool, or reviewing a diff that touches `mise.toml`/`mise.lock`.
- A tool is missing from PATH, or `mise install` fails closed under `locked`.
- mise prompts for trust (commonly inside a `.wt/` worktree that nests under the repo).
- Explaining how moon or `ci.yml` get their binaries.

## mise's lane (non-negotiables)

mise manages **tool + env lifecycle only**. State these as rules:

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
5. This repo defines **no mise tasks**. Do not add them; tasks belong in `moon.yml`.

## How mise is wired here

`mise.toml`:

- `[tools]`: `go = "1.26.6"`, `python = "3.14.3"` (core backends), and five CLIs
  pinned via explicit `aqua:` refs — `golangci/golangci-lint` (format + lint),
  `vektra/mockery` (generated mocks, per AGENTS.md T2/T3), `astral-sh/uv` (the
  MkDocs toolchain), `moonrepo/moon` (task runner), and `cli/cli` (`gh`).
- `[env] GOTOOLCHAIN = "local"`: never auto-download a Go toolchain other than the
  pinned one; matches `go.mod`'s `go 1.26.6`. mise `[env]` is **not** carried by the
  CI action's shims, so `ci.yml` also sets `GOTOOLCHAIN: local` at job level — keep
  both in sync.
- `[settings] lockfile = true` (read/write `mise.lock`) and `locked = true` (the
  integrity gate; equivalent to the `--locked` flag / `MISE_LOCKED=1`).

moon consumes mise, it does not duplicate it: `.moon/toolchains.yml` declares no
language toolchain and `moon.yml` sets `toolchains.default: system`, so every moon
task command is a bare binary (`go`, `golangci-lint`) resolved from PATH. `moon.yml`
also lists `mise.toml` + `mise.lock` as inputs (via the `goSources`/`lintConfig`
groups), so a tool bump re-triggers those tasks and invalidates the result cache of
the cacheable one (`build`; `format`, `lint`, `test`, `mcp-smoke`, `race`, and
`check` all run with `cache: false`). See the `worktrunk` skill for worktree
mechanics.

CI (`.github/workflows/ci.yml`) installs via
`jdx/mise-action@… with: version: 2026.6.14, cache: true`. The action installs
every tool from `mise.toml` honoring `mise.lock` (locked → fail closed), including
`moon`, and prepends the shim dir to PATH so moon's `system` tasks find the
binaries. CI uses mise-action, **not** `moonrepo/setup-toolchain`.

## The lockfile, precisely

`mise.lock` is `# @generated`. Per tool it records a `[[tools."<ref>"]]` block
(`version`, `backend`) and one `[tools."<ref>"."platforms.<plat>"]` table for each
of the four platforms: `linux-x64`, `linux-arm64`, `macos-x64`, `macos-arm64`.

- Every platform entry carries a `url`. **`locked = true` requires a pre-resolved
  `url` per platform** and fails closed otherwise (per `mise install --help`: it
  prevents API calls to GitHub/aqua at install time).
- Every entry in this repo also carries `checksum = "sha256:…"`, which is enforced.
  (Tools that publish no upstream checksum would carry a `url` but no `checksum`;
  none of this repo's pinned tools are in that situation, so a missing checksum here
  would be unexpected, not normal.)
- A subset additionally records `provenance = "github-attestations"`, reflecting the
  verification the aqua registry applies for that tool: `uv`, `gh`, `golangci-lint`,
  and `python`. The remaining tools (`go`, `moon`, `mockery`) carry no `provenance`
  field. Do **not** claim every tool is attestation-verified; the always-on
  guarantees are the pinned `url` and the `checksum`.

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

## Removing a tool

1. Delete its `[tools]` entry from `mise.toml`.
2. Re-run `mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64`; the
   regenerated lockfile drops the removed tool's blocks. Never delete them by hand.

## Worktree trust gotcha

`.wt/` worktrees nest **under** the repo, so mise's upward config search loads both
the worktree's config and the parent repo's `mise.toml`. When mise prompts, trust
both:

```bash
mise trust --all      # trust this dir and its parents
mise trust --show     # inspect trust status without changing it
```

The main checkout `/Users/josh/code/meigma/codemode` is already trusted.

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
  `mise activate` runs in the shell, or you go through `mise exec` / shims. CI relies
  on mise-action prepending the shim dir; locally use `eval "$(mise activate zsh)"`
  once, or prefix one-off commands with `mise exec --`.
- `mise.local.toml` / `.mise.local.toml` are gitignored per-developer overrides.
  Never commit them and never put project pins there — project pins belong in the
  committed `mise.toml`.
- `mockery` is pinned here but driven by `.mockery.yml`; regenerate mocks with
  `mise exec -- mockery` rather than a locally installed copy.

## Command reference

See [references/mise-commands.md](references/mise-commands.md) for the version-stamped
command and flag map.
