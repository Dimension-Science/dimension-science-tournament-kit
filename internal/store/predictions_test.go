package store

import "testing"

func TestPredictionPointsForRound(t *testing.T) {
	tests := []struct {
		round string
		want  int
	}{
		{round: "quarterfinal", want: 5},
		{round: "semifinal", want: 15},
		{round: "final", want: 50},
		{round: "third_place", want: 0},
	}

	for _, test := range tests {
		t.Run(test.round, func(t *testing.T) {
			if got := PredictionPointsForRound(test.round); got != test.want {
				t.Fatalf("PredictionPointsForRound(%q) = %d, want %d", test.round, got, test.want)
			}
		})
	}
}

func TestValidPickemChain(t *testing.T) {
	quarterfinals := [4]string{"a", "b", "c", "d"}
	if !validPickemChain(quarterfinals, [2]string{"a", "d"}, "d") {
		t.Fatal("expected coherent bracket to be valid")
	}
	if validPickemChain(quarterfinals, [2]string{"c", "d"}, "d") {
		t.Fatal("expected semifinal winner from the wrong branch to be invalid")
	}
	if validPickemChain(quarterfinals, [2]string{"a", "d"}, "b") {
		t.Fatal("expected finalist outside semifinal winners to be invalid")
	}
}
