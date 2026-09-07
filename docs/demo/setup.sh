#!/bin/bash
# Stage a fictional demo home for the README screenshot.
# Every name here is invented (an imaginary AI product called "nimbus").
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
SRC="$ROOT/src"
DEMOHOME="$ROOT/home"
CLAUDEHOME="$DEMOHOME/.claude"
PROJECTS="$CLAUDEHOME/projects"
PLANS="$CLAUDEHOME/plans"

rm -rf "$SRC" "$DEMOHOME"
mkdir -p "$SRC" "$PROJECTS" "$PLANS" "$DEMOHOME/.config"

# Claude Code encodes a project path by flattening / . _ (and -) to "-".
encode() { printf '%s' "$1" | tr '/._' '---'; }

make_project() { # $1 = project dir name under $SRC
  local real="$SRC/$1"
  mkdir -p "$real"
  local enc; enc="$(encode "$real")"
  local mem="$PROJECTS/$enc/memory"
  mkdir -p "$mem"
  printf '%s' "$mem"
}

# --- the global rules file (files source, "global" scope) -------------------
cat > "$CLAUDEHOME/CLAUDE.md" <<'EOF'
# User-wide rules

- Ask before touching anything outside the repo I named.
- Small, reviewable steps. Verify one before starting the next.
- Never paste real user data into a prompt, an eval, or a test.
EOF

# --- nimbus-api ------------------------------------------------------------
M="$(make_project nimbus-api)"

cat > "$SRC/nimbus-api/CLAUDE.md" <<'EOF'
# nimbus-api

The Go service behind nimbus: routing, retrieval, and the model calls.

- Prompts live in `prompts/`, versioned in git. Never inline one.
- Every model call goes through `internal/llm` so spend is tracked.
- `make eval` must be green before a prompt change is pushed.
EOF

cat > "$M/model-routing.md" <<'EOF'
---
name: model-routing
description: Haiku classifies the request, Sonnet writes the answer
metadata:
  type: project
---

# Haiku triages, Sonnet answers

Every request hits Haiku first with the intent prompt. Only a
`needs_answer` intent goes on to Sonnet with retrieved context.

- Halves cost at the same p50 latency
- The routing prompt lives in `prompts/route.md`
- On a classifier timeout (>800ms) we skip to Sonnet
EOF

cat > "$M/rag-pipeline-defaults.md" <<'EOF'
---
name: rag-pipeline-defaults
description: The chunking and retrieval settings that beat the baseline
metadata:
  type: project
---

# RAG pipeline defaults

Retrieval was flat until the chunking was fixed.

## What works

- **Chunking:** 512 tokens, 64-token overlap
- **Retrieval:** top-8 by cosine, reranked to top-3
- **Context budget:** 2k tokens of retrieved text

## Why

Smaller chunks kept splitting tables mid-row.

Reranking moved accuracy (+11 pts), not a bigger `top_k`.

Related: [[model-routing]], which reads these chunks.
EOF

cat > "$M/redact-prompts.md" <<'EOF'
---
name: redact-prompts
description: Strip PII before a prompt leaves the process
metadata:
  type: feedback
---

# Never log raw prompts

**Why:** prompts carry emails and account numbers, so one leaked
breadcrumb is a compliance incident.

**How to apply:** send every outbound string through `redact()` in
`internal/privacy`, error reporters included.
EOF

cat > "$M/MEMORY.md" <<'EOF'
# nimbus-api — project memory

- [Haiku triages, Sonnet answers](model-routing.md) — half the cost, same latency
- [RAG pipeline defaults](rag-pipeline-defaults.md) — the settings that beat the baseline
- [Never log raw prompts](redact-prompts.md) — PII stays in-process
EOF

# --- nimbus-chat -----------------------------------------------------------
M="$(make_project nimbus-chat)"

cat > "$SRC/nimbus-chat/CLAUDE.md" <<'EOF'
# nimbus-chat

The web client: composer, streaming transcript, and history.

- pnpm workspaces, `strict: true`, no `any` escapes.
- The transcript renders tokens as they stream. Don't buffer it.
- Run `pnpm test` before every push.
EOF

cat > "$M/sse-streaming.md" <<'EOF'
---
name: sse-streaming
description: Tokens stream over SSE; websockets were dropped on purpose
metadata:
  type: project
---

# Stream tokens over SSE

