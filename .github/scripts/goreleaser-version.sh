#!/usr/bin/env bash
# Print the pinned GoReleaser version as a step output line (`version=vX.Y.Z`).
#
# The pin lives in .github/goreleaser-version and nowhere else. release.yml publishes
# with it and ci.yml's `snapshot` job rehearses with it, so both read this one file
# instead of each carrying a copy that could drift: the whole value of the rehearsal is
# that it runs the exact version a real release will.
#
# The content is checked against the full `vX.Y.Z` shape on purpose. goreleaser-action
# also accepts `~> v2`, `latest` and `nightly`, and any of those would quietly turn the
# pin back into a moving target.
set -euo pipefail

file="$(cd "$(dirname "$0")/.." && pwd)/goreleaser-version"
if [ ! -r "$file" ]; then
  echo "::error::$file is missing; it holds the GoReleaser version both workflows run." >&2
  exit 1
fi
version="$(tr -d '[:space:]' < "$file")"

# stdout is the step-output file (`>> "$GITHUB_OUTPUT"`), so the error must go to stderr
# to reach the log; written into the output file it would be hidden and could trip the
# runner's own parsing of that file.
if ! [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::$file must hold one exact GoReleaser version like v2.19.0, got '$version'." >&2
  exit 1
fi

echo "version=$version"
