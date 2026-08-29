// Package source holds the per-source capability model: what the user may be
// offered to do to the files a browsable source shows. It contains no UI and
// no IO — it exists so capability is a fact each source's data package
// declares once, not a srcKind comparison scattered through the TUI.
package source

// Caps is one source's capability set (ENGR-12). The zero value grants
// nothing: a new source is read-only until a capability is explicitly
// granted, so forgetting to declare one fails as a missing feature, never as
// a silently granted write.
//
// The governing test for granting Edit to a source: engram can keep the
// promise "stay compatible with the tool that owns this file". It can for
// Claude memories, where engram only ever adds frontmatter keys it owns; it
// cannot for another vendor's instruction files, which therefore stay
// read-only in-app with repairs routed through the assistant seam instead.
type Caps struct {
	Edit   bool // open the selected file in $EDITOR
	Create bool // create a new file in this source
	Delete bool // delete the selected file
	Share  bool // team actions (promote / pull / withdraw / resolve) and batch marks
}
