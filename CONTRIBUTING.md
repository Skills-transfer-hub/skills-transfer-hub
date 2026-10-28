# Contributing to STH

Welcome — this document covers the conventions you need to ship a change that
flows cleanly through the release pipeline.

## Getting started

Use Git and Go 1.25.0 or later, as declared in `go.mod`. Start from `develop`,
create a branch in your fork, and open your pull request against `develop`.
Changes are contributed under the repository's [MIT license](LICENSE).

```sh
git clone https://github.com/Skills-transfer-hub/skills-transfer-hub.git
cd skills-transfer-hub
git switch develop
go build ./...
go test ./...
go vet ./...
```

The first Go module download needs network access. No STH account, publishing
secret, or access to STH Code is required. The PR CI runs local bridge tests;
maintainer pushes and releases additionally run a pinned private STH Code
consumer contract using Node.js and a read-only token. Forks do not run that
private check or publish releases.

Follow [AGENTS.md](AGENTS.md) for Go conventions, including descriptive names,
no code comments, and no `t.Parallel()` in `cmd` tests using `t.Setenv`.
Keep your change focused and describe how you verified it. Report security
issues through the private channel in [SECURITY.md](SECURITY.md).

## Pull request titles

PR titles become the default squash commit subject. Write them for someone
who has never read your PR description.

### Required format

```
<type>(<scope>): <user-facing summary>
```

- **type** — one of: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`,
  `chore`, `ci`.
- **scope** — the area of the CLI touched. Current scopes:
  `scanner`, `writer`, `pack`, `telemetry`, `init`, `install`, `update`,
  `status`, `remove`, `buddy`, `auth`, `cloud`, `schema`, `licensing`,
  `release`, `menu`, `http`, `fs`, `cmd`.
- **summary** — what changed from a user's perspective, not from the PR's
  internal perspective. Imperative mood, no trailing period.

### Breaking changes

Suffix the type (or scope) with `!`:

```
feat(writer)!: rename target detection field from kind to target
```

Describe the migration in the PR's `Release notes` section; it is not
generated automatically from the title.

### Banned markers in titles

Do **not** put internal tracking artefacts in PR titles that will be merged to
`main`:

- ❌ `feat(pack): scaffolder behind a Pro gate (PR6/6)` — `(PR6/6)` leaks
  internal planning into the public changelog.
- ❌ `fix(menu): part 2 — actually wire the highlight` — split markers belong
  in the description, not the headline.
- ❌ `wip: rewrite scanner` — `wip` PRs should be draft, not merged.

If you need to track a multi-PR series, use the PR description, milestones, or
labels — none of which surface in release notes.

### Good vs bad examples

```
✅ feat(writer): Copilot, Codex and Cursor target auto-detection
✅ fix(install): preserve CRLF line endings when writing managed files
✅ feat(telemetry)!: replace alpha gate with 3-tier user consent
✅ perf(scanner): cache tree walks across catalog reads

❌ feat: stuff (PR6/6)
❌ Fixed bug — does not start with a type, missing scope
❌ feat(pack): add command. Closes #142. — trailing period, refs in title
```

## PR description and release notes

After a tested merge to `main`, `auto-release.yml` extracts only the
`## Release notes` section of the PR body. Those notes are reused on
`sth-releases` and in CLI update highlights. The release title is
`Alpha <tag>`; commit messages are not appended to the published notes.

For a release-worthy PR (most user-visible changes), the description should
contain a short hand-written summary of what the user is getting and why. A
bullet list of 1–3 items is ideal — that's what the STH CLI will pull in as
"highlights" when it tells users an update is available:

```markdown
## Release notes

- New `sth telemetry` command and 3-tier consent picker in `sth init`.
- Existing users with `analytics_consent=true` migrate automatically to
  `connected`.
- See the [telemetry policy](https://sth-dashboard.vercel.app/docs/telemetry)
  for the full breakdown.
```

For changes without user-facing notes, leave that section empty; the release
workflow uses a generic maintenance message. Put implementation details and
validation in `Why`, `What`, and `Tests`. These sections are visible to anyone
who can read the repository, but are not included in release notes. Never put
secrets or confidential information in PRs or comments.

## Commit messages

The same rules as PR titles apply: `<type>(<scope>): <user-facing summary>`. When a PR
is squash-merged, the squash commit subject defaults to the PR title — so
getting the PR title right covers the common case.

## What ships where

### Release approval and test releases

Pull requests targeting `main` require a passing CI gate and an approval from
`@Skills-transfer-hub/sth-release-approvers`. That team contains only
`@Grandpied33` and `@jeremy-guilpin`; adding a repository collaborator does not
add them to this approval team. Authors cannot approve their own pull requests.

For a release validation run, add the `prerelease` label before merging the
approved pull request into `main`. The release workflow generates an `-rc.N`
version and skips stable package-manager publication and dashboard dispatch.
Verify the signed assets before promoting any change to a stable release.

- **CLI code & history**, under MIT, live on `Skills-transfer-hub/skills-transfer-hub`.
- **Binaries, archives, SHA256SUMS, release notes** live on
  `Skills-transfer-hub/sth-releases` (public).
- The CLI's `sth update self` reads release metadata (incl. body highlights)
  from the public releases feed.

If you touch the release pipeline, run `goreleaser check` and the
[local snapshot smoke test](README.md#smoke-test-local-avant-tag), which uses
an ephemeral signing key and does not publish. It additionally needs
GoReleaser v2, `jq`, `zip`, and a POSIX shell. Never use a production signing
key for a local test.
