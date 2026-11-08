#!/usr/bin/env bash
# Verify what a GoReleaser snapshot build produced, after
# `goreleaser release --snapshot --clean --skip=publish`.
#
# ci.yml's `snapshot` job runs this on every push to main and every pull request, and it
# is meant to be run locally before a tag too (CONTRIBUTING's "Releasing"). The two
# failures it looks for are the ones a release would otherwise be the first to show:
#
#   1. The cask's quarantine hook. The install hook in .goreleaser.yaml must render to
#      Homebrew's `postflight_steps` block holding the exact xattr call verified on
#      2026-09-05, with Homebrew's own `{{staged_path}}` token. The GoReleaser spelling
#      `.StagedPath` pasted raw into a .rb, a dropped `-r`, a changed attribute name, or
#      the deprecated `postflight do` stanza all install cleanly and leave the binary
#      quarantined, so macOS refuses to run it. Nothing else in CI sees the rendered cask.
#   2. The version stamp. Release binaries print `engram vX.Y.Z`, with the `v`; the
#      ldflags in .goreleaser.yaml add it, and v0.6.1 had to fix that. The cask's own
#      `version` line must name the same release, since both come from one template value.
#
# This cannot prove the hook clears quarantine on a Mac: that remains the manual
# `brew reinstall` check described beside the hook in .goreleaser.yaml. It proves the
# rendered text is the one that check was verified on.
#
# Usage: verify-snapshot.sh [dist-dir]   (default ./dist). Exits 0 only when all pass.
set -euo pipefail

dist="${1:-dist}"
cask="$dist/homebrew/Casks/engram.rb"
fail=0

problem() {
  echo "::error::$*"
  fail=1
}

if [ ! -f "$cask" ]; then
  echo "::error::$cask was not generated; run the snapshot build first."
  exit 1
fi

# --- 1. the quarantine hook, as rendered -------------------------------------------
# The whole call, verbatim: the path token alone would still match a hook that lost
# its -r or clears a different attribute, and both of those leave the binary quarantined.
hook='run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{staged_path}}/engram"]'
grep -qF 'postflight_steps do' "$cask" \
  || problem "$cask has no 'postflight_steps do' block; the install hook did not render."
grep -qF "$hook" "$cask" \
  || problem "$cask does not contain the verified xattr call: $hook"
if grep -qF 'postflight do' "$cask"; then
  problem "$cask uses the deprecated 'postflight do' stanza; Homebrew 6 warns on every brew command."
fi
if grep -qF 'StagedPath' "$cask"; then
  problem "$cask contains the GoReleaser spelling 'StagedPath'; the Homebrew token did not render."
fi

# --- 2. the version stamp ------------------------------------------------------------
# The binary for this host: GoReleaser names the folder <build id>_<os>_<arch>_<variant>.
goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
binary="$(find "$dist" -maxdepth 2 -type f -path "$dist/engram_${goos}_${goarch}_*/engram" | head -n 1)"
if [ -z "$binary" ]; then
  problem "no $goos/$goarch binary under $dist; the build matrix changed or the snapshot failed."
elif ! printed="$("$binary" --version 2>&1)"; then
  problem "'$binary --version' failed: $printed"
else
  if ! [[ "$printed" =~ ^engram\ v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
    problem "'$binary --version' printed '$printed'; expected 'engram vX.Y.Z...' (the ldflags lost the 'v'?)."
  else
    cask_version="$(sed -n 's/^  version "\(.*\)"$/\1/p' "$cask" | head -n 1)"
    if [ "$printed" != "engram v$cask_version" ]; then
      problem "binary prints '$printed' but the cask says version \"$cask_version\"; the two should agree."
    fi
  fi
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "Snapshot verified: the cask carries the postflight_steps xattr call with {{staged_path}}, and $binary prints '$printed' matching the cask."
