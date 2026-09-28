package ui

import (
	"math"
	"sort"
	"time"
)

// starPoint is a single terminal cell in the Zen sky.
type starPoint struct {
	x, y  int
	char  rune
	color string
	core  bool
}

// These values follow MiMoCode's terminal meteor: one event every 8 seconds,
// a 3.6 second flight, and a 32-cell Braille wake.
const (
	ssCycleTicks  = 160 // 8s at the meteor's 50ms frame interval
	ssMinFlight   = 72  // 3.6s at the meteor's 50ms frame interval
	ssMaxFlight   = 72
	ssTrailMax    = 32
	ssTrailStep   = 0.15
	ssAngle       = 0.36 // radians from horizontal in terminal cell coordinates
	ssFrameTime   = 50 * time.Millisecond
	ssFlightTime  = 3600 * time.Millisecond
	ssCycleTime   = 8 * time.Second
	ssSpeedMin    = 0.011
	ssSpeedMax    = 0.038
	ssWakeNear    = 5.0
	ssBeamGlow    = "#b4d7ff"
	ssBeamCore    = "#ffffff"
	ssWakeFadeExp = 1.3
)

// hash01 returns a deterministic pseudo-random value in [0, 1) from a seed.
// A stable seed makes the meteor repeatable without carrying random state.
func hash01(seed float64) float64 {
	v := math.Sin(seed*12.9898+78.233) * 43758.5453
	return v - math.Floor(v)
}

// computeShootingStarPoints keeps the frame-based entry point used by the
// shooting-star unit tests. Runtime rendering uses elapsed time directly.
func computeShootingStarPoints(frame, width, height int, _ string) []starPoint {
	if frame < 0 {
		return nil
	}
	cycle := frame / ssCycleTicks
	cycleFrame := frame % ssCycleTicks
	elapsed := time.Duration(cycleFrame) * ssFrameTime
	return computeShootingStarPointsAt(elapsed, cycle, width, height, func(int) string {
		return "#0a0a0a"
	})
}

// computeShootingStarPointsAt rasterizes the MiMoCode-style meteor into
// Braille cells. Each terminal cell holds a 2x4 subpixel mask, which lets the
// diagonal wake stay thin and smooth while its head travels between corners.
func computeShootingStarPointsAt(elapsed time.Duration, cycle, width, height int, rowBackground func(int) string) []starPoint {
	if width < 8 || height < 5 || elapsed < 0 || elapsed >= ssFlightTime {
		return nil
	}
	if rowBackground == nil {
		rowBackground = func(int) string { return "#0a0a0a" }
	}

	cycleSeed := float64(cycle)
	direction := int(hash01(cycleSeed*2.1+11) * 4)
	dxSign, dySign := 1.0, 1.0
	if direction&1 != 0 {
		dxSign = -1
	}
	if direction&2 == 0 {
		dySign = -1
	}

	startXOffset := hash01(cycleSeed*1.3+5) * math.Max(1, float64(width)*0.15)
	startX := startXOffset
	if dxSign < 0 {
		startX = float64(width) - startXOffset
	}
	startYOffset := float64(int(hash01(cycleSeed*1.7+6) * 2))
	startY := startYOffset
	verticalSpan := float64(height) - startYOffset
	if dySign < 0 {
		startY = float64(height-1) - startYOffset
		verticalSpan = startY + 1
	}
	speed := verticalSpan / (math.Sin(ssAngle) * float64(ssFlightTime/time.Millisecond))
	speed = math.Max(ssSpeedMin, math.Min(ssSpeedMax, speed))
	distance := float64(elapsed/time.Millisecond) * speed
	dx := dxSign * math.Cos(ssAngle)
	dy := dySign * math.Sin(ssAngle)
	headX := startX + distance*dx
	headY := startY + distance*dy
	envelope := math.Sin((float64(elapsed) / float64(ssFlightTime)) * math.Pi)

	type cellData struct {
		mask uint8
		minT float64
	}
	cells := make(map[[2]int]cellData, 48)

	setSubpixel := func(subX, subY int, t float64) {
		cellX := subX >> 1
		cellY := subY >> 2
		if cellX < 0 || cellX >= width || cellY < 0 || cellY >= height {
			return
		}
		bit := brailleBit(subX&1, subY&3)
		key := [2]int{cellX, cellY}
		cell := cells[key]
		if cell.mask == 0 {
			cell.minT = math.Inf(1)
		}
		cell.mask |= 1 << bit
		cell.minT = math.Min(cell.minT, t)
		cells[key] = cell
	}

	setDot := func(x, y, t float64) {
		setSubpixel(int(math.Floor(x*2)), int(math.Floor(y*4)), t)
	}

	// Trace the full wake behind the head. Braille dots are accumulated per
	// terminal cell so neighboring samples form one continuous diagonal glyph.
	for t := 0.0; t <= float64(ssTrailMax); t += ssTrailStep {
		setDot(headX-t*dx, headY-t*dy, t)
	}

	// A small five-dot disk gives the leading point the same rounded hot core.
	headSubX := int(math.Floor(headX * 2))
	headSubY := int(math.Floor(headY * 4))
	for subDX := -1; subDX <= 1; subDX++ {
		for subDY := -1; subDY <= 1; subDY++ {
			if subDX*subDX+subDY*subDY > 1 {
				continue
			}
			setSubpixel(headSubX+subDX, headSubY+subDY, 0)
		}
	}

	headCell := [2]int{headSubX >> 1, headSubY >> 2}
	keys := make([][2]int, 0, len(cells))
	for key := range cells {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		aHead := a == headCell
		bHead := b == headCell
		if aHead != bHead {
			return aHead
		}
		ta, tb := cells[a].minT, cells[b].minT
		if ta != tb {
			return ta < tb
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[0] < b[0]
	})

	points := make([]starPoint, 0, len(keys))
	for _, key := range keys {
		cell := cells[key]
		fade := math.Pow(math.Max(0, 1-cell.minT/float64(ssTrailMax)), ssWakeFadeExp) * envelope
		headBlend := math.Max(0, 1-cell.minT/ssWakeNear)
		beamColor := LerpHex(ssBeamGlow, ssBeamCore, headBlend)
		bg := rowBackground(key[1])
		if bg == "" {
			bg = "#0a0a0a"
		}
		color := LerpHex(bg, beamColor, math.Max(0.02, fade))
		points = append(points, starPoint{
			x:     key[0],
			y:     key[1],
			char:  rune(0x2800 + int(cell.mask)),
			color: color,
			core:  key == headCell,
		})
	}
	return points
}

// brailleBit maps a 2x4 terminal-cell subpixel coordinate to its Unicode
// Braille bit. Braille's dot numbering is not row-major in the lower row.
func brailleBit(col, row int) uint {
	if col == 0 {
		if row == 3 {
			return 6
		}
		return uint(row)
	}
	if row == 3 {
		return 7
	}
	return uint(3 + row)
}
