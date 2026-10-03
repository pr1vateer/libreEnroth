package ui

import (
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Title screen messages.
//
// mm8: 0x433bbd (Menu_ProcessMessages: 0x36 new, 0x37 load, 0x38 credits, 0x39 quit)
const (
	msgTitleNew     = 0x36
	msgTitleLoad    = 0x37
	msgTitleCredits = 0x38
	msgTitleQuit    = 0x39
)

type title struct {
	ct    Container
	bg    *gfx.Sprite
	font  *text.Font
	toast string
	ttl   int
}

// newTitle builds the main menu: title.pcx and four buttons at x = 515.
//
// mm8: 0x4c74b6 (GuiTitle_Build)
func newTitle(r *Resources) (*title, error) {
	l := &loader{r: r}
	t := &title{bg: l.pcx("title.pcx"), font: l.font("arrus.fnt")}
	for i, b := range []struct {
		y      int
		name   string
		hotkey Key
	}{
		{202, "new", 'N'},
		{241, "load", 'L'},
		{279, "cred", 'C'},
		{317, "quit", 'Q'},
	} {
		btn := l.button(515, b.y, Msg{ID: msgTitleNew + i, Param: i}, "T_"+b.name+"_up", "T_"+b.name+"_dn", "T_"+b.name+"_ht", true)
		btn.Hotkey = b.hotkey
		t.ct.Add(btn)
	}
	return t, l.err
}

func (t *title) Update(in *Input) Transition {
	t.ct.Update(in)
	if t.ttl > 0 {
		t.ttl--
	}
	for _, m := range t.ct.Queue.Drain() {
		switch m.ID {
		case msgTitleNew:
			return goTo(StateCreate)
		case msgTitleLoad:
			t.toast, t.ttl = "Loading saved games is not implemented yet (M10).", 3*60
		case msgTitleCredits:
			return goTo(StateCredits)
		case msgTitleQuit:
			return quitGame
		}
	}
	return Transition{}
}

func (t *title) Draw(c *gfx.Canvas) {
	c.Blit(t.bg, 0, 0)
	t.ct.Draw(c)
	if t.ttl > 0 {
		t.font.DrawCentered(c, text.Rect{X: 0, Y: 440, W: 640, H: 30}, 0, 0, gfx.RGB16(0xff, 0xff, 0x9b), t.toast, 3)
	}
}
