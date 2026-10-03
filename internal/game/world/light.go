package world

import "math"

// Clock is the time of day the world is lit for, and the animation clock.
type Clock struct {
	Hour, Minute int
	Ticks        int // game ticks, 128 per second: texture/sprite animation, sky drift
}

// TicksPerSecond is the game's timer rate.
const TicksPerSecond = 128

// fixed16 converts to the 16.16 values the game's cosine table holds.
func fixed16(v float64) int { return int(math.Round(v * 65536)) }

// cos2048 is the game's cosine of an angle in 2048ths of a turn, 16.16.
//
// mm8: 0x402e04
func cos2048(a int) int { return fixed16(math.Cos(float64(a&2047) * (2 * math.Pi / 2048))) }

// Sun is the outdoor light at a time of day.
type Sun struct {
	Dir    [3]int  // towards the sun, 16.16
	MaxDim int     // darkest shade a surface may get (0 at 13:00 .. 20 at 5:00 and 21:00)
	Night  float64 // 0 by day, 1 at night, ramps 5:00-6:00 and 20:00-21:00
	Dark   bool    // before 5:00 or after 20:59: lit only by the party's torch
}

// SunAt computes the outdoor light. The sun rises in the east at 5:00 and sets in the
// west at 21:00; outside those hours the direction keeps its last value, which here is
// the 5:00 or 21:00 one.
//
// mm8: 0x48a779 (sun vector, max dim), 0x48a274 (night factor)
func SunAt(hour, minute int) Sun {
	var s Sun
	h := min(max(hour, 5), 20)
	m := (h-5)*60 + minute
	if hour < 5 {
		m = 0
	} else if hour > 20 {
		m = 960
	}
	a := 1024 * m / 960
	s.Dir = [3]int{cos2048(a), 0, cos2048(a - 512)}
	x := m
	if m >= 480 {
		x = 960 - m
	}
	s.MaxDim = int(20 - float64(x)*(1.0/480)*20)
	switch {
	case hour < 5 || hour > 20:
		s.Night, s.Dark = 1, true
	case hour < 6:
		s.Night = float64(60-(hour*60-300+minute)) / 60
	case hour == 20:
		s.Night = float64((hour-20)*60+minute) / 60
	}
	return s
}

// TerrainDim is the shade (0 bright .. 20 dark) of a terrain triangle with unit
// normal n.
//
// mm8: 0x4815f0 (20 - (sun . n) * 20, rounded, clamped to 0..20)
func (s *Sun) TerrainDim(n [3]float32) int {
	dot := float64(n[0])*float64(s.Dir[0])/65536 + float64(n[1])*float64(s.Dir[1])/65536 + float64(n[2])*float64(s.Dir[2])/65536
	return min(max(int(math.RoundToEven(20-dot*20)), 0), 20)
}

// FaceDim is the shade of a BModel face with a 16.16 normal.
//
// mm8: 0x47893a (20 - ((n . sun) * 0x140000 >> 32), clamped to 0..31)
func (s *Sun) FaceDim(n [3]int32) int {
	dot := int64(n[0])*int64(s.Dir[0])>>16 + int64(n[1])*int64(s.Dir[1])>>16 + int64(n[2])*int64(s.Dir[2])>>16
	return min(max(20-int((dot*0x140000)>>32), 0), 31)
}

// Shading distances (mm6.ini [shading], forced outdoors by 0x465f96).
const (
	distShade = 0x800
	distMist  = 0x2000
)

// torchRange is the party light radius in units of 1024 at night without a light
// spell (the game reads the Torch Light spell power, 1 when none).
const torchRange = 1

// OutdoorLight is the vertex brightness (0..1) of a surface with shade dim at depth.
//
// mm8: 0x47d384
func (s *Sun) OutdoorLight(dim int, depth float64) float64 {
	if depth == 0 {
		return float64(0xf8) / 255
	}
	if s.Dark {
		l := 0xd8
		if depth > 0 && depth <= torchRange*1024 {
			l = min(int(math.RoundToEven(depth*216/(torchRange*1024))), 0xd8)
		}
		return float64(255-l) / 255
	}
	base := min(max(dim*8, 0), 0xd8)
	l := int(math.RoundToEven((depth/distShade*32+216)*s.Night)) + base
	l = min(l, 0xd8)
	l = max(l, base)
	l = min(l, s.MaxDim*8)
	return float64(255-l) / 255
}
