package web

import (
	"fmt"

	"github.com/dimension-science/tournament-kit/internal/store"
)

type LeaderboardRowView struct {
	Rank            int
	DisplayName     string
	TwitchLogin     string
	MinecraftNick   string
	AvatarURL       string
	BestTimeMS      int
	Category        string
	Version         string
	DateLabel       string
	RankDeltaLabel  string
	RankDeltaClass  string
	RankMotionClass string
	DetailID        string
	HasSplitDetails bool
	Milestones      []LeaderboardMilestoneView
	MissingReason   string
	AfterCut        bool
}

type LeaderboardMilestoneView struct {
	Label           string
	SectorMS        *int
	CumulativeMS    *int
	ComparisonLabel string
	ComparisonClass string
	ComparisonIcon  string
}

type NowPlayingRowView struct {
	DisplayName string
	TwitchLogin string
	TwitchURL   string
	AvatarURL   string
	StreamTitle string
	Category    string
	StatusLabel string
}

func buildLeaderboardRows(entries []store.LeaderboardEntry, playoffCutoff int) []LeaderboardRowView {
	if len(entries) == 0 {
		return nil
	}

	leader := entries[0]
	rows := make([]LeaderboardRowView, 0, len(entries))
	for index, entry := range entries {
		row := LeaderboardRowView{
			Rank:            entry.Rank,
			DisplayName:     firstNonEmpty(entry.TwitchDisplayName, entry.TwitchLogin),
			TwitchLogin:     entry.TwitchLogin,
			MinecraftNick:   entry.MinecraftNick,
			AvatarURL:       entry.AvatarURL,
			BestTimeMS:      entry.BestTimeMS,
			Category:        "Any% Glitchless",
			Version:         "1.21.11",
			DateLabel:       formatDate(entry.BestRunFinishedAt),
			RankDeltaLabel:  rankDeltaLabel(entry.RankDelta),
			RankDeltaClass:  rankDeltaClass(entry.RankDelta),
			RankMotionClass: rankMotionClass(entry.RankDelta),
			DetailID:        fmt.Sprintf("leaderboard-detail-%d", index+1),
			AfterCut:        entry.Rank == playoffCutoff,
		}

		hasSplitDetails := entry.NetherSplitMS != nil || entry.EndSplitMS != nil
		row.HasSplitDetails = hasSplitDetails
		if !hasSplitDetails {
			row.MissingReason = "РЎРїР»РёС‚С‹ РґР»СЏ СЌС‚РѕРіРѕ Р»СѓС‡С€РµРіРѕ СЂРµР·СѓР»СЊС‚Р°С‚Р° РµС‰Рµ РЅРµ Р·Р°РїРёСЃР°РЅС‹."
			rows = append(rows, row)
			continue
		}

		isLeader := index == 0
		netherComparison := comparisonView(entry.NetherSplitMS, leader.NetherSplitMS, isLeader)
		endComparison := comparisonView(entry.EndSplitMS, leader.EndSplitMS, isLeader)
		finishComparison := comparisonView(intPointer(entry.BestTimeMS), intPointer(leader.BestTimeMS), isLeader)
		row.Milestones = []LeaderboardMilestoneView{
			{
				Label:           "РћРІРµСЂ -> РќРµР·РµСЂ",
				SectorMS:        cloneInt(entry.NetherSplitMS),
				CumulativeMS:    cloneInt(entry.NetherSplitMS),
				ComparisonLabel: netherComparison.label,
				ComparisonClass: netherComparison.class,
				ComparisonIcon:  netherComparison.icon,
			},
			{
				Label:           "РќРµР·РµСЂ -> Р­РЅРґ",
				SectorMS:        segmentBetween(entry.NetherSplitMS, entry.EndSplitMS),
				CumulativeMS:    cloneInt(entry.EndSplitMS),
				ComparisonLabel: endComparison.label,
				ComparisonClass: endComparison.class,
				ComparisonIcon:  endComparison.icon,
			},
			{
				Label:           "РЈР±РёР№СЃС‚РІРѕ РґСЂР°РєРѕРЅР°",
				SectorMS:        segmentToFinish(entry.EndSplitMS, entry.BestTimeMS),
				CumulativeMS:    intPointer(entry.BestTimeMS),
				ComparisonLabel: finishComparison.label,
				ComparisonClass: finishComparison.class,
				ComparisonIcon:  finishComparison.icon,
			},
		}
		rows = append(rows, row)
	}

	return rows
}

