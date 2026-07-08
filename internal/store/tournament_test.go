package store

import "testing"

func TestBestOfForRound(t *testing.T) {
	for _, round := range []string{"quarterfinal", "semifinal", "third_place", "final"} {
		t.Run(round, func(t *testing.T) {
			if got := bestOfForRound(round); got != 1 {
				t.Fatalf("bestOfForRound(%q) = %d, want 1", round, got)
			}
		})
	}
}
