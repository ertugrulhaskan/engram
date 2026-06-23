package tui

import (
	"testing"
	"time"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

func TestRecencyBucket(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		mod     time.Time
		wantKey string
	}{
		{"just now", now.Add(-1 * time.Minute), "today"},
		{"hours ago", now.Add(-5 * time.Hour), "today"},
		{"two days", now.Add(-2 * 24 * time.Hour), "week"},
		{"six days", now.Add(-6 * 24 * time.Hour), "week"},
		{"two weeks", now.Add(-14 * 24 * time.Hour), "older"},
		{"zero time", time.Time{}, "older"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := recencyBucket(c.mod); got != c.wantKey {
				t.Errorf("recencyBucket(%v) = %q, want %q", c.mod, got, c.wantKey)
			}
		})
	}
}

// planItems groups by recency: contiguous buckets, each with its own header
// color, in newest-first order.
func TestPlanItemsGrouping(t *testing.T) {
	now := time.Now()
	plans := []plan.Plan{
		{Title: "fresh", Body: "# fresh\n", Path: "/p/a.md", Modified: now.Add(-2 * time.Hour)},
		{Title: "midweek", Body: "# midweek\n", Path: "/p/b.md", Modified: now.Add(-3 * 24 * time.Hour)},
		{Title: "ancient", Body: "# ancient\n", Path: "/p/c.md", Modified: now.Add(-30 * 24 * time.Hour)},
	}
	m := New(nil, plans, config.Config{})
	items := m.planItems()
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	wantKeys := []string{"today", "week", "older"}
	for i, it := range items {
		if it.GroupKey != wantKeys[i] {
			t.Errorf("item %d GroupKey=%q, want %q", i, it.GroupKey, wantKeys[i])
		}
		if it.Kind != "plan" {
			t.Errorf("item %d Kind=%q, want plan", i, it.Kind)
		}
		if it.GroupColor == "" {
			t.Errorf("item %d has no GroupColor", i)
		}
	}
	if items[0].GroupColor == items[1].GroupColor {
		t.Error("adjacent buckets share a header color")
	}
}
