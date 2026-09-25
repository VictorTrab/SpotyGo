package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/logger"
)

const (
	CoverWidthChars     = 10
	CoverHeightChars    = 5
	BigCoverWidthChars  = 30
	BigCoverHeightChars = 15
)

type CoverData struct {
	Image         image.Image
	Lines         []string
	BigLines      []string
	DominantColor color.RGBA
	DominantHex   string
	SecondaryHex  string
}

const MaxMemoryCovers = 25

type CoverManager struct {
	mu        sync.RWMutex
	memory    map[string]CoverData
	order     []string
	diskCache string
	client    *http.Client
}

var (
	defaultCoverMgr *CoverManager
	coverMgrOnce    sync.Once
)

func GetCoverManager() *CoverManager {
	coverMgrOnce.Do(func() {
		dir, err := os.UserConfigDir()
		cacheDir := filepath.Join(os.TempDir(), "SpotifyGo", "cache", "covers")
		if err == nil {
			cacheDir = filepath.Join(dir, "SpotyGo", "cache", "covers")
		}
		_ = os.MkdirAll(cacheDir, 0755)

		defaultCoverMgr = &CoverManager{
			memory:    make(map[string]CoverData),
			order:     make([]string, 0, MaxMemoryCovers+1),
			diskCache: cacheDir,
			client: &http.Client{
				Timeout: 4 * time.Second,
			},
		}
	})
	return defaultCoverMgr
}

type coverLoadedMsg struct {
	url  string
	data CoverData
}

func (cm *CoverManager) FetchCover(url string) tea.Cmd {
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		if data, ok := cm.getMemory(url); ok {
			return coverLoadedMsg{url: url, data: data}
		}

		data, err := cm.loadOrDownload(url)
		if err != nil {
			logger.Warn("No se pudo cargar carátula para %s: %v", url, err)
			return nil
		}

		cm.putMemory(url, data)
		return coverLoadedMsg{url: url, data: data}
	}
}

func (cm *CoverManager) getMemory(url string) (CoverData, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	data, ok := cm.memory[url]
	return data, ok
}

func (cm *CoverManager) putMemory(url string, data CoverData) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, exists := cm.memory[url]; !exists {
		cm.order = append(cm.order, url)
		if len(cm.order) > MaxMemoryCovers {
			evictURL := cm.order[0]
			cm.order = cm.order[1:]
			delete(cm.memory, evictURL)
		}
	}
	cm.memory[url] = data
}

func (cm *CoverManager) loadOrDownload(url string) (CoverData, error) {
	hash := sha256.Sum256([]byte(url))
	cacheKey := hex.EncodeToString(hash[:16])
	filePath := filepath.Join(cm.diskCache, cacheKey+".jpg")

	// Check disk cache first
	if file, err := os.Open(filePath); err == nil {
		defer file.Close()
		img, _, err := image.Decode(file)
		if err == nil {
			return processCoverImage(img, CoverWidthChars, CoverHeightChars), nil
		}
	}

	// Download from Spotify CDN
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return CoverData{}, err
	}
	resp, err := cm.client.Do(req)
	if err != nil {
		return CoverData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CoverData{}, fmt.Errorf("HTTP %d al descargar carátula", resp.StatusCode)
	}

	// Read body into memory and persist to disk cache asynchronously/non-blocking
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return CoverData{}, err
	}
	_ = os.WriteFile(filePath, data, 0644)

	// Decode directly from memory (0 extra disk I/O reads)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return CoverData{}, err
	}

	return processCoverImage(img, CoverWidthChars, CoverHeightChars), nil
}

