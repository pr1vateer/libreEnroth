package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Dialogs is a world whose events open house and NPC dialogues and the event message
// box; internal/game/world implements it.
type Dialogs interface {
	// Dialog is the open dialogue, nil when none (an NPC deferred by OnMapReload opens
	// here).
	Dialog() *dialog.Dialog
	// CloseDialog leaves the house or ends the NPC's dialogue.
	CloseDialog()
	// MessageBox is the message box an event waits in: its text and whether it asks
	// for an answer.
	MessageBox() (text string, input, ok bool)
	// Answer resumes the waiting event (ok false: Esc).
	Answer(text string, ok bool)
	// TransitionDialog is the open transition dialogue (a MoveToMap asking, a map
	// edge), nil for none; AnswerTransition closes it with OK or Close.
	TransitionDialog() *dialog.Transition
	AnswerTransition(ok bool)
}

// Text colours of the dialogues (Color16 values the draw functions make).
var (
	inkName  = gfx.RGB16(0x15, 0x99, 0xe9) // names above and below the portraits
	inkHover = gfx.RGB16(0xe1, 0xcd, 0x23) // the topic under the mouse
	inkWhite = gfx.RGB16(0xff, 0xff, 0xff)
)

// dialogArt are the pictures and fonts of the dialogue screens.
type dialogArt struct {
	topbar, topbar2, bar8, panel, frame *gfx.Sprite
	leather, endcap, pending, scoreBG   *gfx.Sprite
	arrus, create, smallnum, lucida     *text.Font
	r                                   *Resources
}

// loadDialogArt loads what Game_Init and the dialogue openers load: topbar/topbar2,
// ib-8pxbar, ib-mb-A (the right panel), evtnpc (the portrait frame), LEATHER and endcap
// (the reply box), ScoreBG (behind food and gold), arrus.fnt for the dialogue text.
//
// mm8: 0x4106f2.. (HUD textures), 0x41b7c4.. (fonts), 0x443b6f, 0x443f4b, 0x44328b
func loadDialogArt(r *Resources) (*dialogArt, error) {
	if r.dialogArt != nil {
		return r.dialogArt, nil
	}
	l := &loader{r: r}
	a := &dialogArt{
		r:        r,
		topbar:   l.icon("topbar", false),
		topbar2:  l.icon("topbar2", false),
		bar8:     l.icon("ib-8pxbar", false),
		panel:    l.icon("ib-mb-A", false),
		frame:    l.icon("evtnpc", false),
		leather:  l.icon("LEATHER", false),
		endcap:   l.icon("endcap", false),
		pending:  l.icon("PENDING", false),
		scoreBG:  l.icon("ScoreBG", false),
		arrus:    l.font("arrus.fnt"),
		create:   l.font("create.fnt"),
		smallnum: l.font("smallnum.fnt"),
		lucida:   l.font("lucida.fnt"),
	}
	if l.err != nil {
		return nil, l.err
	}
	r.dialogArt = a
	return a, nil
}

// icon loads an icons.lod picture; a missing one is PENDING, as TexLod_LoadTexture
// does.
//
// mm8: 0x411278 (TexLod_LoadTexture: "pending" on failure)
func (a *dialogArt) icon(name string) *gfx.Sprite {
	if s, err := a.r.Cache.Icon(name, false); err == nil {
		return s
	}
	return a.pending
}

// drawGoldFood draws the food and gold counters in the topbar.
//
// mm8: 0x41b3de (Hud_DrawGoldFood: ScoreBG keyed at 0x52,2; "%lu" in smallnum.fnt,
// white on black, at x 0x87 and 0xeb, y 5)
func drawGoldFood(c *gfx.Canvas, scoreBG *gfx.Sprite, f *text.Font, m *party.Members) {
	c.BlitKeyed(scoreBG, 0x52, 2)
	full := text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}
	f.Draw(c, full, 0x87, 5, inkWhite, gfx.RGB16(0, 0, 0), fmt.Sprint(uint32(m.Food)), 0)
	f.Draw(c, full, 0xeb, 5, inkWhite, gfx.RGB16(0, 0, 0), fmt.Sprint(uint32(m.Gold)), 0)
}

// textHeight is the height of s wrapped to width from x offset xoff: a line is the
// font height less 3.
//
// mm8: 0x44ae82
func textHeight(f *text.Font, s string, width, xoff int) int {
	h := f.Height - 3
	t := f.Wrap(s, width, xoff, false)
	for i := 0; i < len(s) && i < len(t); i++ {
		c := t[i]
		if !f.IsPrintable(c) {
			continue
		}
		switch c {
		case '\t', '\r':
			i += 3
		case '\n':
			h += f.Height - 3
		case '\f':
			i += 5
		}
	}
	return h
}

