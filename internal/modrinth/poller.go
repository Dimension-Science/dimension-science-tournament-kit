package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dimension-science/tournament-kit/internal/discordbot"
	"github.com/dimension-science/tournament-kit/internal/store"
)

type Version struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	VersionNumber string    `json:"version_number"`
	Changelog     string    `json:"changelog"`
	DatePublished time.Time `json:"date_published"`
	VersionType   string    `json:"version_type"`
}

type Poller struct {
	db         *store.Store
	discord    *discordbot.Client
	httpClient *http.Client
	logger     *log.Logger
	projectIDs []string
	apiBase    string
}

func NewPoller(db *store.Store, discord *discordbot.Client, logger *log.Logger) *Poller {
	if logger == nil {
		logger = log.Default()
	}
	return &Poller{
		db:         db,
		discord:    discord,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
		projectIDs: []string{"lHBv9YNe", "XD2aAKNK"},
		apiBase:    "https://api.modrinth.com",
	}
}

func (p *Poller) Start(ctx context.Context) {
	if p.db == nil || p.discord == nil {
		p.logger.Println("Modrinth poller disabled: missing db or discord client")
		return
	}
	p.logger.Println("Starting Modrinth release poller background worker...")

	go func() {
		// Run first poll shortly after start
		time.Sleep(10 * time.Second)
		p.pollOnce(ctx)

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pollOnce(ctx)
			}
		}
	}()
}

var projectNames = map[string]string{
	"lHBv9YNe": "Tournament Timer",
	"XD2aAKNK": "Tournament entities fix",
}

var projectSlugs = map[string]string{
	"lHBv9YNe": "tournament-timer",
	"XD2aAKNK": "tournament-entities-fix",
}

func (p *Poller) pollOnce(ctx context.Context) {
	for _, projID := range p.projectIDs {
		versions, err := p.fetchVersions(ctx, projID)
		if err != nil {
			p.logger.Printf("Modrinth poller: error fetching versions for %s: %v", projID, err)
			continue
		}

		// Process versions from oldest to newest to post in chronological order
		for i := len(versions) - 1; i >= 0; i-- {
			v := versions[i]
			seen, err := p.db.HasSeenModrinthVersion(ctx, v.ID)
			if err != nil {
				p.logger.Printf("Modrinth poller: db error checking version %s: %v", v.ID, err)
				continue
			}
			if seen {
				continue
			}

			// Save to database first to prevent double-posting
			err = p.db.SaveSeenModrinthVersion(ctx, v.ID, v.ProjectID, v.VersionNumber, v.DatePublished)
			if err != nil {
				p.logger.Printf("Modrinth poller: db error saving version %s: %v", v.ID, err)
				continue
			}

			p.logger.Printf("Modrinth poller: detected new version %s (%s) for project %s", v.VersionNumber, v.ID, projID)

			// Send notification to Discord only if it was published recently (e.g. within 24 hours)
			if time.Since(v.DatePublished) < 24*time.Hour {
				p.notifyDiscord(ctx, v)
			} else {
				p.logger.Printf("Modrinth poller: version %s was published in the past (%v), skipping Discord notification", v.VersionNumber, v.DatePublished)
			}
		}
	}
}

func (p *Poller) fetchVersions(ctx context.Context, projectID string) ([]Version, error) {
	url := fmt.Sprintf("%s/v2/project/%s/version", p.apiBase, projectID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "dimension-science/tournament-kit (tournament@leaderboard)")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var versions []Version
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (p *Poller) notifyDiscord(ctx context.Context, v Version) {
	projName := projectNames[v.ProjectID]
	if projName == "" {
		projName = v.ProjectID
	}
	projSlug := projectSlugs[v.ProjectID]
	if projSlug == "" {
		projSlug = v.ProjectID
	}

	modrinthLink := fmt.Sprintf("https://modrinth.com/mod/%s", projSlug)
	versionTypeDisplay := strings.ToUpper(v.VersionType)

	// Clean up and format changelog
	changelog := strings.TrimSpace(v.Changelog)
	if changelog == "" {
		changelog = "*РћРїРёСЃР°РЅРёРµ РёР·РјРµРЅРµРЅРёР№ РѕС‚СЃСѓС‚СЃС‚РІСѓРµС‚.*"
	}

	header := fmt.Sprintf("**РџСЂРѕРµРєС‚:** [%s](%s)\n**Р’РµСЂСЃРёСЏ:** `%s` (%s)\n\n", projName, modrinthLink, v.VersionNumber, versionTypeDisplay)
	footer := "\n**РЎРїРёСЃРѕРє РёР·РјРµРЅРµРЅРёР№:**\n"

	// Embed description length check (Discord limits description to 4096, but we keep text safe)
	maxChangelogLen := 1800 - len(header) - len(footer)
	if maxChangelogLen < 100 {
		maxChangelogLen = 100
	}
	if len(changelog) > maxChangelogLen {
		changelog = changelog[:maxChangelogLen-3] + "..."
	}

	message := header + footer + changelog
	embedTitle := fmt.Sprintf("Р’С‹С€Р»Рѕ РѕР±РЅРѕРІР»РµРЅРёРµ РјРѕРґР° %s!", projName)

	p.discord.NotifyModUpdate(ctx, discordbot.NewsPost{
		Title:   embedTitle,
		Text:    message,
		Mention: v.VersionType == "release", // Mention runners for Release versions
	})
}