// RenderCoverLines converts an image.Image into ANSI TrueColor half-blocks of widthChars x heightChars.
func RenderCoverLines(img image.Image, widthChars, heightChars int) []string {
	targetW := widthChars
	targetH := heightChars * 2 // each terminal row displays 2 vertical pixels via upper half block '▀'

	scaled := scaleImage(img, targetW, targetH)

	lines := make([]string, heightChars)
	for y := 0; y < heightChars; y++ {
		line := ""
		for x := 0; x < widthChars; x++ {
			cTop := scaled.At(x, y*2)
			cBot := scaled.At(x, y*2+1)
			r1, g1, b1, _ := cTop.RGBA()
			r2, g2, b2, _ := cBot.RGBA()

			// 8-bit color conversion
			topR, topG, topB := uint8(r1>>8), uint8(g1>>8), uint8(b1>>8)
			botR, botG, botB := uint8(r2>>8), uint8(g2>>8), uint8(b2>>8)

			// ANSI 24-bit TrueColor: foreground = top, background = bottom, character = ▀
			line += fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", topR, topG, topB, botR, botG, botB)
		}
		line += "\x1b[0m"
		lines[y] = line
	}
	return lines
}

// RenderBlurredCoverLines converts img into ANSI TrueColor half-blocks with progressive diffusion / blur
// and optional cross-dissolve with prevImg based on progress (0.0 = deeply diffuse/blurred, 1.0 = 100% sharp).
func RenderBlurredCoverLines(img image.Image, prevImg image.Image, widthChars, heightChars int, progress float64) []string {
	if progress >= 1.0 {
		return RenderCoverLines(img, widthChars, heightChars)
	}

	targetW := widthChars
	targetH := heightChars * 2

	scaled := scaleImage(img, targetW, targetH)
	var prevScaled image.Image
	if prevImg != nil {
		prevScaled = scaleImage(prevImg, targetW, targetH)
	}

	progress = max(0.0, min(1.0, progress))

	// Rich, cinematic progressive blur radius:
	// t: 0.00 - 0.28 -> radius 3 (7x7 neighborhood - wide dreamy bokeh/diffusion)
	// t: 0.28 - 0.55 -> radius 2 (5x5 neighborhood - soft forms emerging)
	// t: 0.55 - 0.78 -> radius 1 (3x3 neighborhood - details crystallizing)
	// t: 0.78 - 1.00 -> radius 0 (interpolating to 100% razor sharp)
	blurRadius := 0
	switch {
	case progress < 0.28:
		blurRadius = 3
	case progress < 0.55:
		blurRadius = 2
	case progress < 0.78:
		blurRadius = 1
	default:
		blurRadius = 0
	}

	blurred := applyBoxBlur(scaled, targetW, targetH, blurRadius)

	// Contrast blooms smoothly into full radiance from 38% up to 100%
	contrastFactor := 0.38 + 0.62*progress
	// Slower easing on sharpness so the diffuse feeling lingers and then snaps into focus
	sharpWeight := math.Pow(progress, 1.8)

	lines := make([]string, heightChars)
	for y := 0; y < heightChars; y++ {
		var sb strings.Builder
		for x := 0; x < widthChars; x++ {
			cTop := getInterpolatedPixel(x, y*2, blurred, scaled, prevScaled, sharpWeight, contrastFactor)
			cBot := getInterpolatedPixel(x, y*2+1, blurred, scaled, prevScaled, sharpWeight, contrastFactor)

			sb.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", cTop.R, cTop.G, cTop.B, cBot.R, cBot.G, cBot.B))
		}
		sb.WriteString("\x1b[0m")
		lines[y] = sb.String()
	}
	return lines
}

func applyBoxBlur(src image.Image, w, h, radius int) image.Image {
	if radius <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		yMin := max(0, y-radius)
		yMax := min(h-1, y+radius)
		for x := 0; x < w; x++ {
			xMin := max(0, x-radius)
			xMax := min(w-1, x+radius)

			var rSum, gSum, bSum uint64
			var count uint64
			for sy := yMin; sy <= yMax; sy++ {
				for sx := xMin; sx <= xMax; sx++ {
					r, g, b, _ := src.At(sx, sy).RGBA()
					rSum += uint64(r >> 8)
					gSum += uint64(g >> 8)
					bSum += uint64(b >> 8)
					count++
				}
			}
			if count > 0 {
				dst.Set(x, y, color.RGBA{
					R: uint8(rSum / count),
					G: uint8(gSum / count),
					B: uint8(bSum / count),
					A: 255,
				})
			}
		}
	}
	return dst
}

