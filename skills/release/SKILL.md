---
name: release
description: "Trigger: release, version, tag, changelog, release notes. Create versioned releases with categorized notes from conventional commits."
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use when the user asks to create a release, bump the version, generate release notes, or publish a new version. Also use when saying "genera el release", "release notes", "new version", "bump version", or "tag and publish".

## Hard Rules

- NEVER create a release without running `go build ./...` and `go test ./... -count=1` first. Both must pass.
- NEVER push a tag without user confirmation.
- Use **semantic versioning**: `feat:` → minor bump, `fix:` → patch bump, `BREAKING CHANGE` or `!:` → major bump.
- Release notes MUST categorize commits. Do not dump a flat list.
- The tag triggers goreleaser via GitHub Actions — do NOT run goreleaser locally.

## Execution Steps

1. **Detect current version**:
   ```bash
   git tag -l 'v*' --sort=-v:refname | head -1
   ```

2. **List commits since last tag**:
   ```bash
   git log <last-tag>..HEAD --oneline
   ```

3. **Determine next version** using conventional commits:

   | Commit prefix | Version bump |
   |---------------|-------------|
   | `feat:` | minor (0.X.0) |
   | `fix:` | patch (0.0.X) |
   | `refactor:`, `chore:`, `docs:` | patch |
   | `BREAKING CHANGE` or `!:` in any commit | major (X.0.0) |

   If mixed, use the highest bump. If only `feat:` and `fix:`, use minor.

4. **Generate release notes** using this template:

   ```markdown
   ## What's New

   ### Features
   - {feat commits, one bullet per commit, human-readable}

   ### Fixes
   - {fix commits}

   ### Improvements
   - {refactor/chore commits}

   ### Breaking Changes
   - {only if applicable}

   ## Full Changelog
   https://github.com/bsantosio/claudio/compare/{prev-tag}...{new-tag}
   ```

   Omit empty sections. Write descriptions in human-readable English, not raw commit messages. Group related commits into single bullets when they form one logical change.

5. **Confirm with user**: Show the proposed version and notes summary. Ask: "Tag v{X.Y.Z} and create release?"

6. **Create tag and release**:
   ```bash
   git tag v{X.Y.Z}
   git push origin v{X.Y.Z}
   gh release create v{X.Y.Z} --title "claudio v{X.Y.Z}" --notes "{notes}"
   ```

7. **Verify**: Check that GitHub Actions triggered the goreleaser workflow. Report the release URL.

## Output Contract

Return:
- Release URL
- Version number
- Summary of what was included
- Reminder: "Homebrew tap updates automatically. Users run `brew upgrade claudio`."