type splitComparison struct {
	label string
	class string
	icon  string
}

func comparisonView(current, leader *int, isLeader bool) splitComparison {
	if current == nil {
		return splitComparison{label: "РЅРµС‚ РґР°РЅРЅС‹С… РґР»СЏ СЃСЂР°РІРЅРµРЅРёСЏ", class: "is-muted"}
	}
	if isLeader {
		return splitComparison{label: "СЌС‚Р°Р»РѕРЅ РїРµСЂРІРѕРіРѕ РјРµСЃС‚Р°", class: "is-leader"}
	}
	if leader == nil {
		return splitComparison{label: "Сѓ РїРµСЂРІРѕРіРѕ РјРµСЃС‚Р° РЅРµС‚ РґР°РЅРЅС‹С… РґР»СЏ СЃСЂР°РІРЅРµРЅРёСЏ", class: "is-muted"}
	}

	delta := *current - *leader
	switch {
	case delta == 0:
		return splitComparison{label: "РІСЂРѕРІРµРЅСЊ СЃ РїРµСЂРІС‹Рј РјРµСЃС‚РѕРј", class: "is-even", icon: "="}
	case delta < 0:
		return splitComparison{label: "Р±С‹СЃС‚СЂРµРµ РЅР° " + formatDuration(-delta), class: "is-better", icon: "в†‘"}
	default:
		return splitComparison{label: "РјРµРґР»РµРЅРЅРµРµ РЅР° " + formatDuration(delta), class: "is-worse", icon: "в†“"}
	}
}

func rankDeltaLabel(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("+%d", delta)
	case delta < 0:
		return fmt.Sprintf("%d", delta)
	default:
		return ""
	}
}

func rankDeltaClass(delta int) string {
	switch {
	case delta > 0:
		return "is-up"
	case delta < 0:
		return "is-down"
	default:
		return ""
	}
}

func rankMotionClass(delta int) string {
	if delta > 0 {
		return "is-rank-up"
	}
	return ""
}

func segmentBetween(start, end *int) *int {
	if start == nil || end == nil {
		return nil
	}
	value := *end - *start
	if value <= 0 {
		return nil
	}
	return intPointer(value)
}

func segmentToFinish(start *int, total int) *int {
	if total <= 0 {
		return nil
	}
	if start == nil {
		return intPointer(total)
	}
	value := total - *start
	if value <= 0 {
		return nil
	}
	return intPointer(value)
}

func intPointer(value int) *int {
	copy := value
	return &copy
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	return intPointer(*value)
}

func buildNowPlayingRows(participants []store.Participant) []NowPlayingRowView {
	if len(participants) == 0 {
		return nil
	}
	rows := make([]NowPlayingRowView, 0, len(participants))
	for _, participant := range participants {
		avatarURL := participant.AvatarURL
		if avatarURL == "" {
			avatarURL = participant.TwitchProfileImageURL
		}
		if avatarURL == "" {
			avatarURL = "/static/avatar-placeholder.svg"
		}
		rows = append(rows, NowPlayingRowView{
			DisplayName: firstNonEmpty(participant.TwitchDisplayName, participant.TwitchLogin),
			TwitchLogin: participant.TwitchLogin,
			TwitchURL:   "https://www.twitch.tv/" + participant.TwitchLogin,
			AvatarURL:   avatarURL,
			StreamTitle: firstNonEmpty(participant.StreamTitle, "Minecraft run"),
			Category:    firstNonEmpty(participant.StreamGameName, "Minecraft"),
			StatusLabel: "LIVE",
		})
	}
	return rows
}