func getInterpolatedPixel(x, y int, blurred, sharp, prev image.Image, sharpWeight, contrast float64) color.RGBA {
	rB, gB, bB, _ := blurred.At(x, y).RGBA()
	rS, gS, bS, _ := sharp.At(x, y).RGBA()

	// Blend blurred and sharp
	rf := float64(rB>>8)*(1.0-sharpWeight) + float64(rS>>8)*sharpWeight
	gf := float64(gB>>8)*(1.0-sharpWeight) + float64(gS>>8)*sharpWeight
	bf := float64(bB>>8)*(1.0-sharpWeight) + float64(bS>>8)*sharpWeight

	// Cross-dissolve with prev if present and sharpWeight < 0.5
	if prev != nil && sharpWeight < 0.5 {
		prevWeight := 1.0 - (sharpWeight / 0.5)
		rP, gP, bP, _ := prev.At(x, y).RGBA()
		rf = rf*(1.0-prevWeight) + float64(rP>>8)*prevWeight
		gf = gf*(1.0-prevWeight) + float64(gP>>8)*prevWeight
		bf = bf*(1.0-prevWeight) + float64(bP>>8)*prevWeight
	}

	// Apply contrast factor
	rf = rf * contrast
	gf = gf * contrast
	bf = bf * contrast

	return color.RGBA{
		R: uint8(max(0, min(255, int(math.Round(rf))))),
		G: uint8(max(0, min(255, int(math.Round(gf))))),
		B: uint8(max(0, min(255, int(math.Round(bf))))),
		A: 255,
	}
}


// processCoverImage renders both standard and high-resolution TrueColor covers and extracts the dominant color.
func processCoverImage(img image.Image, widthChars, heightChars int) CoverData {
	lines := RenderCoverLines(img, widthChars, heightChars)
	bigLines := RenderCoverLines(img, BigCoverWidthChars, BigCoverHeightChars)

	scaledSmall := scaleImage(img, widthChars, heightChars*2)
	domColor, domHex, secHex := extractCoverPalette(scaledSmall)

	return CoverData{
		Image:         img,
		Lines:         lines,
		BigLines:      bigLines,
		DominantColor: domColor,
		DominantHex:   domHex,
		SecondaryHex:  secHex,
	}
}

// scaleImage resizes src to targetW x targetH using area-averaging (supersampling / box filter)
// to maximize clarity and avoid pixelation/aliasing on downscaled covers.
func scaleImage(src image.Image, targetW, targetH int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	if srcW == 0 || srcH == 0 || targetW == 0 || targetH == 0 {
		return dst
	}

	for y := 0; y < targetH; y++ {
		y0 := bounds.Min.Y + (y*srcH)/targetH
		y1 := bounds.Min.Y + ((y+1)*srcH)/targetH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > bounds.Max.Y {
			y1 = bounds.Max.Y
		}

		for x := 0; x < targetW; x++ {
			x0 := bounds.Min.X + (x*srcW)/targetW
			x1 := bounds.Min.X + ((x+1)*srcW)/targetW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > bounds.Max.X {
				x1 = bounds.Max.X
			}

			var rSum, gSum, bSum, aSum uint64
			var count uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := src.At(sx, sy).RGBA()
					rSum += uint64(r >> 8)
					gSum += uint64(g >> 8)
					bSum += uint64(b >> 8)
					aSum += uint64(a >> 8)
					count++
				}
			}

			if count == 0 {
				count = 1
			}

			dst.Set(x, y, color.RGBA{
				R: uint8(rSum / count),
				G: uint8(gSum / count),
				B: uint8(bSum / count),
				A: uint8(aSum / count),
			})
		}
	}
	return dst
}

// rgbToHSL converts 8-bit RGB values to HSL color space.
func rgbToHSL(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	delta := maxC - minC

	l = (maxC + minC) / 2.0

	if delta < 1e-5 {
		return 0, 0, l
	}

	if l < 0.5 {
		s = delta / (maxC + minC)
	} else {
		s = delta / (2.0 - maxC - minC)
	}

	switch maxC {
	case rf:
		h = (gf - bf) / delta
		if gf < bf {
			h += 6.0
		}
	case gf:
		h = (bf - rf)/delta + 2.0
	case bf:
		h = (rf - gf)/delta + 4.0
	}
	h *= 60.0
	if h < 0 {
		h += 360.0
	}
	return h, s, l
}

