package tui

import "testing"

func TestFitFurnitureShedsColumns(t *testing.T) {
	// Wide pane: everything fits — nothing is shed.
	m := Model{listW: 80}
	if sc, rc := m.fitFurniture(6, 7, 11, 8); sc != 7 || rc != 8 {
		t.Errorf("wide pane should keep all furniture, got scope=%d right=%d", sc, rc)
	}

	// Tight pane: the scope chip drops first; the Right column survives once the
	// title (>= minTitleW) fits, protecting the color-coded sync pill.
	m = Model{listW: 40}
	if sc, rc := m.fitFurniture(6, 7, 11, 8); sc != 0 || rc != 8 {
		t.Errorf("tight pane should shed only the scope chip, got scope=%d right=%d", sc, rc)
	}

	// Very narrow pane: both scope and Right drop; badge + pill alone remain.
	m = Model{listW: 24}
	if sc, rc := m.fitFurniture(6, 7, 11, 8); sc != 0 || rc != 0 {
		t.Errorf("narrow pane should shed scope and right, got scope=%d right=%d", sc, rc)
	}
}
