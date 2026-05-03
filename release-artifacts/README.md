# Release artifacts — Homebrew tap & Scoop bucket

GoReleaser auto-pushes the Homebrew Formula and Scoop manifest to the tap and
bucket repos on every release. No manual sync needed.

## One-time setup

Create the three distribution repos as **public** with "Initialize with README"
checked (so they have a default branch GoReleaser can push to):

- `Grandpied33/sth-releases` — public artifacts repo (where binaries land)
- `Grandpied33/homebrew-sth` — public Homebrew tap
- `Grandpied33/scoop-sth` — public Scoop bucket

Then create a fine-grained PAT (`RELEASE_PAT`) with **Contents: Read and write**
on all three repos, and store it as a secret named `RELEASE_PAT` in the source
repo (`skills-transfer-hub`) under Settings → Secrets and variables → Actions.

## Per-release flow

```bash
git tag v0.2.0
git push origin v0.2.0
# Then publish the GitHub Release for v0.2.0 on Grandpied33/sth-releases
# (or trigger the release.yml workflow_dispatch).
```

That's it. GoReleaser will:
1. Build all binaries on the CI runner
2. Upload archives + SHA256SUMS to the GitHub Release on `sth-releases`
3. Push `Formula/sth.rb` to `homebrew-sth`
4. Push `bucket/sth.json` to `scoop-sth`

Verify post-release:

```bash
brew tap grandpied33/sth
brew install grandpied33/sth/sth
sth version    # should print the new tag

scoop bucket add sth https://github.com/Grandpied33/scoop-sth
scoop install sth
sth version    # should print the new tag
```

## Files in this folder

`homebrew-sth/` and `scoop-sth/` contain placeholder Formula/manifest files —
kept as a reference of what GoReleaser generates. They are NOT used at release
time anymore (auto-PR overwrites the real ones in the tap/bucket repos
directly). You can ignore them or delete them.