// hslToRGB converts HSL values to 8-bit RGB.
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	if s < 1e-5 {
		v := uint8(math.Round(math.Min(255, math.Max(0, l*255.0))))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1.0 + s)
	} else {
		q = l + s - (l * s)
	}
	p := 2.0*l - q

	hNorm := math.Mod(h, 360.0) / 360.0
	if hNorm < 0 {
		hNorm += 1.0
	}

	hueToRGB := func(p, q, t float64) float64 {
		if t < 0 {
			t += 1.0
		}
		if t > 1.0 {
			t -= 1.0
		}
		if t < 1.0/6.0 {
			return p + (q-p)*6.0*t
		}
		if t < 1.0/2.0 {
			return q
		}
		if t < 2.0/3.0 {
			return p + (q-p)*(2.0/3.0-t)*6.0
		}
		return p
	}

	r := hueToRGB(p, q, hNorm+1.0/3.0)
	g := hueToRGB(p, q, hNorm)
	b := hueToRGB(p, q, hNorm-1.0/3.0)

	clampByte := func(val float64) uint8 {
		if val <= 0 {
			return 0
		}
		if val >= 1.0 {
			return 255
		}
		return uint8(math.Round(val * 255.0))
	}

	return clampByte(r), clampByte(g), clampByte(b)
}

// extractCoverPalette performs weighted hue clustering to extract a primary and secondary palette
// that remains faithfully close to the album art ("sin irse a los extremos").
func extractCoverPalette(img image.Image) (domColor color.RGBA, domHex string, secHex string) {
	bounds := img.Bounds()

	type hueBin struct {
		weight float64
		sumH   float64
		sumS   float64
		sumL   float64
		count  int
	}

	var bins [24]hueBin
	var totalChromaWeight float64
	var monoLSum float64
	var monoCount int

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 128<<8 {
				continue
			}
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			h, s, l := rgbToHSL(r8, g8, b8)

			// Ignore extreme darks and extreme lights
			if l < 0.08 || l > 0.92 {
				continue
			}
			// Low saturation = monochrome / grayscale
			if s < 0.12 {
				monoLSum += l
				monoCount++
				continue
			}

			// Weight saturated mid-tones highest
			weight := s * (1.0 - 0.85*math.Abs(l-0.50))
			binIdx := int(h/15.0) % 24
			if binIdx < 0 {
				binIdx += 24
			}

			bins[binIdx].weight += weight
			bins[binIdx].sumH += h * weight
			bins[binIdx].sumS += s * weight
			bins[binIdx].sumL += l * weight
			bins[binIdx].count++
			totalChromaWeight += weight
		}
	}

	// If no chromatic pixels found (monochrome/grayscale cover), return elegant deep charcoal slate (NO GREEN)
	if totalChromaWeight < 0.20 {
		avgL := 0.20
		if monoCount > 0 {
			avgL = monoLSum / float64(monoCount)
		}
		targetL := math.Min(0.24, math.Max(0.12, avgL*0.45))
		domR, domG, domB := hslToRGB(220.0, 0.08, targetL)
		secR, secG, secB := hslToRGB(220.0, 0.05, targetL*0.75)
		domColor = color.RGBA{R: domR, G: domG, B: domB, A: 255}
		domHex = fmt.Sprintf("#%02x%02x%02x", domR, domG, domB)
		secHex = fmt.Sprintf("#%02x%02x%02x", secR, secG, secB)
		return domColor, domHex, secHex
	}

	// Find the top 2 distinct winning hue bins
	bestBin := -1
	maxW := -1.0
	for i := 0; i < 24; i++ {
		if bins[i].weight > maxW {
			maxW = bins[i].weight
			bestBin = i
		}
	}

	secondBin := -1
	secondMaxW := -1.0
	for i := 0; i < 24; i++ {
		diff := int(math.Abs(float64(i - bestBin)))
		if diff > 12 {
			diff = 24 - diff
		}
		if diff >= 2 && bins[i].weight > secondMaxW {
			secondMaxW = bins[i].weight
			secondBin = i
		}
	}

	// 1. Primary Dominant Color
	primH := bins[bestBin].sumH / bins[bestBin].weight
	primRawS := bins[bestBin].sumS / bins[bestBin].weight
	primRawL := bins[bestBin].sumL / bins[bestBin].weight

	// Calibrate ambient saturation and lightness:
	// "Debe usar una paleta cercana a la caratula sin irse a los extremos"
	ambientS := math.Min(0.68, math.Max(0.38, primRawS))
	ambientL := math.Min(0.25, math.Max(0.14, primRawL*0.50))

	domR, domG, domB := hslToRGB(primH, ambientS, ambientL)
	domColor = color.RGBA{R: domR, G: domG, B: domB, A: 255}
	domHex = fmt.Sprintf("#%02x%02x%02x", domR, domG, domB)

	// 2. Secondary Color
	if secondBin != -1 && bins[secondBin].weight >= 0.15*bins[bestBin].weight {
		secH := bins[secondBin].sumH / bins[secondBin].weight
		secRawS := bins[secondBin].sumS / bins[secondBin].weight
		secRawL := bins[secondBin].sumL / bins[secondBin].weight

		secAmbientS := math.Min(0.65, math.Max(0.35, secRawS))
		secAmbientL := math.Min(0.23, math.Max(0.12, secRawL*0.48))
		secR, secG, secB := hslToRGB(secH, secAmbientS, secAmbientL)
		secHex = fmt.Sprintf("#%02x%02x%02x", secR, secG, secB)
	} else {
		// Harmonious analog hue shift
		analogH := math.Mod(primH+20.0, 360.0)
		secR, secG, secB := hslToRGB(analogH, ambientS*0.85, ambientL*0.82)
		secHex = fmt.Sprintf("#%02x%02x%02x", secR, secG, secB)
	}

	return domColor, domHex, secHex
}

