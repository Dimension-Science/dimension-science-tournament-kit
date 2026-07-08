package discordbot

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/dimension-science/tournament-kit/internal/store"
)

func TestNormalizeDiscordName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Runner", "runner"},
		{"  Runner  ", "runner"},
		{"@Runner", "runner"},
		{"Runner Name", "runnername"},
		{"geekfreak#0", "geekfreak"},
		{"geekfreak#0000", "geekfreak"},
		{"geekfreak#1234", "geekfreak#1234"},
	}

	for _, tt := range tests {
		actual := normalizeDiscordName(tt.input)
		if actual != tt.expected {
			t.Errorf("normalizeDiscordName(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestNormalizedDiscordNames(t *testing.T) {
	member := &discordgo.Member{
		Nick: "NickName",
		User: &discordgo.User{
			Username:      "UserName",
			GlobalName:    "GlobalName",
			Discriminator: "0",
		},
	}

	names := normalizedDiscordNames(member)

	expected := []string{"nickname", "username", "globalname"}
	for _, exp := range expected {
		if !names[exp] {
			t.Errorf("Expected names to contain %q, but it did not. Got: %v", exp, names)
		}
	}

	// Test with discriminator
	memberWithDisc := &discordgo.Member{
		User: &discordgo.User{
			Username:      "UserName",
			Discriminator: "1234",
		},
	}
	namesWithDisc := normalizedDiscordNames(memberWithDisc)
	if !namesWithDisc["username#1234"] {
		t.Errorf("Expected names to contain 'username#1234', got: %v", namesWithDisc)
	}

	// Test with discriminator "0000"
	memberWithZeroDisc := &discordgo.Member{
		User: &discordgo.User{
			Username:      "UserName",
			Discriminator: "0000",
		},
	}
	namesWithZeroDisc := normalizedDiscordNames(memberWithZeroDisc)
	if namesWithZeroDisc["username#0000"] {
		t.Errorf("Expected names to not contain 'username#0000', got: %v", namesWithZeroDisc)
	}
	if !namesWithZeroDisc["username"] {
		t.Errorf("Expected names to contain 'username', got: %v", namesWithZeroDisc)
	}
}

func TestMatchApplicationForMember(t *testing.T) {
	appByName := map[string]*store.TournamentApplication{
		"geekfreak": {
			ID:              "app1",
			DiscordUsername: "Runner#0",
			Status:          "approved",
		},
		"runner123": {
			ID:              "app2",
			DiscordUsername: "Runner123",
			Status:          "approved",
		},
	}

	member1 := &discordgo.Member{
		User: &discordgo.User{
			Username:      "geekfreak",
			Discriminator: "0000",
		},
	}
	app1 := matchApplicationForMember(appByName, member1)
	if app1 == nil || app1.ID != "app1" {
		t.Errorf("Expected match for geekfreak, got %v", app1)
	}

	member2 := &discordgo.Member{
		Nick: "Runner 123",
	}
	app2 := matchApplicationForMember(appByName, member2)
	if app2 == nil || app2.ID != "app2" {
		t.Errorf("Expected match for runner123, got %v", app2)
	}

	member3 := &discordgo.Member{
		User: &discordgo.User{
			Username:      "unmatched",
			Discriminator: "0",
		},
	}
	app3 := matchApplicationForMember(appByName, member3)
	if app3 != nil {
		t.Errorf("Expected no match for unmatched user, got %v", app3)
	}
}
