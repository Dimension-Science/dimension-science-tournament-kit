package web

import (
	"sort"
	"strconv"
	"time"

	"github.com/dimension-science/tournament-kit/internal/store"
)

const defaultPlayoffSlots = 8

type tournamentStatusView struct {
	Available           bool
	RegistrationOpen    bool
	Phase               string
	PhaseLabel          string
	StateLabel          string
	StartsAt            time.Time
	QualificationEndsAt time.Time
	PlayoffEndsAt       time.Time
	EndsAt              time.Time
	PlayoffSlots        int
	BracketReady        bool
	BracketOpen         bool
	CountdownValue      string
	CountdownLabel      string
	CountdownDetail     string
	ShowCountdown       bool
	PrimaryLine         string
	SecondaryLine       string
}

type bracketMatchView struct {
	ID              string
	Round           string
	RoundLabel      string
	Position        int
	Status          string
	StatusLabel     string
	StartsAt        time.Time
	EndsAt          time.Time
	BestOf          int
	Player1Name     string
	Player2Name     string
	Player1ID       string
	Player2ID       string
	Player1Seed     string
	Player2Seed     string
	Player1BestTime any
	Player2BestTime any
	WinnerName      string
}

type bracketRoundView struct {
	Key     string
	Label   string
	Matches []bracketMatchView
}

func defaultTournamentSchedule(now time.Time) (time.Time, time.Time, time.Time, time.Time, int) {
	startsAt := now.UTC()
	qualificationEndsAt := startsAt.Add(14 * 24 * time.Hour)
	endsAt := startsAt.Add(30 * 24 * time.Hour)
	playoffEndsAt := endsAt.Add(-3 * 24 * time.Hour)
	return startsAt, qualificationEndsAt, playoffEndsAt, endsAt, defaultPlayoffSlots
}

func validTournamentSchedule(startsAt, qualificationEndsAt, playoffEndsAt, endsAt time.Time, slots int) bool {
	return slots >= 2 &&
		slots <= 16 &&
		qualificationEndsAt.After(startsAt) &&
		playoffEndsAt.After(qualificationEndsAt) &&
		endsAt.After(playoffEndsAt)
}

func playoffCutoff(tournament *store.Tournament) int {
	if tournament == nil || tournament.PlayoffSlots <= 0 {
		return defaultPlayoffSlots
	}
	return tournament.PlayoffSlots
}

func phaseFromTournament(tournament *store.Tournament) string {
	if tournament == nil {
		return "scheduled"
	}
	return tournament.Phase
}

func buildTournamentStatusView(tournament *store.Tournament) tournamentStatusView {
	now := time.Now()
	if tournament == nil {
		return tournamentStatusView{
			Available:        false,
			RegistrationOpen: true,
			PhaseLabel:       "Турнир не запущен",
			StateLabel:       "нет активного расписания",
			PlayoffSlots:     defaultPlayoffSlots,
			CountdownValue:   "-",
			CountdownLabel:   "дней",
			CountdownDetail:  "Квалификация начнется после старта турнира.",
			PrimaryLine:      "Турнир пока не запущен.",
			SecondaryLine:    "Формат: квалификация, затем плей-офф и финал.",
		}
	}
	phaseLabel := phaseLabel(tournament.Phase)
	registrationOpen := tournament.State != "running" || tournament.Phase == "scheduled" || tournament.Phase == "finished"
	primary := "Квалы до " + formatDateTime(tournament.QualificationEndsAt)
	switch tournament.Phase {
	case "qualification":
		primary = "Квалификация до " + formatDateTime(tournament.QualificationEndsAt)
	case "playoff":
		primary = "Плей-офф до " + formatDateTime(tournament.PlayoffEndsAt)
	case "final":
		primary = "Финал до " + formatDateTime(tournament.EndsAt)
	case "scheduled":
		primary = "Старт " + formatDateTime(tournament.StartsAt)
	case "finished":
		primary = "Турнир завершен " + formatDateTime(tournament.EndsAt)
	}
	bracketOpen := !now.Before(tournament.QualificationEndsAt)
	countdownValue, countdownLabel, countdownDetail := qualificationCountdown(tournament, now)
	showCountdown := tournament.State == "running" && tournament.Phase == "qualification" && now.Before(tournament.QualificationEndsAt)
	return tournamentStatusView{
		Available:           true,
		RegistrationOpen:    registrationOpen,
		Phase:               tournament.Phase,
		PhaseLabel:          phaseLabel,
		StateLabel:          tournament.State,
		StartsAt:            tournament.StartsAt,
		QualificationEndsAt: tournament.QualificationEndsAt,
		PlayoffEndsAt:       tournament.PlayoffEndsAt,
		EndsAt:              tournament.EndsAt,
		PlayoffSlots:        tournament.PlayoffSlots,
		BracketReady:        tournament.BracketGeneratedAt != nil,
		BracketOpen:         bracketOpen,
		CountdownValue:      countdownValue,
		CountdownLabel:      countdownLabel,
		CountdownDetail:     countdownDetail,
		ShowCountdown:       showCountdown,
		PrimaryLine:         primary,
		SecondaryLine:       "В плей-офф проходит топ-" + intString(tournament.PlayoffSlots) + ". Формат: квалы, плей-офф, финал.",
	}
}

