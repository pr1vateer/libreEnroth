package ui

import (
	"image"
	"strings"

	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Credits layout.
//
// mm8: 0x49618b (Credits_Run)
var (
	creditsWin     = image.Rect(20, 145, 620, 475) // 600x330 window, also the clip rect
	creditsBody    = gfx.RGB16(0x70, 0x8f, 0xfe)
	creditsHeading = gfx.RGB16(0xec, 0xe6, 0x9c)
)

const creditsShadow gfx.Color16 = 1 // the shadow pass colour (almost black)

type creditLine struct {
	font  *text.Font
	color gfx.Color16
	text  string // without the '_' heading marks
	x, y  int    // in scroller coordinates
}

type credits struct {
	bg     *gfx.Sprite
	lines  []creditLine
	height int // scroller height: text + a window height above and below
	offset int // scroller row at the top of the window
}

// newCredits lays the credits out like the scroller image the original renders once:
// lines centred in 600 px, '_' lines in cchar.fnt, starting one window height down.
//
// mm8: 0x49618b (Credits_Run), 0x44bc83 (Font_DrawScroller), 0x44af21 (scroller height)
func newCredits(r *Resources) (*credits, error) {
	l := &loader{r: r}
	cr := &credits{bg: l.pcx("mm6title.pcx")}
	body, head := l.font("quick.fnt"), l.font("cchar.fnt")
	raw, err := r.Cache.Text("credits.txt")
	l.fail(err)
	if l.err != nil {
		return nil, l.err
	}
	s := string(raw)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	w, h := creditsWin.Dx(), creditsWin.Dy()
	// Scroller height: (body height - 3) per line break plus one, plus a window above
	// and below.
	cr.height = (body.Height-3)*(strings.Count(s, "\n")+1) + 2*h
	y := h
	for _, tok := range strings.Split(s, "\n") {
		if tok == "" { // strtok(..., "\n") skips empty tokens
			continue
		}
		f, col, measured := body, creditsBody, tok
		if tok[0] == '_' {
			f, col, measured = head, creditsHeading, tok[1:]
		}
		t := strings.ReplaceAll(tok, "_", "") // Font_DrawScroller skips every '_'
		cr.lines = append(cr.lines, creditLine{font: f, color: col, text: t, x: f.CenterOffset(w, measured), y: y})
		y += f.Height - 3
	}
	return cr, nil
}

func (cr *credits) Update(in *Input) Transition {
	// A full-screen button with Esc as its hotkey posts 0x71, which leaves the credits.
	if in.Clicked() || in.Pressed(KeyEscape) {
		return goTo(StateTitle)
	}
	cr.offset++
	if cr.offset >= cr.height {
		return goTo(StateTitle)
	}
	return Transition{}
}

func (cr *credits) Draw(c *gfx.Canvas) {
	c.Blit(cr.bg, 0, 0)
	c.SetClip(creditsWin)
	for _, pass := range []struct {
		dy     int
		shadow bool
	}{{1, true}, {0, false}} {
		for _, ln := range cr.lines {
			y := creditsWin.Min.Y + ln.y + pass.dy - cr.offset
			if y+ln.font.Height <= creditsWin.Min.Y || y >= creditsWin.Max.Y {
				continue
			}
			col := ln.color
			if pass.shadow {
				col = creditsShadow
			}
			drawScrollerLine(c, ln.font, creditsWin.Min.X+ln.x, y, col, ln.text)
		}
	}
	c.ResetClip()
}

// drawScrollerLine draws one credits line: glyph value 1 from the font palette, the rest
// in col.
//
// mm8: 0x44ba36 (scroller line), 0x410fad (scroller glyph)
func drawScrollerLine(c *gfx.Canvas, f *text.Font, x, y int, col gfx.Color16, s string) {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !f.IsPrintable(ch) || ch == '\t' || ch == '\r' {
			continue
		}
		switch ch {
		case '\n':
			return
		case '\f':
			col = gfx.Color16(atoi(s, i+1, 5))
			i += 5
			continue
		}
		m := f.Metrics[ch]
		if m.Width == 0 {
			continue
		}
		if i > 0 {
			x += int(m.Left)
		}
		f.GlyphWithShadowPal(c, x, y, ch, col)
		x += int(m.Width) + int(m.Right)
	}
}

// atoi parses up to n leading digits of s[i:].
func atoi(s string, i, n int) int {
	v := 0
	for j := i; j < min(i+n, len(s)) && s[j] >= '0' && s[j] <= '9'; j++ {
		v = v*10 + int(s[j]-'0')
	}
	return v
}
