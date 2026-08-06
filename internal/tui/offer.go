package tui

import "github.com/ertugrulhaskan/engram/internal/team"

// offeredAction maps a memory's sync state and scope to the one team action the
// UI offers for it. It is the single source of truth consumed by the contextual
// status bar (and later the preview sync strip), so the advertised key and the
// key that actually works can never disagree.
//
// behind+project offers pull; behind+global offers resolve instead — Pull
// deliberately walks past global memories (see team/pull.go), so advertising
// `p` there would be a dead key. ahead and missing offer promote (missing means
// re-promote puts it back); conflict and unknown offer resolve. synced rows and
// personal rows offer nothing — team keys are never advertised where they can't
// act.
func offeredAction(s team.SyncState, scope string) (key, verb string) {
	switch s {
	case team.StateIncoming:
		if scope == "global" {
			return "r", "resolve"
		}
		return "p", "pull"
	case team.StateLocalAhead, team.StateMissing:
		return "P", "promote"
	case team.StateDiverged, team.StateDiffers:
		return "r", "resolve"
	default:
		return "", ""
	}
}
