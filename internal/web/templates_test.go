package web

import (
	"html/template"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	_, err := template.New("").
		Funcs(template.FuncMap{
			"formatDuration":      formatDuration,
			"formatDateTime":      formatDateTime,
			"formatShortDateTime": formatShortDateTime,
			"add":                 func(a, b int) int { return a + b },
		}).
		ParseFS(assets, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
}
