package tui

import "github.com/ertugrulhaskan/engram/internal/team"

// offeredAction maps a memory's sync state to the one team action the UI
// offers for it. It is the single source of truth consumed by the contextual
// status bar (and later the preview sync strip), so the advertised key and the
// key that actually works can never disagree.
//
// behind offers pull for both scopes — Pull reconciles an existing local copy
// of a global memory too (see team/pull.go), so `p` is live everywhere behind
// appears. ahead and missing offer promote (missing means re-promote puts it
// back); conflict and unknown offer resolve. synced rows and personal rows
// offer nothing — team keys are never advertised where they can't act.
func offeredAction(s team.SyncState) (key, verb string) {
	switch s {
	case team.StateIncoming:
		return "p", "pull"
	case team.StateLocalAhead, team.StateMissing:
		return "P", "promote"
	case team.StateDiverged, team.StateDiffers:
		return "r", "resolve"
	default:
		return "", ""
	}
}

// stateSentence is the design spec's plain sentence for a sync state, shown in
// the preview's sync strip. Empty for StateNone (the strip doesn't render).
func stateSentence(s team.SyncState) string {
	switch s {
	case team.StateSynced:
		return "In sync with the team store."
	case team.StateIncoming:
		return "The team copy moved ahead. Yours is untouched."
	case team.StateLocalAhead:
		return "You have edits the team has not seen yet."
	case team.StateDiverged:
		return "Both sides changed since you last synced."
	case team.StateMissing:
		return "Promoted once, but it is not in the store anymore."
	case team.StateDiffers:
		return "Shared before sync tracking existed, so there is no direction."
	default:
		return ""
	}
}

// gaugeGlyph is the direction glyph between the gauge's two bars: which side
// moved, both, neither, or unknowable. Words can be misread ("behind" sounds
// broken); the glyph on the gauge is not.
func gaugeGlyph(s team.SyncState) string {
	switch s {
	case team.StateSynced:
		return "="
	case team.StateIncoming:
		return "←"
	case team.StateLocalAhead:
		return "→"
	case team.StateDiverged:
		return "↔"
	case team.StateMissing:
		return "✕"
	case team.StateDiffers:
		return "?"
	default:
		return ""
	}
}