The composer reads tokens from a plain `EventSource`. We tried
websockets first and dropped them: SSE reconnects for free and the
server stays stateless.
EOF

cat > "$M/user-prefs.md" <<'EOF'
---
name: user-prefs
description: pnpm workspaces, strict TypeScript, small reviewed PRs
metadata:
  type: user
---

# Prefers pnpm + strict TypeScript

The monorepo uses pnpm workspaces. `strict: true` everywhere, no
`any` escapes. Prefers small, reviewable PRs over big drops.
EOF

cat > "$M/claude-api-links.md" <<'EOF'
---
name: claude-api-links
description: The tabs that stay open while debugging model behavior
metadata:
  type: reference
---

# Claude API docs + status page

- Docs: https://docs.anthropic.com
- Status: https://status.anthropic.com
- Spend dashboard: `grafana/nimbus-llm-spend`
EOF

cat > "$M/MEMORY.md" <<'EOF'
# nimbus-chat — project memory

- [Stream tokens over SSE](sse-streaming.md) — stateless server, free reconnects
- [Prefers pnpm + strict TypeScript](user-prefs.md) — workspace and review habits
- [Claude API docs + status page](claude-api-links.md) — debugging bookmarks
EOF

# --- eval-harness ----------------------------------------------------------
M="$(make_project eval-harness)"

cat > "$SRC/eval-harness/CLAUDE.md" <<'EOF'
# eval-harness

Runs the golden set against a prompt or model change and scores it.

- A new case needs a failure-mode tag, or the report can't group it.
- Scores are only comparable within one model version. Say which.
- Never edit a golden answer to make a run pass.
EOF

cat > "$M/golden-set.md" <<'EOF'
---
name: golden-set
description: 40 tagged prompts gate every prompt or model change
metadata:
  type: project
---

# Golden set: 40 tagged prompts

Lives in `evals/golden/`. Every case carries a failure-mode tag
(`hallucination`, `refusal`, `format`), so a regression names what
broke instead of just dropping a score.
EOF

cat > "$M/run-evals-first.md" <<'EOF'
---
name: run-evals-first
description: No prompt change ships without a green eval run
metadata:
  type: feedback
---

# Run evals before prompt edits

**Why:** prompt edits look harmless and regress quietly; the golden
set catches what code review cannot.

**How to apply:** run `make eval` before pushing anything under
`prompts/`, and paste the score table into the PR.
EOF

cat > "$M/score-dashboard.md" <<'EOF'
---
name: score-dashboard
description: Where the nightly eval scores land
metadata:
  type: reference
---

# Nightly eval scores dashboard

- Scores: `grafana/nimbus-evals`, per failure-mode tag
- Raw transcripts: `s3://nimbus-evals/runs/`
EOF

cat > "$M/MEMORY.md" <<'EOF'
# eval-harness — project memory

- [Golden set: 40 tagged prompts](golden-set.md) — regressions name what broke
- [Run evals before prompt edits](run-evals-first.md) — make eval, paste the table
- [Nightly eval scores dashboard](score-dashboard.md) — grafana and raw transcripts
EOF

# --- plans (~/.claude/plans, what plan mode writes) -------------------------
cat > "$PLANS/streaming-tool-use.md" <<'EOF'
# Plan: Stream tool use into the composer

Tool calls currently land only after the turn finishes, so a long
search looks like a hang.

1. Emit `tool_use` and `tool_result` as their own SSE events
2. Render a collapsed row per call, expandable on click
3. Keep the transcript scrolled to the newest event
4. Add a Playwright case for a turn with two tool calls
EOF

cat > "$PLANS/prompt-cache-rollout.md" <<'EOF'
# Plan: Cache the system prompt

The system prompt and the tool definitions are resent on every turn.

1. Split the prompt into a stable prefix and the per-turn tail
2. Mark the prefix as cacheable
3. Log cache reads and writes per request
4. Compare spend over a week before rolling it out everywhere
EOF

cat > "$PLANS/cross-encoder-rerank.md" <<'EOF'
# Plan: Try a cross-encoder reranker

Reranking is the step that moved accuracy, so it is worth a better
model before anything else is tuned.

1. Score the golden set with the current reranker as a baseline
2. Swap in the cross-encoder behind a flag
3. Compare accuracy and added latency per failure-mode tag
4. Keep it only if p95 latency stays under budget
EOF

echo "demo home staged at: $DEMOHOME"
