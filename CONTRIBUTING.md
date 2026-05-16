# Contributing to STH

Welcome — this document covers the conventions you need to ship a change that
flows cleanly through the release pipeline.

## Pull request titles

PR titles are user-facing. They are reused verbatim:

- as the headline of the GitHub release on `sth-releases` (via the auto-release
  workflow);
- as the bullet under the matching section (Added / Fixed / Breaking…) of the
  categorised changelog GoReleaser appends to the release body;
- as the message in `git log` for the squashed merge commit on `main`.

Write them for someone who has never read your PR description.

### Required format

```
<type>(<scope>): <user-facing summary>
```

- **type** — one of: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`,
  `chore`, `ci`. Only `feat`, `fix`, `perf`, `refactor` and breaking changes
  appear in the public changelog; the others are filtered out.
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

Commits matching `^[a-z]+(\([^)]+\))?!:` land in the **Breaking changes**
section of the release notes, so users see the migration cost first.

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

## PR description (becomes the release headline)

When a PR is merged, `auto-release.yml` reads the PR **title** and **body** and
writes them as the top of the GitHub release. Below that, GoReleaser appends
the categorised changelog computed from commit messages.

For a release-worthy PR (most user-visible changes), the description should
contain a short hand-written summary of what the user is getting and why. A
bullet list of 1–3 items is ideal — that's what the STH CLI will pull in as
"highlights" when it tells users an update is available:

```markdown
## What changed

- New `sth telemetry` command and 3-tier consent picker in `sth init`.
- Existing users with `analytics_consent=true` migrate automatically to
  `connected`.
- See the [telemetry policy](https://sth-dashboard.vercel.app/docs/telemetry)
  for the full breakdown.
```

For internal-only PRs (CI, docs, tests, chores) the description is whatever
helps the reviewer — it won't reach the public changelog.

## Commit messages

Each commit on `main` becomes a bullet in the categorised changelog. The same
rules as PR titles apply: `<type>(<scope>): <user-facing summary>`. When a PR
is squash-merged, the squash commit subject defaults to the PR title — so
getting the PR title right covers the common case.

## What ships where

- **Code & history** live on `Grandpied33/skills-transfer-hub` (private).
- **Binaries, archives, SHA256SUMS, release notes** live on
  `Grandpied33/sth-releases` (public). Users never see this repo.
- The CLI's `sth update self` reads release metadata (incl. body highlights)
  from the public releases feed.

If you touch the release pipeline, smoke-test locally before tagging:

```
goreleaser check
goreleaser release --snapshot --clean --skip=publish,chocolatey
```
