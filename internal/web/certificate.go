package web

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dimension-science/tournament-kit/internal/store"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type roundedRectMask struct {
	r      image.Rectangle
	radius int
}

func (m roundedRectMask) ColorModel() color.Model {
	return color.AlphaModel
}

func (m roundedRectMask) Bounds() image.Rectangle {
	return m.r
}

func (m roundedRectMask) At(x, y int) color.Color {
	x0, y0 := m.r.Min.X, m.r.Min.Y
	x1, y1 := m.r.Max.X, m.r.Max.Y
	r := m.radius

	if x < x0+r && y < y0+r {
		dx, dy := x-(x0+r), y-(y0+r)
		if dx*dx+dy*dy > r*r {
			return color.Alpha{0}
		}
	}
	if x >= x1-r && y < y0+r {
		dx, dy := x-(x1-r), y-(y0+r)
		if dx*dx+dy*dy > r*r {
			return color.Alpha{0}
		}
	}
	if x < x0+r && y >= y1-r {
		dx, dy := x-(x0+r), y-(y1-r)
		if dx*dx+dy*dy > r*r {
			return color.Alpha{0}
		}
	}
	if x >= x1-r && y >= y1-r {
		dx, dy := x-(x1-r), y-(y1-r)
		if dx*dx+dy*dy > r*r {
			return color.Alpha{0}
		}
	}
	return color.Alpha{255}
}

func loadFontFace(size float64) (font.Face, error) {
	var fontPaths = []string{
		"data/assets/font.ttf",
		"internal/web/static/assets/font.ttf",
		"C:\\Windows\\Fonts\\segoeuib.ttf", // Windows Segoe UI Bold
		"C:\\Windows\\Fonts\\segoeui.ttf",  // Windows Segoe UI
		"C:\\Windows\\Fonts\\arialbd.ttf",  // Windows Arial Bold
		"C:\\Windows\\Fonts\\arial.ttf",    // Windows Arial
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
	}

	var fontBytes []byte
	var readErr error
	for _, p := range fontPaths {
		fontBytes, readErr = os.ReadFile(p)
		if readErr == nil {
			break
		}
	}

	if len(fontBytes) == 0 {
		return nil, fmt.Errorf("failed to read any font file: last err: %w", readErr)
	}

	f, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font: %w", err)
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create font face: %w", err)
	}

	return face, nil
}

func drawCenteredText(dst draw.Image, face font.Face, text string, y int, c color.Color, canvasWidth int) {
	bounds, _ := font.BoundString(face, text)
	width := (bounds.Max.X - bounds.Min.X).Ceil()
	x := (canvasWidth - width) / 2
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(x << 6),
			Y: fixed.Int26_6(y << 6),
		},
	}
	d.DrawString(text)
}

func drawLeftText(dst draw.Image, face font.Face, text string, x, y int, c color.Color) {
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(x << 6),
			Y: fixed.Int26_6(y << 6),
		},
	}
	d.DrawString(text)
}

func drawRightText(dst draw.Image, face font.Face, text string, rightX, y int, c color.Color) {
	bounds, _ := font.BoundString(face, text)
	width := (bounds.Max.X - bounds.Min.X).Ceil()
	x := rightX - width
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(x << 6),
			Y: fixed.Int26_6(y << 6),
		},
	}
	d.DrawString(text)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func drawLeftTextTopAlign(dst draw.Image, face font.Face, text string, x, topY int, c color.Color) {
	metrics := face.Metrics()
	ascent := metrics.Ascent.Floor()
	baselineY := topY + ascent
	drawLeftText(dst, face, text, x, baselineY, c)
}

func fetchAvatarImage(url string) (image.Image, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar fetch returned HTTP status %d", resp.StatusCode)
	}
	img, _, err := image.Decode(resp.Body)
	return img, err
}

func (s *Server) handleDownloadCertificate(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}

	participant, err := s.store.FindParticipantByTwitchUserID(r.Context(), claims.TwitchUserID)
	if err != nil || participant == nil {
		http.Error(w, "Participant not found", http.StatusNotFound)
		return
	}

	certStatus, certNo, available, err := s.store.GetParticipantCertificateStatus(r.Context(), participant.ID)
	if err != nil {
		http.Error(w, "Failed to get certificate status", http.StatusInternalServerError)
		return
	}
	if !available {
		http.Error(w, "Certificate is not unlocked yet for your stage progress.", http.StatusForbidden)
		return
	}

	img, err := s.generateCertificateImage(r.Context(), participant, certStatus, certNo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render certificate: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"certificate_%s.png\"", participant.TwitchLogin))
	_ = png.Encode(w, img)
}

