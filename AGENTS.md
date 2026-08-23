<!-- BEGIN ai-protocol -->
# Agent Instructions

This repository's operating protocol lives in `.session.md`.

Before doing substantive work, read `.session.md` in full and follow it. It
covers startup context loading, session setup, session lifecycle, skill loading,
Worktrunk branching, session journaling, file schemas, architecture, and process
expectations.

If `.session.md` is missing, stop and tell the user the session protocol is not
installed correctly.
<!-- END ai-protocol -->

# Go Best Practices

Rules are grouped by category and numbered for reference. Cite them by ID in
reviews and feedback: "this violates A1", "apply D4 here".

## A — Architecture

- **A1**: Follow hexagonal architecture strictly. Business logic must run and
  test without side effects. Put all I/O behind ports; adapters implement them.
- **A2**: Build each adapter for one purpose. Do not write broad adapters that
  wrap an entire service surface.
- **A3**: A package does one obvious thing and keeps a strong contract. Export
  a thin set of types and functions; keep the core logic unexported.
- **A4**: Package names are short, pronounceable, and obvious. Do not join
  several words into one long name. Duplicate names across the tree are fine
  (two packages can each have a `mocks/` subpackage); disambiguate with import
  aliases at the call site.

## T — Testing

- **T1**: Test at three layers: unit tests for functions, integration tests
  with mock adapters, and end-to-end tests against live services (for example,
  testcontainers).
- **T2**: Never write a mock by hand. Generate all mocks with `mockery`.
- **T3**: Generate mocks for every adapter by default. Mocks live in a `mocks/`
  subpackage under the adapter's primary package.

## R — Readability

- **R1**: Code must be easy to follow. This cuts both ways: no compressed
  one-liners that need decoding, and no layered abstractions that hide simple
  logic.
- **R2**: Hard cap of 1,000 lines per Go source file. Before reaching it, split
  into two well-named files or extract a new package.
- **R3**: Never reimplement a standard library function. Check what the
  project's Go version ships before assuming a function does not exist.

## P — Performance

- **P1**: When touching or creating code, decide whether it sits on a critical
  path. If it does, make extra passes until it meets a high bar for efficiency.
- **P2**: Do not abuse the heap. Watch what gets copied. Stream large data
  (`io.Reader`/`io.Writer` piping) instead of buffering it into memory.

## D — Documentation

- **D1**: Every function, type, and struct field gets a Godoc comment, exported
  or not. This feeds IDE intellisense and helps developers move through the
  code.
- **D2**: Functions that form a public API boundary get extra effort: thorough
  Godoc plus examples where they help.
- **D3**: Use inline comments sparingly. Reserve them for genuinely complex
  code or code that would otherwise mislead, such as a non-obvious workaround.
- **D4**: Every package has a `doc.go` with a package-level Godoc comment. No
  exceptions.
- **D5**: All user-facing documentation lives in `docs/`, follows the
  [Diátaxis](https://diataxis.fr/) conventions (tutorials, how-to guides,
  reference, explanation), and is written in the Plain Language style.
- **D6**: When a change affects behavior a user can see and the repository has
  user documentation, update the docs in the same PR as the change.

## I — Types and Interfaces

- **I1**: Create types for domain terms instead of passing plain types around.
  Prefer `type ProjectName string` over accepting `projectName string` in every
  function.
- **I2**: Accept interfaces, return concrete types. Deviate only when following
  the idiom makes the API clearly worse to use.

## E — Errors and Reliability

- **E1**: Define sentinel errors when callers need to inspect why something
  failed. Keep them high-level: a sentinel wraps the plain errors bubbling up
  from below. Do not scatter sentinels everywhere.
- **E2**: Check pointers and interfaces for nil before use. If a value was
  already checked upstream, do not check it again.
- **E3**: Do not give up and return an error when a retry is the acceptable
  behavior. Failing on the first transient error is the easy way out.

## L — Dependencies

- **L1**: Prefer mature, well-tested libraries. Never import an obviously
  abandoned one. Weigh size against need: writing a small piece yourself beats
  importing 5% of a very large library.
