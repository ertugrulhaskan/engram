#!/usr/bin/env bash
# Verify HOMEBREW_TAP_TOKEN can write the Homebrew tap.
#
# Used by two workflows, which is why it is a script and not an inline step:
# release.yml runs it as a pre-flight gate before GoReleaser publishes anything,
# and verify-tap-token.yml runs it on demand. A second inline copy would drift
# from this one, and the whole point is that both ask the same question.
#
# Reads GH_TOKEN from the environment. Exits 0 only when the token can push.
set -euo pipefail

TAP="${TAP_REPO:-ertugrulhaskan/homebrew-tap}"

if [ -z "${GH_TOKEN:-}" ]; then
  echo "::error::HOMEBREW_TAP_TOKEN is empty or unset in this repo's Actions secrets."
  exit 1
fi

err="$(mktemp)"
trap 'rm -f "$err"' EXIT

if ! push="$(gh api "repos/${TAP}" --jq .permissions.push 2>"$err")"; then
  echo "::error::HOMEBREW_TAP_TOKEN cannot read ${TAP}: $(cat "$err")"
  echo "::error::A 401 means the token is expired or revoked; a 404 means it cannot see the repo at all."
  exit 1
fi

# stdout only: a stderr warning on the success path must not corrupt $push,
# or a good token gets reported as read-only and a valid release is blocked.
case "$push" in
  true)
    echo "tap token OK (permissions.push=true) — GoReleaser can push the cask."
    ;;
  false)
    echo "::error::HOMEBREW_TAP_TOKEN can read ${TAP} but not write it (permissions.push=false)."
    echo "::error::Fix: fine-grained PAT, Only select repositories -> homebrew-tap, Contents: Read and write."
    exit 1
    ;;
  *)
    echo "::error::Unexpected permissions.push=${push:-<empty>} — the response carried no permissions object."
    echo "::error::This is NOT the read-only case; check the token type before changing its permissions."
    exit 1
    ;;
esac