func (s *Server) generateCertificateImage(ctx context.Context, p *store.Participant, certStatus, certNo string) (image.Image, error) {
	bgFilename := "member.png"
	legacyFallback := "certificate_participant_work-export_member.png"
	switch certStatus {
	case "ПОБЕДИТЕЛЬ":
		bgFilename = "winner.png"
		legacyFallback = ""
	case "ФИНАЛИСТ":
		bgFilename = "finalist.png"
		legacyFallback = ""
	case "ИГРОК ПЛЕЙ-ОФФ":
		bgFilename = "playoff.png"
		legacyFallback = "play offer.png"
	default:
		bgFilename = "member.png"
	}

	bgPath := filepath.Join(s.cfg.CertificateTemplateDir, bgFilename)
	bgFile, err := os.Open(bgPath)
	if err != nil && legacyFallback != "" {
		bgPath = filepath.Join(s.cfg.CertificateTemplateDir, legacyFallback)
		bgFile, err = os.Open(bgPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open certificate background image %s in %s: %w", bgFilename, s.cfg.CertificateTemplateDir, err)
	}
	defer bgFile.Close()

	bgImg, _, err := image.Decode(bgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode background image: %w", err)
	}

	bounds := bgImg.Bounds()
	w := bounds.Max.X - bounds.Min.X

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, bgImg, image.Point{}, draw.Src)

	// 2. Avatar loading and drawing (X 260 Y 660)
	const avatarX = 260
	const avatarY = 660
	const avatarW = 480
	const avatarH = 480

	avatarDrawn := false
	avatarURL := p.AvatarURL
	if avatarURL == "" {
		avatarURL = p.TwitchProfileImageURL
	}

	if avatarURL != "" {
		if avImg, err := fetchAvatarImage(avatarURL); err == nil {
			scaledAvatar := image.NewRGBA(image.Rect(0, 0, avatarW, avatarH))
			xdraw.BiLinear.Scale(scaledAvatar, scaledAvatar.Bounds(), avImg, avImg.Bounds(), draw.Over, nil)
			avatarRect := image.Rect(avatarX, avatarY, avatarX+avatarW, avatarY+avatarH)
			draw.DrawMask(dst, avatarRect, scaledAvatar, image.Point{}, roundedRectMask{r: image.Rect(0, 0, avatarW, avatarH), radius: 48}, image.Point{}, draw.Over)
			avatarDrawn = true
		}
	}

	if !avatarDrawn {
		// Draw a solid rounded rect placeholder
		placeholder := image.NewUniform(color.RGBA{135, 105, 75, 255})
		avatarRect := image.Rect(avatarX, avatarY, avatarX+avatarW, avatarY+avatarH)
		draw.DrawMask(dst, avatarRect, placeholder, image.Point{}, roundedRectMask{r: image.Rect(0, 0, avatarW, avatarH), radius: 48}, image.Point{}, draw.Over)
	}

	// 3. Load Fonts
	faceName, err := loadFontFace(46)
	if err != nil {
		return nil, fmt.Errorf("failed to load font face: %w", err)
	}
	defer faceName.Close()

	faceTime, _ := loadFontFace(100)
	defer faceTime.Close()

	faceLabel, _ := loadFontFace(32)
	defer faceLabel.Close()

	faceDate, _ := loadFontFace(44)
	defer faceDate.Close()

	// 4. Render text content
	textColorDark := color.RGBA{54, 32, 10, 255} // С‚РµРјРЅРѕ-РєРѕСЂРёС‡РЅРµРІС‹Р№ РґР»СЏ Р±СѓРјР°РіРё

	// РРјСЏ: X 1041 Y 693
	displayName := p.TwitchDisplayName
	if displayName == "" || !isASCII(displayName) {
		displayName = p.TwitchLogin
	}
	drawLeftTextTopAlign(dst, faceName, displayName, 1041, 693, textColorDark)

	// РќРёРє: X 1041 Y 855
	nickName := "@" + p.TwitchLogin
	if p.MinecraftNick != "" {
		nickName = p.MinecraftNick
	}
	drawLeftTextTopAlign(dst, faceName, nickName, 1041, 855, textColorDark)

	// РџРѕРґРїРёСЃСЊ: X 1220 Y 1005 (Р·Р°РіСЂСѓР·РєР° Рё РѕС‚СЂРёСЃРѕРІРєР° РєР°СЂС‚РёРЅРєРё, РїРѕР±РѕР»СЊС€Рµ)
	if f, err := assets.Open("static/signature.png"); err == nil {
		if sigImg, _, err := image.Decode(f); err == nil {
			const sigH = 80
			const sigW = 117
			scaledSig := image.NewRGBA(image.Rect(0, 0, sigW, sigH))
			xdraw.BiLinear.Scale(scaledSig, scaledSig.Bounds(), sigImg, sigImg.Bounds(), draw.Over, nil)
			sigRect := image.Rect(1220, 1005, 1220+sigW, 1005+sigH)
			draw.Draw(dst, sigRect, scaledSig, image.Point{}, draw.Over)
		}
		f.Close()
	}

	// 4.1 Filter unlocked achievements
	var unlockedSlugs []string
	if s.store != nil {
		if achievements, err := s.store.ListAchievementProgress(ctx, p.ID); err == nil {
			for _, ach := range achievements {
				if ach.Unlocked {
					unlockedSlugs = append(unlockedSlugs, ach.Slug)
				}
			}
		}
	}

	// Р›СѓС‡С€РµРµ РІСЂРµРјСЏ: X 381 Y 1267 (С†РµРЅС‚СЂРёСЂСѓРµРј РїРѕ РіРѕСЂРёР·РѕРЅС‚Р°Р»Рё Рё РІРµСЂС‚РёРєР°Р»Рё РІ СЂР°РјРєРµ)
	recordStr := "-"
	if p.BestTimeMS != nil {
		recordStr = formatDuration(*p.BestTimeMS)
	}

	timeY := 1553
	if len(unlockedSlugs) > 0 {
		timeY = 1400
	}

	metricsTime := faceTime.Metrics()
	ascentTime := metricsTime.Ascent.Floor()
	descentTime := metricsTime.Descent.Floor()
	baselineTime := timeY + (ascentTime-descentTime)/2
	drawCenteredText(dst, faceTime, recordStr, baselineTime, textColorDark, w)

	// Draw achievement badges under best time if unlocked
	if len(unlockedSlugs) > 0 {
		const badgeSize = 144
		const spacing = 32
		n := len(unlockedSlugs)
		rowWidth := n*badgeSize + (n-1)*spacing
		startX := (w - rowWidth) / 2
		badgeY := 1580

		for i, slug := range unlockedSlugs {
			badgeX := startX + i*(badgeSize+spacing)
			badgePath := fmt.Sprintf("static/achievements/%s.png", slug)
			if f, err := assets.Open(badgePath); err == nil {
				if badgeImg, _, err := image.Decode(f); err == nil {
					scaledBadge := image.NewRGBA(image.Rect(0, 0, badgeSize, badgeSize))
					xdraw.BiLinear.Scale(scaledBadge, scaledBadge.Bounds(), badgeImg, badgeImg.Bounds(), draw.Over, nil)
					badgeRect := image.Rect(badgeX, badgeY, badgeX+badgeSize, badgeY+badgeSize)
					draw.Draw(dst, badgeRect, scaledBadge, image.Point{}, draw.Over)
				}
				f.Close()
			}
		}
	}

	// Р”Р°С‚Р° РІС‹РґР°С‡Рё: X 350 Y 2140 (Р±С‹Р»Рѕ X 320 Y 2145)
	dateStr := time.Now().Format("02.01.2006")
	drawLeftTextTopAlign(dst, faceDate, dateStr, 350, 2140, textColorDark)

	// РќРѕРјРµСЂ СЃРµСЂС‚РёС„РёРєР°С‚Р°: X 1340 Y 2145 (Р±С‹Р»Рѕ X 1305 Y 2130)
	drawLeftTextTopAlign(dst, faceLabel, certNo, 1340, 2145, textColorDark)

	return dst, nil
}
