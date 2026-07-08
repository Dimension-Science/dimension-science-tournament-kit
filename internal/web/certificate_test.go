package web

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/store"
)

func TestGenerateCertificateImage(t *testing.T) {
	templateDir := t.TempDir()
	for _, name := range []string{"member.png", "playoff.png", "finalist.png", "winner.png"} {
		file, err := os.Create(filepath.Join(templateDir, name))
		if err != nil {
			t.Fatalf("create template %s: %v", name, err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 2016, 2688))
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				img.Set(x, y, color.RGBA{R: 245, G: 238, B: 220, A: 255})
			}
		}
		if err := png.Encode(file, img); err != nil {
			_ = file.Close()
			t.Fatalf("encode template %s: %v", name, err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close template %s: %v", name, err)
		}
	}

	srv := &Server{cfg: config.Config{CertificateTemplateDir: templateDir}}
	bestTime := 1425000
	p := &store.Participant{
		ID:                "p_test_123",
		TwitchLogin:       "runner",
		TwitchDisplayName: "Runner",
		MinecraftNick:     "Runner_MC",
		BestTimeMS:        &bestTime,
	}

	statuses := []string{"ПОБЕДИТЕЛЬ", "ФИНАЛИСТ", "ИГРОК ПЛЕЙ-ОФФ", "КВАЛИФИКАНТ"}
	for _, status := range statuses {
		img, err := srv.generateCertificateImage(context.Background(), p, status, "CERT-2026-123456")
		if err != nil {
			t.Fatalf("generate certificate for status %s: %v", status, err)
		}
		if img == nil {
			t.Fatalf("generated certificate image is nil for status %s", status)
		}
		bounds := img.Bounds()
		if bounds.Dx() != 2016 || bounds.Dy() != 2688 {
			t.Errorf("unexpected certificate image size: got %dx%d, want 2016x2688", bounds.Dx(), bounds.Dy())
		}
	}
}