func qualificationCountdown(tournament *store.Tournament, now time.Time) (string, string, string) {
	if tournament == nil {
		return "-", "дней", "Квалификация начнется после старта турнира."
	}
	if !now.Before(tournament.QualificationEndsAt) {
		return "0", "дней", "Квалификация завершена."
	}
	days := ceilDays(tournament.QualificationEndsAt.Sub(now))
	detail := "До конца квалификации."
	if days == 0 {
		detail = "До конца квалификации меньше суток."
	}
	return intString(days), dayLabel(days), detail
}

func ceilDays(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	day := 24 * time.Hour
	return int((duration + day - time.Nanosecond) / day)
}

func dayLabel(days int) string {
	remainder := days % 100
	if remainder >= 11 && remainder <= 14 {
		return "дней"
	}
	switch days % 10 {
	case 1:
		return "день"
	case 2, 3, 4:
		return "дня"
	default:
		return "дней"
	}
}

func buildBracketRounds(matches []store.TournamentMatch) []bracketRoundView {
	if len(matches) == 0 {
		return nil
	}
	roundOrder := map[string]int{
		"quarterfinal": 1,
		"semifinal":    2,
		"third_place":  3,
		"final":        4,
	}
	grouped := map[string][]bracketMatchView{}
	for _, match := range matches {
		grouped[match.Round] = append(grouped[match.Round], buildBracketMatchView(match))
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return roundOrder[keys[i]] < roundOrder[keys[j]]
	})

	rounds := make([]bracketRoundView, 0, len(keys))
	for _, key := range keys {
		matches := grouped[key]
		sort.Slice(matches, func(i, j int) bool { return matches[i].Position < matches[j].Position })
		rounds = append(rounds, bracketRoundView{
			Key:     key,
			Label:   roundLabel(key),
			Matches: matches,
		})
	}
	return rounds
}

func buildBracketMatchView(match store.TournamentMatch) bracketMatchView {
	return bracketMatchView{
		ID:              match.ID,
		Round:           match.Round,
		RoundLabel:      roundLabel(match.Round),
		Position:        match.Position,
		Status:          match.Status,
		StatusLabel:     matchStatusLabel(match.Status),
		StartsAt:        match.StartsAt,
		EndsAt:          match.EndsAt,
		BestOf:          match.BestOf,
		Player1Name:     participantName(match.Player1),
		Player2Name:     participantName(match.Player2),
		Player1ID:       participantID(match.Player1),
		Player2ID:       participantID(match.Player2),
		Player1Seed:     seedLabel(match.Player1Seed),
		Player2Seed:     seedLabel(match.Player2Seed),
		Player1BestTime: nullableDuration(match.Player1BestTimeMS),
		Player2BestTime: nullableDuration(match.Player2BestTimeMS),
		WinnerName:      match.WinnerDisplayName,
	}
}

func participantID(participant *store.Participant) string {
	if participant == nil {
		return ""
	}
	return participant.ID
}

func participantName(participant *store.Participant) string {
	if participant == nil {
		return "Ожидается победитель"
	}
	return firstNonEmpty(participant.TwitchDisplayName, participant.TwitchLogin)
}

func seedLabel(seed *int) string {
	if seed == nil {
		return ""
	}
	return "#" + intString(*seed)
}

func nullableDuration(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func phaseLabel(phase string) string {
	switch phase {
	case "qualification":
		return "Квалификация"
	case "playoff":
		return "Плей-офф"
	case "final":
		return "Финал"
	case "scheduled":
		return "Запланирован"
	case "finished":
		return "Завершен"
	default:
		return phase
	}
}

func roundLabel(round string) string {
	switch round {
	case "quarterfinal":
		return "1/4 финала"
	case "semifinal":
		return "Полуфинал"
	case "third_place":
		return "Матч за 3 место"
	case "final":
		return "Финал"
	default:
		return round
	}
}

func matchStatusLabel(status string) string {
	switch status {
	case "running":
		return "идет"
	case "finished":
		return "завершен"
	default:
		return "запланирован"
	}
}

func intString(value int) string {
	return strconv.Itoa(value)
}
