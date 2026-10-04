package ui

import (
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Transition dialogue buttons: OK and Close (0x19b / 0x19c; 0x5a / 0x5b at a map edge).
const (
	msgTransitionOK    = 0x19b
	msgTransitionClose = 0x19c
)

// transitionScreen is the "enter?" dialogue of a dungeon entrance or a house exit
// (g_screenMode 0x12) and the map-edge one (0x11): the house's clip or the paused view
// on the left, the picture, the destination and the question, OK (Space) and Close
// (Esc).
//
// mm8: 0x4d363d (the buttons: c_ok at 0x204,0x187, c_close at 0x204,0x1af, from
// EnglishD), 0x4428bc / 0x442bee (the draws), 0x4d3564 (closing resumes the timer)
type transitionScreen struct {
	r      *Resources
	art    *dialogArt
	t      *dialog.Transition
	topbar *gfx.Sprite
	icon   *gfx.Sprite
	video  *houseVideo
	ct     Container
	px, py int
}

func newTransitionScreen(r *Resources, t *dialog.Transition) (*transitionScreen, error) {
	art, err := loadDialogArt(r)
	if err != nil {
		return nil, err
	}
	l := &loader{r: r}
	s := &transitionScreen{r: r, art: art, t: t, topbar: l.icon(t.Topbar, false)}
	s.icon = art.pending
	if t.Icon != "" {
		s.icon = art.icon(t.Icon)
	}
	px, py := l.exeInts(vaPortraitX+6*4, 1), l.exeInts(vaPortraitY+6*4, 1)
	s.px, s.py = int(px[0]), int(py[0])
	ok := l.button(0x204, 0x187, Msg{ID: msgTransitionOK}, "c_ok_up", "c_ok_dn", "c_ok_ht", true)
	ok.Hotkey = KeySpace
	cl := l.button(0x204, 0x1af, Msg{ID: msgTransitionClose}, "c_close_up", "c_close_dn", "c_close_ht", true)
	cl.Hotkey = KeyEscape
	s.ct.Add(ok)
	s.ct.Add(cl)
	if l.err != nil {
		return nil, l.err
	}
	if t.Video != "" {
		s.video = openHouseVideo(r, t.Video)
	}
	return s, nil
}

// update runs a tick: the clip and the buttons. done reports an answer.
func (s *transitionScreen) update(in *Input) (done, ok bool) {
	if s.video != nil {
		s.video.tick(true)
	}
	s.ct.Update(in)
	for _, m := range s.ct.Queue.Drain() {
		switch m.ID {
		case msgTransitionOK:
			return true, true
		case msgTransitionClose:
			return true, false
		}
	}
	return false, false
}

// draw draws the dialogue over the HUD.
//
// mm8: 0x4428bc (a MoveToMap's), 0x442bee (a map edge's: no left bar, no frame)
func (s *transitionScreen) draw(c *gfx.Canvas) {
	a, t := s.art, s.t
	s.video.draw(c)
	if t.Bar8 {
		c.Blit(a.bar8, 0, 0x15)
	}
	c.Blit(s.topbar, 0, 0)
	drawGoldFood(c, a.scoreBG, a.smallnum, s.r.Party)
	if t.Bar8 {
		// The frame goes under the panel, which hides it; the picture comes after.
		c.Blit(a.frame, s.px-4, s.py-4)
	}
	c.Blit(a.panel, 0x1d4, 0)
	c.Blit(s.icon, s.px, s.py)
	if t.Title != "" {
		a.create.DrawCentered(c, text.Rect{X: 0x1ed, W: 0x7e, H: gfx.ScreenH}, 0, t.TitleY, 0, t.Title, 3)
	}
	if t.Text != "" {
		h := textHeight(a.create, t.Text, 0x94, 0)
		a.create.DrawCentered(c, panelRect, 0, (0xd4-h)/2+0x65, 0, t.Text, 3)
	}
	s.ct.Draw(c)
}
