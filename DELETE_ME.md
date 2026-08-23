# Welcome to the Meigma Go Template

This repository was generated from `template-go`, the standard starter for Meigma Go projects.
It is meant to give new repositories a working baseline on day one: a small Go CLI by default, Moon task orchestration, pinned CI, dependency automation, repository security defaults, and an enabled release pipeline that has already been exercised by the template application.

Delete this file after you finish the first-repository setup checklist below.
It is only here to orient the initial project owner.

## What This Template Provides

- A minimal Go module at `github.com/meigma/template-go`.
- A Cobra/Viper CLI skeleton under `cmd/template-go` and `internal/cli`.
- Moon tasks for `format`, `lint`, `build`, `test`, and `check`.
- `golangci-lint` wired through mise and Moon.
- CI that delegates to `moon ci --summary minimal` with pinned actions, dependency caches, and minimal token permissions.
- A scheduled container vulnerability scan that uploads SARIF results to GitHub code scanning.
- Dependabot coverage for GitHub Actions, Go modules, and the docs uv project.
- MkDocs Material docs scaffolding under `docs/`, with GitHub Pages as the default publishing target.
- Repository settings for signed commits, squash-only merges, immutable releases, private vulnerability reporting, and protected tags.
- Release Please plus a full-SHA-pinned `meigma/release` caller for verified GitHub Release assets and a multi-architecture GHCR image.

## How It Works

Moon is the main entrypoint for local development and CI:

```sh
moon run root:check
```

That aggregate check runs the Go formatter/linter/build/tests plus the docs build.
The GitHub Actions CI workflow runs the same path through:

```sh
moon ci --summary minimal
```

The workflow caches Go modules, Go build artifacts, golangci-lint state, and uv's download cache through GitHub Actions. If that is not enough for a larger generated repository, add Moon remote caching later with Depot or another Bazel Remote Execution-compatible backend and repository credentials.

The `GitHub Pages` workflow builds the MkDocs site on pull requests and deploys the default-branch `docs/build` output to Pages. The repository settings manifest defaults Pages to workflow-based publishing with HTTPS enforcement.

The release machinery is enabled so the template proves the supported
`meigma/release` contract before generated projects inherit it: one static Go
application, downloadable release assets, and a Linux `amd64`/`arm64` OCI image.

## First Setup Checklist

1. Rename the Go module:

   ```sh
   go mod edit -module github.com/meigma/YOUR_REPO
   ```

2. Choose the project shape.

   The reusable release unit supports one Go application with downloadable
   binaries and an OCI image. For other shapes:

   - Binary plus OCI image: keep the default release caller.
   - Binary only: remove the `oci-image` and `oci-publish` jobs, set
     `require-oci-image: false`, and remove `melange.yaml`, `apko.yaml`, the
     `image-local` task, and the container security scan.
   - Container only: the maintained release unit is not a container-only
     contract. Keep its canonical binary path or replace the release workflow
     with a container-specific design.
   - Library only: remove the CLI and artifact workflow. Keep Release Please
     only if the library needs changelogs, tags, and draft GitHub Releases.

3. For a binary-producing project, rename the binary directory:

   ```sh
   mv cmd/template-go cmd/YOUR_BINARY
   ```

   For a library-only project, delete `cmd/template-go`, remove or rewrite `internal/cli`, and remove Cobra/Viper dependencies that are no longer used.

4. Replace template placeholders:

   ```sh
   rg "template-go|TEMPLATE_GO|github.com/meigma/template-go"
   ```

   Update Go imports, Moon metadata, README and docs text, CLI environment
   prefixes, `.goreleaser.yaml`, `melange.yaml`, `apko.yaml`, and the release
   workflow metadata. Update `docs/mkdocs.yml` with the generated repository's
   GitHub Pages URL, usually `https://OWNER.github.io/REPO/`.

5. Refresh module metadata:

   ```sh
   go mod tidy
   ```

6. Configure and rehearse releases.

   - Review a published [`meigma/release`](https://github.com/meigma/release)
     version and resolve it to a full commit SHA.
   - Update every `uses: meigma/release/...` reference and
     `checksum-signing-workflow-ref` in `.github/workflows/release.yml` to that
     same SHA. Mixed revisions are unsupported.
   - Update `.goreleaser.yaml`: project name, build ID, command path, binary
     name, package metadata, archive names, and generated Homebrew/Scoop
     metadata.
   - Update `melange.yaml`: package metadata and the destination binary path.
     Melange must install the staged `application` file rather than compile a
     second binary.
   - Update `apko.yaml`: package name, entrypoint, runtime packages, and OCI
     annotations.
   - Update `.github/workflows/security-scan.yml`: local image name and SARIF
     category.
   - Configure `MEIGMA_RELEASE_APP_CLIENT_ID` and
     `MEIGMA_RELEASE_APP_PRIVATE_KEY`, install the App on the repository, and
     grant it protected-tag bypass if `v*` tags are protected.
   - Leave `publish-image` and `publish-release` set to `false` for the first
     stable candidate. Merge the Release Please PR, inspect the populated draft
     and workflow artifacts, and verify their checksums, attestations, and OCI
     digest.
   - After the rehearsal succeeds, enable both publication inputs in a reviewed
     change and publish the next stable release.

   The template does not call the optional Homebrew, Scoop, or central native
   package-repository publishers. To add them:

   - Follow
     [Add Homebrew and Scoop](https://github.com/meigma/release/blob/v0.1.17/docs/how-to/add-homebrew-and-scoop.md)
     before adding the `publish-homebrew.yml` or `publish-scoop.yml` caller jobs.
   - Follow
     [Operate a native package repository](https://github.com/meigma/release/blob/v0.1.17/docs/how-to/operate-a-native-package-repository.md)
     before enabling native package signing or adding
     `request-package-repository.yml`.

   Do not enable an optional publisher until its destination repository, App
   installation, credentials, and required checks exist.

7. Run the full local check:

   ```sh
   moon run root:check
   ```

8. Update project-facing docs:

   - Rewrite `README.md` for the actual project.
   - Review `CONTRIBUTING.md` and `SECURITY.md`.
   - Add a real license before publishing the repository.

9. Delete this file:

   ```sh
   rm DELETE_ME.md
   ```
