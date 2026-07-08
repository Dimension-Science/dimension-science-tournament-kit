package web

import (
	"context"
	"html"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dimension-science/tournament-kit/internal/store"
)

type NewsPostView struct {
	Text        string
	Title       string
	BodyHTML    template.HTML
	ImageURL    string
	SourceURL   string
	PublishedAt time.Time
	DateText    string
	TimeText    string
}

var newsURLPattern = regexp.MustCompile(`https?://[^\s<]+`)

func (s *Server) handleNewsPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	posts, err := s.store.ListTelegramNewsPosts(ctx, 50)
	if err != nil {
		http.Error(w, "failed to load news", http.StatusInternalServerError)
		return
	}

	_ = s.renderTemplate(w, "news.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Posts":           buildNewsPostViews(posts),
	})
}

func buildNewsPostViews(posts []store.TelegramNewsPost) []NewsPostView {
	if len(posts) == 0 {
		return nil
	}
	views := make([]NewsPostView, 0, len(posts))
	for _, post := range posts {
		title, body := splitNewsText(post.Text)
		views = append(views, NewsPostView{
			Text:        post.Text,
			Title:       title,
			BodyHTML:    linkifyNewsText(body),
			ImageURL:    post.ImageURL,
			SourceURL:   post.SourceURL,
			PublishedAt: post.PublishedAt,
			DateText:    formatNewsDate(post.PublishedAt),
			TimeText:    post.PublishedAt.Local().Format("15:04"),
		})
	}
	return views
}

func splitNewsText(text string) (string, string) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return "РќРѕРІРѕСЃС‚СЊ Tournament", ""
	}
	paragraphs := strings.Split(text, "\n\n")
	clean := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph != "" {
			clean = append(clean, paragraph)
		}
	}
	if len(clean) == 0 {
		return "РќРѕРІРѕСЃС‚СЊ Tournament", ""
	}
	if len(clean) == 1 {
		lines := strings.Split(clean[0], "\n")
		title := strings.TrimSpace(lines[0])
		if len(lines) == 1 {
			return title, ""
		}
		return title, strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return clean[0], strings.Join(clean[1:], "\n\n")
}

func linkifyNewsText(text string) template.HTML {
	escaped := html.EscapeString(strings.TrimSpace(text))
	if escaped == "" {
		return ""
	}
	linked := newsURLPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		href := match
		trailing := ""
		for strings.ContainsAny(href[len(href)-1:], ".,);:!?") {
			trailing = href[len(href)-1:] + trailing
			href = href[:len(href)-1]
			if href == "" {
				return match
			}
		}
		return `<a href="` + href + `" target="_blank" rel="noopener noreferrer">` + href + `</a>` + trailing
	})
	linked = strings.ReplaceAll(linked, "\n", "<br>")
	return template.HTML(linked)
}

func formatNewsDate(value time.Time) string {
	months := [...]string{
		"СЏРЅРІР°СЂСЏ",
		"С„РµРІСЂР°Р»СЏ",
		"РјР°СЂС‚Р°",
		"Р°РїСЂРµР»СЏ",
		"РјР°СЏ",
		"РёСЋРЅСЏ",
		"РёСЋР»СЏ",
		"Р°РІРіСѓСЃС‚Р°",
		"СЃРµРЅС‚СЏР±СЂСЏ",
		"РѕРєС‚СЏР±СЂСЏ",
		"РЅРѕСЏР±СЂСЏ",
		"РґРµРєР°Р±СЂСЏ",
	}
	local := value.Local()
	return local.Format("2") + " " + months[int(local.Month())-1] + " " + local.Format("2006")
}
