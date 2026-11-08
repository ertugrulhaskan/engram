package team

import "sync"

// storeMu serialises the store operations the TUI runs off its event loop:
// promote, withdraw and pull, which commit, push or fetch in the clone and
// stamp local memories. Without it a disconnect confirmed while a push was
// still running could move the clone out from under it, and miss the memory
// that push was about to stamp team. Disconnect only tries the lock and
// refuses while it is held; the others wait their turn.
//
// FinishConflictResolve doesn't take it: the TUI calls it on its event loop,
// where waiting behind a slow push would freeze the screen, and the resolve
// editor holds the terminal until then, so no disconnect can start meanwhile.
// Read-only lookups (sync states, a store copy's last change) don't take it
// either: they are best-effort, and a disconnect at worst makes one fail.
var storeMu sync.Mutex