// extractDominantColor is kept for compatibility.
func extractDominantColor(img image.Image) color.RGBA {
	c, _, _ := extractCoverPalette(img)
	return c
}

// parseHexRGB quickly parses "#RRGGBB" or "RRGGBB" into RGB components without heap allocations or fmt.Sscanf.
func parseHexRGB(s string) (r, g, b uint8, ok bool) {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	parseNibble := func(c byte) (byte, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		default:
			return 0, false
		}
	}
	n0, ok0 := parseNibble(s[0])
	n1, ok1 := parseNibble(s[1])
	n2, ok2 := parseNibble(s[2])
	n3, ok3 := parseNibble(s[3])
	n4, ok4 := parseNibble(s[4])
	n5, ok5 := parseNibble(s[5])
	if !ok0 || !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return 0, 0, 0, false
	}
	return (n0 << 4) | n1, (n2 << 4) | n3, (n4 << 4) | n5, true
}

func stringsTrimHex(s string) string {
	if len(s) > 0 && s[0] == '#' {
		return s[1:]
	}
	return s
}

// LerpHex smoothly interpolates between two hex colors by factor t (0.0 to 1.0).
func LerpHex(fromHex, toHex string, t float64) string {
	if t <= 0 {
		return fromHex
	}
	if t >= 1 {
		return toHex
	}
	r1, g1, b1, ok1 := parseHexRGB(fromHex)
	r2, g2, b2, ok2 := parseHexRGB(toHex)
	if !ok1 || !ok2 {
		return fromHex
	}

	r := uint8(float64(r1)*(1.0-t) + float64(r2)*t)
	g := uint8(float64(g1)*(1.0-t) + float64(g2)*t)
	b := uint8(float64(b1)*(1.0-t) + float64(b2)*t)

	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// ApplyRowBackground wraps an ANSI-formatted line with a TrueColor background color,
// properly preserving inner foreground styles and terminal line boundaries.
func ApplyRowBackground(line string, bgHex string) string {
	if bgHex == "" {
		return line
	}
	r, g, b, ok := parseHexRGB(bgHex)
	if !ok {
		return line
	}
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	res := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+bgSeq)
	res = strings.ReplaceAll(res, "\x1b[m", "\x1b[m"+bgSeq)
	return bgSeq + res + "\x1b[0m"
}
