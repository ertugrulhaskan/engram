# engram: brand and voice

The source of truth for how engram talks about itself, on the site and in the TUI.
Product behavior lives in [SPEC.md](SPEC.md); this file governs wording and claims.

## What it is, in one sentence

engram is a single-binary terminal UI for the memories Claude Code writes to `~/.claude`,
with team sharing over plain git.

## Who reads it

A developer who already uses Claude Code daily. They are comfortable in a terminal, skeptical
of another SaaS, and tired of tools that want an account before they show anything. They are not
looking to be sold to. They are deciding in about thirty seconds whether to run one install
command.

## What they use today

`grep` and `cat` over `~/.claude`, or nothing at all. The competition is not another product;
it is doing without.

## The page's one job

Get a `brew install` (or `go install`) copied and run. Everything else on the site exists to
remove a reason not to.

## Their biggest objection

"Does this ship my notes somewhere?" Privacy and lock-in come before features. The answer must
be reachable within one scroll of the fold, and it must be verifiable, not asserted.

## Proof we actually have

Open source under MIT · the binary runs with no account and no engram service · memories stay
files on disk · a public changelog and tagged releases · a real capture of the TUI in the
README, and page mockups that reproduce its strings and palette exactly. That is the complete
list.

## Tone

Plain · precise · unhurried. Write like a good README, not like a launch post. State the
mechanism instead of the adjective: "promote a memory; teammates pull it" over "seamless team
collaboration". Confidence comes from being specific, never from intensifiers.

Two rules that can be measured, both given while reviewing the landing page. They bind the
site and `llms.txt` today; README and the TUI's own strings follow as they are touched, so a
dash already there is not yet a defect:

- **No em dashes.** Rewrite rather than swapping the glyph: a colon when the clause introduces
  a list, a semicolon when two statements balance, or two sentences. En dashes in numeric
  ranges are fine. Two carve-outs: a terminal mockup that reproduces a Go string byte for byte
  keeps its dash until the Go string changes, and HTML comments are not copy.
- **Match the neighbours in length.** A string several times the length of its siblings is the
  defect, even when every sentence in it is true. Measure it: FAQ answers sit around 20 to 40
  words and roadmap cards around 15 to 30. Cut to fit before showing it.

## Vocabulary

**Use:** memory / memories · promote · withdraw · pull · plan · the palette · TUI · single
binary · plain git · your git remote · read-only until you act.

**Avoid:** "sync" (implies a service we don't run) · "cloud" · "platform" · "AI-powered" ·
"seamless" · "supercharge" · "unlock" · "revolutionize" · anything implying an account,
a server, or a subscription.

## Naming and casing

- **engram** is lowercase everywhere, including at the start of a sentence.
- **Claude Code** is the product; **Claude** is the assistant. Not "Claude-Code", not "claude".
- Commands, paths, and flags in code type: `engram init-team`, `~/.claude`, `$EDITOR`.
- Sentence case for headings, on the site and in the UI.

## Claims we must not make

- **No platform we haven't verified.** macOS and Linux only. Native Windows is unverified
  and, as of 2026-09-06, not planned (SPEC §10), so treat this as settled rather than
  temporary: the site and README must not list it. The Windows archives that ship are
  labelled untested and must never be worded as support.
- **No invented proof.** No user counts, no testimonials, no logos, no ratings, no "trusted by".
- **No blurring of shipped vs planned.** If ROADMAP says in progress, the page says in progress.
- **No promise beyond what the tool does.** engram never modifies a user's memory files except
  on an explicit action, and only ever adds its own frontmatter. Copy must not imply more.
