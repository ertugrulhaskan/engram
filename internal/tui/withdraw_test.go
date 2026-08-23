package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// TestOwnerUnverified covers ENGR-33's reporting rule: an unverifiable owner check
// must name which side was missing, because the fixes differ — a memory with no
// recorded owner is history, an unset git email is one config line away.
func TestOwnerUnverified(t *testing.T) {
	cases := []struct {
		name  string
		own   team.OwnerStatus
		want  string // "" means: disclose nothing
		names string // substring the message must carry, when it discloses
	}{
		{
			name: "verified owner discloses nothing",
			own:  team.OwnerStatus{Owner: "alice@example.com", Me: "alice@example.com"},
			want: "",
		},
		{
			// A mismatch is verifiable — Withdraw refuses it outright with its own
			// error, so the dialog must not muddy that with an "unverified" caution.
			name: "known mismatch discloses nothing",
			own:  team.OwnerStatus{Owner: "alice@example.com", Me: "bob@example.com"},
			want: "",
		},
		{
			name:  "no recorded owner names the memory",
			own:   team.OwnerStatus{Me: "alice@example.com"},
			want:  "disclose",
			names: "records no owner",
		},
		{
			name:  "unset git email names this machine",
			own:   team.OwnerStatus{Owner: "alice@example.com"},
			want:  "disclose",
			names: "git config user.email",
		},
		{
			name:  "both missing names both",
			own:   team.OwnerStatus{},
			want:  "disclose",
			names: "and no git email is set",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ownerUnverified(c.own)
			if c.want == "" {
				if got != "" {
					t.Errorf("expected no disclosure, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected a disclosure, got none")
			}
			if !strings.Contains(got, c.names) {
				t.Errorf("message must name the cause %q, got %q", c.names, got)
			}
		})
	}
}

// TestWithdrawModalShowsUnverifiedOwner proves the caution actually reaches the
// rendered dialog, not just the helper — the helper being right is worthless if
// the modal never calls it.
func TestWithdrawModalShowsUnverifiedOwner(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var tm tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := tm.(Model)
	m.withdrawTitle = "a shared note"
	m.mode = modeWithdrawConfirm

	m.withdrawOwner = team.OwnerStatus{Owner: "alice@example.com", Me: "alice@example.com"}
	if out := m.withdrawModal(); strings.Contains(out, "Ownership unverified") {
		t.Error("a verified owner must not raise the caution")
	}

	m.withdrawOwner = team.OwnerStatus{Owner: "alice@example.com"}
	out := m.withdrawModal()
	if !strings.Contains(out, "Ownership unverified") {
		t.Error("an unverifiable check must be disclosed in the modal")
	}
	if !strings.Contains(out, "still allowed") {
		t.Error("the modal must say the withdraw is still permitted — the guard fails open")
	}
}