// drawReply draws the reply box at the bottom of the left area: as many rows of
// LEATHER as the text needs, endcap above them, the text from x 0xd (0xc for the
// message box). smaller reports whether the box is too tall, then create.fnt is used.
//
// mm8: 0x4b3dec, 0x443441, 0x4b363b, 0x442dc0 (with 0x4a6d77 drawing the top rows of
// LEATHER)
func (a *dialogArt) drawReply(c *gfx.Canvas, s string, width, xoff int, smaller func(th int) bool) {
	f := a.arrus
	th := textHeight(f, s, width, xoff) + 7
	if smaller(th) {
		f = a.create
		th = textHeight(f, s, width, xoff) + 7
	}
	if th < a.leather.H {
		c.SetClip(image.Rect(8, 0x16f-th, 8+a.leather.W, 0x16f))
		c.Blit(a.leather, 8, 0x16f-th)
		c.ResetClip()
	}
	c.Blit(a.endcap, 8, 0x16a-th)
	f.Draw(c, text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}, xoff, 0x171-th, 0, 0, f.Wrap(s, width, xoff, false), 0)
}

// topicSpot is a button of the topic column laid out for drawing.
type topicSpot struct {
	b     dialog.Button
	label string
	r     image.Rectangle // hit rectangle (inclusive right/bottom as the original's)
}

// layoutTopics lays the labels out down the right panel: spread over 0xae pixels from
// y 0x8a, at most 0x20 apart, centred in x 0x1e3..0x1e3+0x94; the buttons are 0x8c
// wide from x 0x1e0.
//
// mm8: 0x443441 / 0x4b363b (the button positions are recomputed every frame)
func layoutTopics(f *text.Font, d *dialog.Dialog) []topicSpot {
	n := len(d.Buttons)
	if n == 0 {
		return nil
	}
	spots := make([]topicSpot, n)
	total := 0
	for i, b := range d.Buttons {
		spots[i].b = b
		spots[i].label = d.Label(b)
		total += textHeight(f, spots[i].label, 0x94, 0)
	}
	sp := min((0xae-total)/n, 0x20)
	y := ((0xae-n*sp-total)/2 - sp/2) + 0x8a
	for i := range spots {
		by := y + sp
		h := textHeight(f, spots[i].label, 0x94, 0)
		spots[i].r = image.Rect(0x1e0, by, 0x1e0+0x8c-1, by+h-1)
		y = by - 1 + h
	}
	return spots
}

// drawTopics draws the laid-out labels, the hovered one in yellow.
func drawTopics(c *gfx.Canvas, f *text.Font, spots []topicSpot, hover int) {
	for i, s := range spots {
		ink := inkWhite
		if i == hover {
			ink = inkHover
		}
		f.DrawCentered(c, text.Rect{X: 0x1e3, W: 0x94, H: gfx.ScreenH}, 0, s.r.Min.Y, ink, s.label, 3)
	}
}

// topicAt is the topic under (x, y), -1 for none.
func topicAt(spots []topicSpot, x, y int) int {
	for i, s := range spots {
		if hit(s.r, x, y) {
			return i
		}
	}
	return -1
}

// The dialogue's back/exit button: 0xa9 x 0x23 at 0x1d7,0x1bd, msg 0x71 (its icons,
// ib-bcu-a and BUTTESC2, are not in icons.lod: nothing is drawn).
//
// mm8: 0x41c235 (type 10), 0x4b509f
var exitButton = image.Rect(0x1d7, 0x1bd, 0x1d7+0xa9-1, 0x1bd+0x23-1)

// Portrait positions of the house screen for 1..5 portraits (the evtnpc frame is 4
// pixels up and left of each): x at 0x4f8568, y at 0x4f85f8, [count*6 + i].
const (
	vaPortraitX = 0x4f8568
	vaPortraitY = 0x4f85f8
)

// dialogScreen shows the open dialogue over the game view: an NPC's (the paused view
// shows through on the left) or a house's (its clip plays there).
type dialogScreen struct {
	r        *Resources
	art      *dialogArt
	w        Dialogs
	d        *dialog.Dialog
	video    *houseVideo
	px, py   []int32
	spots    []topicSpot
	hover    int
	portHits []image.Rectangle
	msg      *messageBox
	ticks    int
}

func newDialogScreen(r *Resources, w Dialogs, d *dialog.Dialog) (*dialogScreen, error) {
	art, err := loadDialogArt(r)
	if err != nil {
		return nil, err
	}
	s := &dialogScreen{r: r, art: art, w: w, d: d, hover: -1}
	l := &loader{r: r}
	s.px = l.exeInts(vaPortraitX, 36)
	s.py = l.exeInts(vaPortraitY, 36)
	if l.err != nil {
		return nil, l.err
	}
	if d.Kind == dialog.KindHouse {
		s.video = openHouseVideo(r, d.Anim.Video)
	}
	return s, nil
}

// portraitPos is portrait i's position for count portraits.
func (s *dialogScreen) portraitPos(i, count int) (int, int) {
	k := count*6 + i
	if k < 0 || k >= len(s.px) {
		return 0, 0
	}
	return int(s.px[k]), int(s.py[k])
}

