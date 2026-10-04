package ui

import (
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Status is the status line above the portraits: a timed message (events, refusals,
// the rest screen) that wins over the hover text (what is under the mouse) until it
// expires.
type Status struct {
	timed      string
	timedUntil int // tick when the timed text expires; 0 = none
	hover      string
	tick       int
}

// statusTicksPerSecond: the original times the message with GetTickCount (ms); here
// in 60 Hz ticks.
const statusTicksPerSecond = 60

// Status line geometry: centred in 0x1c2 pixels from x 0xb, at y 0x171.
const (
	statusX     = 0xb
	statusY     = 0x171
	statusWidth = 0x1c2
)

// Show sets the timed message for seconds.
//
// mm8: 0x44a847 (Status_SetTimed)
func (s *Status) Show(msg string, seconds int) {
	s.timed = msg
	s.timedUntil = s.tick + seconds*statusTicksPerSecond
	if s.timedUntil == 0 {
		s.timedUntil = 1
	}
}

// ShowDefault shows "Nothing here" unless a message is up (the empty event).
//
// mm8: 0x44a8a2 (globalTxt[0x209] for 2 seconds)
func (s *Status) ShowDefault(r *Resources) {
	if s.timedUntil == 0 {
		msg, _ := r.Global(0x209)
		s.Show(msg, 2)
	}
}

// Timed is the timed message while it lasts.
func (s *Status) Timed() string {
	if s.timedUntil == 0 {
		return ""
	}
	return s.timed
}

// SetHover sets the hover text; it is ignored while a timed message is up, and an empty
// text leaves the hover text alone (ClearHover clears it).
//
// mm8: 0x41be4a (Status_SetHover)
func (s *Status) SetHover(msg string) {
	if s.timedUntil == 0 && msg != "" {
		s.hover = msg
	}
}

// ClearHover clears the hover text when nothing is under the mouse.
//
// mm8: 0x420aab (Mouse_UpdateHover, the end)
func (s *Status) ClearHover() { s.hover = "" }

// Hover is the current hover text.
func (s *Status) Hover() string { return s.hover }

// Tick advances the clock of the timed message and expires it.
//
// mm8: 0x446f93 (Status_Expire, from Game_Loop)
func (s *Status) Tick() {
	s.tick++
	if s.timedUntil != 0 && s.tick >= s.timedUntil {
		s.timedUntil = 0
	}
}

// fitWidth chops characters off the end of msg until it fits in w.
func fitWidth(f *text.Font, msg string, w int) string {
	for msg != "" && f.TextWidth(msg) > w {
		msg = msg[:len(msg)-1]
	}
	return msg
}

// Draw draws the timed message, else the hover text, centred, white with a black
// shadow in lucida.
//
// mm8: 0x41bf09 (Hud_DrawStatusLine, game view)
func (s *Status) Draw(c *gfx.Canvas, f *text.Font) {
	msg := s.hover
	if s.timedUntil != 0 {
		msg = s.timed
	}
	msg = fitWidth(f, msg, statusWidth)
	if msg == "" {
		return
	}
	x := f.CenterOffset(statusWidth, msg) + statusX
	f.Draw(c, text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}, x, statusY,
		gfx.RGB16(0xff, 0xff, 0xff), gfx.RGB16(0, 0, 0), msg, 0)
}
