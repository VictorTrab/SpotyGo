package ui

import (
	"math"
	"reflect"
	"testing"
)

const testAccent = "#1db954"

func TestShootingStarPointsInBounds(t *testing.T) {
	sizes := []struct{ w, h int }{
		{8, 5}, {20, 10}, {80, 24}, {120, 40}, {200, 60},
	}
	for _, sz := range sizes {
		for frame := 0; frame < 5000; frame += 7 {
			for _, p := range computeShootingStarPoints(frame, sz.w, sz.h, testAccent) {
				if p.x < 0 || p.x >= sz.w || p.y < 0 || p.y >= sz.h {
					t.Fatalf("point out of bounds at frame=%d size=%dx%d: (%d,%d)", frame, sz.w, sz.h, p.x, p.y)
				}
			}
		}
	}
	if pts := computeShootingStarPoints(100, 4, 4, testAccent); pts != nil {
		t.Fatalf("expected nil for tiny grid, got %v", pts)
	}
}

func TestShootingStarDeterministic(t *testing.T) {
	a := computeShootingStarPoints(36, 100, 40, testAccent)
	b := computeShootingStarPoints(36, 100, 40, testAccent)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same frame must produce identical points")
	}
}

func TestShootingStarLifecycle(t *testing.T) {
	const maxScan = 3 * ssCycleTicks
	var frameRuns [][]int
	var run []int
	for frame := 0; frame < maxScan; frame++ {
		pts := computeShootingStarPoints(frame, 120, 40, testAccent)
		if len(pts) > 0 {
			run = append(run, frame)
		} else if len(run) > 0 {
			frameRuns = append(frameRuns, run)
			run = nil
		}
	}
	if len(run) > 0 {
		frameRuns = append(frameRuns, run)
	}
	if len(frameRuns) == 0 {
		t.Fatal("expected at least one meteor flight")
	}

	for _, fr := range frameRuns {
		if len(fr) < ssMinFlight || len(fr) > ssMaxFlight {
			t.Errorf("flight length %d, expected %d frames", len(fr), ssMinFlight)
		}
		pts := computeShootingStarPoints(fr[0], 120, 40, testAccent)
		if len(pts) == 0 || !pts[0].core {
			t.Fatal("first point should be the bright meteor core")
		}
		for i, p := range pts {
			if p.char < 0x2800 || p.char > 0x28ff {
				t.Errorf("point %d is not a Braille cell: %U", i, p.char)
			}
			if i > 0 && p.core {
				t.Errorf("point %d should not be the meteor core cell", i)
			}
		}
		if len(pts) > 60 {
			t.Errorf("point count %d exceeds expected Braille-wake max", len(pts))
		}
	}
}

func TestShootingStarNoDuplicateCells(t *testing.T) {
	for frame := 0; frame < 3000; frame++ {
		pts := computeShootingStarPoints(frame, 100, 30, testAccent)
		seen := make(map[[2]int]bool, len(pts))
		for _, p := range pts {
			key := [2]int{p.x, p.y}
			if seen[key] {
				t.Fatalf("duplicate cell (%d,%d) at frame %d", p.x, p.y, frame)
			}
			seen[key] = true
		}
	}
}

func TestShootingStarCadence(t *testing.T) {
	const maxFrame = 20000
	var starts []int
	prevEmpty := true
	for frame := 0; frame < maxFrame; frame++ {
		pts := computeShootingStarPoints(frame, 100, 30, testAccent)
		if len(pts) > 0 && prevEmpty {
			starts = append(starts, frame)
		}
		prevEmpty = len(pts) == 0
	}
	if len(starts) < 2 {
		t.Fatalf("expected multiple flights, got %d", len(starts))
	}

	var gaps []int
	for i := 1; i < len(starts); i++ {
		gaps = append(gaps, starts[i]-starts[i-1])
	}
	sum := 0
	for _, g := range gaps {
		sum += g
	}
	mean := float64(sum) / float64(len(gaps))
	if mean < float64(ssCycleTicks-1) || mean > float64(ssCycleTicks+1) {
		t.Errorf("mean inter-arrival %.1f frames, expected about %d", mean, ssCycleTicks)
	}
}

func TestShootingStarTrajectoryDirection(t *testing.T) {
	seenDirections := make(map[[2]int]bool, 4)
	for cycle := 0; cycle < 128; cycle++ {
		frame := cycle*ssCycleTicks + 10
		p0, ok0 := shootingStarHead(computeShootingStarPoints(frame, 120, 40, testAccent))
		p1, ok1 := shootingStarHead(computeShootingStarPoints(frame+20, 120, 40, testAccent))
		if !ok0 || !ok1 {
			continue
		}
		dx := p1.x - p0.x
		dy := p1.y - p0.y
		if dx == 0 || dy == 0 {
			continue
		}
		direction := [2]int{1, 1}
		if dx < 0 {
			direction[0] = -1
		}
		if dy < 0 {
			direction[1] = -1
		}
		seenDirections[direction] = true

		angle := math.Atan2(math.Abs(float64(dy)), math.Abs(float64(dx))) * 180 / math.Pi
		if angle < 15 || angle > 27 {
			t.Fatalf("trajectory angle %.1f degrees outside expected range", angle)
		}
	}
	if len(seenDirections) != 4 {
		t.Fatalf("expected all four diagonal directions, saw %v", seenDirections)
	}
}

func TestShootingStarUsesTargetPalette(t *testing.T) {
	pts := computeShootingStarPoints(ssMinFlight/2, 120, 40, testAccent)
	head, ok := shootingStarHead(pts)
	if !ok {
		t.Fatal("expected a meteor core at mid-flight")
	}
	if head.color != ssBeamCore {
		t.Fatalf("core should be white, got %s", head.color)
	}

	blueWake := false
	for _, p := range pts {
		if p.core {
			continue
		}
		r, _, b, valid := parseHexRGB(p.color)
		if valid && int(b)-int(r) >= 20 {
			blueWake = true
			break
		}
	}
	if !blueWake {
		t.Fatal("wake should carry the icy-blue target tint")
	}
}

func TestShootingStarBodyFadesAndThins(t *testing.T) {
	var best []starPoint
	for frame := 0; frame < 3*ssCycleTicks; frame++ {
		pts := computeShootingStarPoints(frame, 160, 50, testAccent)
		if len(pts) > len(best) {
			best = pts
		}
	}
	if len(best) < 10 {
		t.Fatalf("expected a substantial Braille wake, got %d points", len(best))
	}
	if !best[0].core {
		t.Fatal("the core should be first in the deterministic point order")
	}
	for i, p := range best {
		if p.char < 0x2800 || p.char > 0x28ff {
			t.Errorf("point %d is not a Braille cell: %U", i, p.char)
		}
	}

	lum := func(p starPoint) int {
		r, g, b, ok := parseHexRGB(p.color)
		if !ok {
			return 0
		}
		return int(r) + int(g) + int(b)
	}
	trail := best[1:]
	third := len(trail) / 3
	if third == 0 {
		return
	}
	nearSum, farSum := 0, 0
	for i := 0; i < third; i++ {
		nearSum += lum(trail[i])
	}
	for i := len(trail) - third; i < len(trail); i++ {
		farSum += lum(trail[i])
	}
	if nearSum/third <= farSum/third {
		t.Errorf("wake should fade away from its head: near=%d far=%d", nearSum/third, farSum/third)
	}
}

func shootingStarHead(points []starPoint) (starPoint, bool) {
	for _, p := range points {
		if p.core {
			return p, true
		}
	}
	return starPoint{}, false
}