// update runs one tick of the dialogue: the clip, the mouse over the topics, clicks and
// Esc. It reports the dialogue closing.
//
// mm8: 0x42f877 (msgs 0x19a, 0x88, 0xaf, 0x195, 0x71 in g_screenMode 4 and 0xd)
func (s *dialogScreen) update(in *Input) (closed bool) {
	d := s.d
	s.ticks++
	if s.video != nil {
		s.video.tick(!d.VideoOnce)
	}
	if s.updateGoldInput(in) {
		return s.afterClick()
	}
	s.layout()
	s.hover = topicAt(s.spots, in.X, in.Y)
	if in.LeftPressed {
		switch {
		case s.hover >= 0:
			b := s.spots[s.hover].b
			d.Click(b)
			if s.video != nil && b.Msg == dialog.MsgHouseTopic {
				s.video.rewind() // mm8: 0x4b2b74 ends with Video_Rewind
			}
		case d.Kind == dialog.KindHouse && d.Sel == 0 && !d.NoPortraits:
			for i := range d.Portraits {
				x, y := s.portraitPos(i, len(d.Portraits))
				if hit(image.Rect(x, y, x+0x3f-1, y+0x49-1), in.X, in.Y) {
					d.SelectResident(i)
					break
				}
			}
		case hit(exitButton, in.X, in.Y) && (d.Kind == dialog.KindNPC || d.Sel != 0):
			if !s.back() {
				return true
			}
		}
	}
	if s.afterClick() {
		return true
	}
	if d.Input == nil && in.Pressed(KeyEscape) && !s.back() {
		return true
	}
	return false
}

// layout lays out the buttons: the proprietor's per-type screen, else the topic column.
func (s *dialogScreen) layout() {
	if s.d.OnProprietor() || s.d.Type == dialog.TypePrison && s.d.Kind == dialog.KindHouse {
		s.spots = s.servicePanel(-1).spots
	} else {
		s.spots = layoutTopics(s.art.arrus, s.d)
	}
}

// afterClick takes the back steps a click asked for and reports the dialogue closing
// (all the way back, or the house left by a service).
func (s *dialogScreen) afterClick() (closed bool) {
	d := s.d
	if d.Closed {
		s.w.CloseDialog()
		return true
	}
	for d.TakeBack() {
		if !s.back() {
			return true
		}
	}
	return false
}

// back is msg 0x71: one step back; false when the dialogue is over.
func (s *dialogScreen) back() bool {
	if s.d.Back() {
		if s.video != nil && s.d.Kind == dialog.KindHouse {
			s.video.rewind() // mm8: 0x4bda0e (House_Back rewinds the clip)
		}
		return true
	}
	s.w.CloseDialog()
	return false
}

// draw draws the dialogue over the HUD.
//
// mm8: 0x443441 (NPC dialogue), 0x4b3dec (house)
func (s *dialogScreen) draw(c *gfx.Canvas) {
	if s.d.Kind == dialog.KindNPC {
		s.drawNPC(c)
	} else {
		s.drawHouse(c)
	}
}

// drawFrame draws the topbar (topbar2 in houses), the left bar (before the topbar in
// NPC dialogues and the message box, after it in houses), the food and gold and the
// right panel. The evt%02d picture the original puts at 0x1dd,0 does not exist ("evt02"
// loads PENDING) and the panel covers it.
func (a *dialogArt) drawFrame(c *gfx.Canvas, top *gfx.Sprite, barY int, barFirst bool) {
	if barFirst {
		c.Blit(a.bar8, 0, barY)
	}
	c.Blit(top, 0, 0)
	drawGoldFood(c, a.scoreBG, a.smallnum, a.r.Party)
	if !barFirst {
		c.Blit(a.bar8, 0, barY)
	}
	c.Blit(a.panel, 0x1d4, 0)
}

// drawPortrait draws a portrait in its evtnpc frame at x, y.
func (s *dialogScreen) drawPortrait(c *gfx.Canvas, icon string, x, y int) {
	c.Blit(s.art.frame, x-4, y-4)
	if icon != "" {
		c.Blit(s.art.icon(icon), x, y)
	}
}

// drawNPC is an NPC's own dialogue: portrait, name, the greeting or reply, the topics.
//
// mm8: 0x443441
func (s *dialogScreen) drawNPC(c *gfx.Canvas) {
	a, d := s.art, s.d
	a.drawFrame(c, a.topbar, 0x15, true)
	x, y := s.portraitPos(0, 1)
	s.drawPortrait(c, d.PortraitIcon(), x, y)
	a.arrus.DrawCentered(c, text.Rect{W: 630, H: gfx.ScreenH}, 0x1e3, 0x70, inkName, d.Name(), 3)
	if t := d.Text(); t != "" {
		a.drawReply(c, t, 0x1cc, 0xd, func(th int) bool { return 0x16f-th < 8 })
	}
	drawTopics(c, a.arrus, s.spots, s.hover)
}
