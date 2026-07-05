#!/bin/bash
# Stage a fictional demo home for the README screenshot.
# All names are fictional (an imaginary AI app called "nimbus").
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
SRC="$ROOT/src"
DEMOHOME="$ROOT/home"
PROJECTS="$DEMOHOME/.claude/projects"

rm -rf "$SRC" "$DEMOHOME"
mkdir -p "$SRC" "$PROJECTS" "$DEMOHOME/.config"

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

# --- nimbus-api ------------------------------------------------------------
M="$(make_project nimbus-api)"

cat > "$M/model-routing.md" <<'EOF'
---
name: model-routing
description: Haiku classifies the request; Sonnet writes the answer
metadata:
  type: project
---

# Model routing: Haiku triages, Sonnet answers

Every request hits Haiku first with the 12-label intent prompt; only
`needs_answer` intents get forwarded to Sonnet with retrieved context.

- Cuts cost roughly in half at p50 latency parity
- Routing prompt lives in `prompts/route.md`, versioned in git
- Fallback: on classifier timeout (>800ms) we skip straight to Sonnet
EOF

cat > "$M/rag-pipeline-defaults.md" <<'EOF'
---
name: rag-pipeline-defaults
description: Chunking, retrieval and rerank settings that beat the eval baseline
metadata:
  type: project
---

# RAG pipeline defaults: chunking + retrieval

Retrieval was flat until chunking was fixed — these beat the baseline.

## What works

- **Chunking:** 512 tokens, 64-token overlap, split on headings first
- **Retrieval:** top-8 by cosine, reranked down to top-3
- **Context budget:** retrieved chunks capped at 2k tokens total

## Why

Smaller chunks kept splitting tables mid-row.

The rerank step moved accuracy (+11 pts) — not a bigger `top_k`.

Related: [[model-routing]] — that model consumes these chunks.
EOF

cat > "$M/redact-prompts.md" <<'EOF'
---
name: redact-prompts
description: Strip PII before prompts leave the process — Sentry included
metadata:
  type: feedback
---

# Never log raw prompts — redact before Sentry

**Why:** user prompts contain emails and account numbers; one leaked
breadcrumb is a compliance incident.

**How to apply:** pass every outbound string through `redact()` in
`internal/privacy` — including error reporters and debug logs.
EOF

cat > "$(dirname "$M")/memory/MEMORY.md" <<'EOF'
# nimbus-api — project memory

- [Model routing: Haiku triages, Sonnet answers](model-routing.md) — cost cut, latency parity
- [RAG pipeline defaults: chunking + retrieval](rag-pipeline-defaults.md) — the settings that beat the baseline
- [Never log raw prompts — redact before Sentry](redact-prompts.md) — PII stays in-process
EOF

# --- nimbus-chat -----------------------------------------------------------
M="$(make_project nimbus-chat)"

cat > "$M/sse-streaming.md" <<'EOF'
---
name: sse-streaming
description: Tokens stream over SSE; websockets were dropped on purpose
metadata:
  type: project
---

# Stream tokens over SSE, not websockets

The composer renders tokens from a plain `EventSource`. We tried
websockets first and dropped them: SSE reconnects for free, plays nice
with the CDN, and the server stays stateless.
EOF

cat > "$M/user-prefs.md" <<'EOF'
---
name: user-prefs
description: pnpm workspaces, strict TypeScript, small reviewed PRs
metadata:
  type: user
---

# Prefers pnpm + strict TypeScript

Monorepo uses pnpm workspaces. `strict: true` everywhere — no `any`
escapes. Prefers small, reviewable PRs over big drops.
EOF

cat > "$M/claude-api-links.md" <<'EOF'
---
name: claude-api-links
description: The three tabs open during any model-behavior debugging
metadata:
  type: reference
---

# Claude API docs + status page

- Docs: https://docs.anthropic.com
- Status: https://status.anthropic.com
- Internal cost dashboard: `grafana/nimbus-llm-spend`
EOF

cat > "$(dirname "$M")/memory/MEMORY.md" <<'EOF'
# nimbus-chat — project memory

- [Stream tokens over SSE, not websockets](sse-streaming.md) — stateless server, CDN-friendly
- [Prefers pnpm + strict TypeScript](user-prefs.md) — workspace + review habits
- [Claude API docs + status page](claude-api-links.md) — debugging bookmarks
EOF

# --- eval-harness ----------------------------------------------------------
M="$(make_project eval-harness)"

cat > "$M/golden-set.md" <<'EOF'
---
name: golden-set
description: 40 tagged prompts gate every prompt or model change
metadata:
  type: project
---

# Golden set: 40 prompts, tagged by failure mode

Lives in `evals/golden/`. Each case is tagged (`hallucination`,
`refusal`, `format`) so a regression names its failure mode instead of
just a score drop.
EOF

cat > "$M/run-evals-first.md" <<'EOF'
---
name: run-evals-first
description: No prompt change ships without a green eval run
metadata:
  type: feedback
---

# Run the evals before every prompt change

**Why:** prompt edits look harmless and regress silently — the golden
set catches what code review can't.

**How to apply:** `make eval` before pushing anything under `prompts/`;
paste the score table into the PR description.
EOF

cat > "$M/score-dashboard.md" <<'EOF'
---
name: score-dashboard
description: Where the nightly eval scores land
metadata:
  type: reference
---

# Nightly eval scores dashboard

- Scores: `grafana/nimbus-evals` (nightly run, per failure-mode tag)
- Raw transcripts: `s3://nimbus-evals/runs/`
EOF

cat > "$(dirname "$M")/memory/MEMORY.md" <<'EOF'
# eval-harness — project memory

- [Golden set: 40 prompts, tagged by failure mode](golden-set.md) — regressions name their failure mode
- [Run the evals before every prompt change](run-evals-first.md) — make eval, paste the table
- [Nightly eval scores dashboard](score-dashboard.md) — grafana + raw transcripts
EOF

echo "demo home staged at: $DEMOHOME"
