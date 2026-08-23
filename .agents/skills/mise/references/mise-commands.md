# mise Command Map

Curated operator reference for `mise 2026.6.14`. Prefer local `--help` output and
the official docs (https://mise.en.dev) if anything here drifts. Every flag below is
taken from this version's `--help`; do not invent flags.

## Global flags

These appear on `mise` itself and on most subcommands:

- `-C, --cd <DIR>`: change directory before running.
- `-E, --env <ENV>`: load `mise.<ENV>.toml` for this invocation.
- `-j, --jobs <JOBS>`: parallelism (`MISE_JOBS`). Root default 8; install/use/exec/run default 4.
- `-q, --quiet`: suppress non-error messages.
- `-v, --verbose...`: extra output (`-vv` for more).
- `-y, --yes`: answer yes to all prompts.
- `--locked`: require pre-resolved lockfile URLs for the current platform; fail
  otherwise. Also via `MISE_LOCKED=1` or `settings.locked = true` (set in this repo).
- `--raw`: read/write directly to stdio (implies `--jobs=1` for backends).
- `--silent`: suppress all task output and mise non-error messages.

Root-only options worth knowing: `--no-config` (`MISE_NO_CONFIG=1`), `--no-env`
(`MISE_NO_ENV=1`), `--no-hooks` (`MISE_NO_HOOKS=1`).

Tool refs are `TOOL@VERSION`; backend-qualified tools use a prefix, e.g.
`aqua:golangci/golangci-lint`, `cargo:ripgrep`, `npm:prettier`.

## Tool lifecycle

### `mise install` (alias `i`)

Purpose: install tool versions to `~/.local/share/mise/installs/...`. Does **not**
activate — installed tools are not on PATH until `mise activate`/`exec`/`run`/shims.

Usage:

```bash
mise install                 # install everything in mise.toml (honors mise.lock)
mise install go@1.26.4        # install a specific version
```

Flags that matter here:

- `-f, --force`: reinstall even if present.
- `-n, --dry-run`: show what would install. `--dry-run-code`: same but exit 1 if work remains.
- `--locked`: require lockfile URLs (already on via `settings.locked`).
- `-v, --verbose`: show backend download/build output.

Notes: with `locked = true`, install fails closed if any tool lacks a pre-resolved
URL for the current platform (prevents GitHub/aqua API calls).

### `mise lock`

Purpose: update lockfile checksums and URLs for specified platforms. Operates on the
lockfile in the current config root. If no lockfile exists, prints what would be created.

Usage:

```bash
mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64   # repo canonical
mise lock golangci-lint       # only one tool
mise lock --dry-run
```

Flags that matter here:

- `-p, --platform <LIST>`: comma-separated platforms (`linux-x64,linux-arm64,macos-x64,macos-arm64`).
  If omitted, only platforms already present in the lockfile are refreshed.
- `[TOOL]...`: limit to named tools; default is all tools in the lockfile.
- `-n, --dry-run`: preview without writing.
- `-g, --global`: target global config lockfiles instead of the project root.
- `--local`: update `mise.local.lock` (for `.local.toml` configs) — not used here.

Notes: this is the second half of every tool bump. Always pass all four platforms so
CI runners and both local archs stay covered, then confirm all four tables landed
for changed tools before committing.

### `mise use` (alias `u`)

Purpose: install a tool and write its version into a config file. Writes `mise.toml`
by default (lowest-precedence file).

Usage:

```bash
mise use --pin "aqua:owner/repo@1.2.3"
```

Flags that matter here:

- `--pin`: write the exact version (vs `--fuzzy`, the default).
- `-g, --global`: write to `~/.config/mise/config.toml` instead of the project.
- `--remove <TOOL>`: drop a tool from config.
- `-p, --path <PATH>`: target a specific config file/dir.
- `-n, --dry-run` / `--dry-run-code`.

Notes: the repo convention is to hand-edit `mise.toml` (preserving the `aqua:` ref)
and run `mise lock`, so the version change is a clean reviewable diff. If you do use
`mise use`, keep the explicit verifying backend and follow with `mise lock`.

### `mise upgrade` (alias `up`)

Purpose: upgrade installed tools. By default stays within the `mise.toml` range and
updates `mise.lock`.

Usage:

```bash
mise upgrade node
mise upgrade <tool> --bump      # also rewrite mise.toml to the latest
```

Flags that matter here:

- `-l, --bump`: bump the version in `mise.toml` to latest (keeps precision) and re-lock.
- `-n, --dry-run` / `--dry-run-code`.
- `-x, --exclude <TOOL>`: skip a tool.
- `-i, --interactive`: multiselect menu.

Notes: convenient, but the committed bump workflow is explicit edit + `mise lock`.

## Running tools

### `mise exec` (alias `x`)

Purpose: run a command with mise tools on PATH without modifying the shell.

Usage:

```bash
mise exec -- golangci-lint version
mise exec go@1.26.4 -- go version    # override one tool ad hoc
```

Flags that matter here:

- `--` separates tool args from the command to run.
- `-c, --command <C>`: command as a string.
- `--no-deps`: skip automatic dependency preparation.
- Sandbox flags exist (`--deny-all`, `--deny-net`, `--allow-read <PATH>`, etc.) but
  are not part of this repo's flow.

### `mise run` (alias `r`)

Purpose: run mise tasks. In this repo only `image-local` exists (a local container
convenience); day-to-day build/test/lint go through moon, not mise.

Usage:

```bash
mise run image-local
```

Flags that matter here:

- `-f, --force`: run even if task outputs are up to date.
- `-n, --dry-run`: print execution order without running.
- `--skip-deps`: run only the named task, skipping dependencies.
- `--skip-tools`: do not auto-install tools first.
- `-t, --tool <TOOL@VERSION>`: add a tool for this run.
- `-o, --output <MODE>`: `prefix|interleave|replacing|timed|keep-order|quiet|silent`.

## Inspection

### `mise ls` (alias `list`)

Purpose: list tools mise knows about (installed and/or config-declared).

Flags: `-c, --current` (only config-specified), `-i, --installed`, `-l, --local`,
`-g, --global`, `-m, --missing`, `--outdated`, `--prunable`, `-J, --json`,
`--no-header`.

### `mise current`

Purpose: print active versions only, script-friendly (`.tool-versions` style).
Optional `[PLUGIN]` argument narrows to one tool.

### `mise outdated`

Purpose: show tools with newer versions available.

Flags: `-l, --bump` (compare against latest across major lines, not just the
configured range), `--local`, `--inactive`, `-J, --json`, `--no-header`.

### `mise which`

Purpose: show the resolved path for a tool's binary.

Usage:

```bash
mise which golangci-lint
mise which go --version
```

Flags: `-t, --tool <TOOL@VERSION>`, `--plugin` (print backend/plugin name),
`--version` (print version instead of path).

### `mise doctor` (alias `dr`)

Purpose: diagnose installation/PATH problems. Subcommand `mise doctor path` prints
the PATH entries mise provides. Flag: `-J, --json`.

## Trust and config

### `mise trust`

Purpose: mark config files as trusted so mise will parse them. Needed when a config
uses templates/tool options or sits in a discovery path that requires approval — in
this repo, commonly the parent `mise.toml` seen from a nested `.wt/` worktree.

Usage:

```bash
mise trust --all     # trust current dir and all parents
mise trust --show    # show trust status without changing it
mise trust --untrust # revoke; --ignore to skip a config in future
```

Notes: configs that contain only `min_version`, plain `[tools]` strings, and plain
`[tasks]` load without a trust prompt; templates or tool options require trust.

### `mise settings`

Purpose: view/manage settings (this repo sets `lockfile = true`, `locked = true`).

Usage:

```bash
mise settings           # list active settings
mise settings get lockfile
```

Subcommands: `add`, `get`, `ls`, `set`, `unset`. Flags: `-a, --all`, `-J, --json`,
`-T, --toml`, `-l, --local`.

## Shell activation

### `mise activate`

Purpose: initialize mise in the current shell (PATH or shims). For rc files.

Usage:

```bash
eval "$(mise activate zsh)"
```

Flags: `[SHELL_TYPE]` one of `bash|zsh|fish|nu|xonsh|elvish|pwsh`; `--shims` (use
shims instead of mutating PATH); `--no-hook-env` (debugging).

### `mise env` (alias `e`)

Purpose: export env vars to activate mise once, without a persistent `activate`.

Usage:

```bash
eval "$(mise env -s zsh)"
```

Flags: `-s, --shell <SHELL>`, `-D, --dotenv`, `-J, --json`, `--json-extended`,
`--values`.
