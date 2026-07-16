package telegrambot

import "testing"

func TestChaosApplicationKeyboard(t *testing.T) {
	tests := []struct {
		step int
		want string
	}{
		{step: 1, want: buttonCancel},
		{step: 5, want: buttonSkip},
		{step: 6, want: buttonSkip},
		{step: 7, want: buttonSend},
	}

	for _, tt := range tests {
		keyboard := chaosApplicationKeyboard(tt.step)
		rows := keyboard["keyboard"].([][]map[string]string)
		if got := rows[0][0]["text"]; got != tt.want {
			t.Fatalf("step %d: first button = %q, want %q", tt.step, got, tt.want)
		}
	}
}
