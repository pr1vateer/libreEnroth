package ui

import (
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// messageBox is the event message box (window type 0x13): the text in the reply box,
// the dialogue's portrait if one is open, and for InputString an answer typed on the
// line below the view. PressAnyKey goes on with any key or click; InputString with
// Enter (the answer) or Esc (no answer).
//
// mm8: 0x44328b (Evt_Suspend opens it), 0x442dc0 (its draw and the end of the input),
// 0x458c04 (the text input: 15 characters)
type messageBox struct {
	r      *Resources
	art    *dialogArt
	text   string
	input  bool
	answer string
	ticks  int
	d      *dialog.Dialog
}

// messageMaxLen is the answer's length limit (FUN_00458c04(..., 0, 0xf, window)).
const messageMaxLen = 0xf

func newMessageBox(r *Resources, text string, input bool, d *dialog.Dialog) (*messageBox, error) {
	art, err := loadDialogArt(r)
	if err != nil {
		return nil, err
	}
	return &messageBox{r: r, art: art, text: text, input: input, d: d}, nil
}

// update reads the keys; done reports the box closing with the answer (ok false: Esc).
func (b *messageBox) update(in *Input) (done bool, answer string, ok bool) {
	b.ticks++
	if !b.input {
		if len(in.Keys) > 0 || in.LeftPressed || in.RightPressed {
			return true, "", !in.Pressed(KeyEscape)
		}
		return false, "", false
	}
	switch {
	case in.Pressed(KeyEnter):
		return true, b.answer, true
	case in.Pressed(KeyEscape):
		return true, "", false
	}
	a := []byte(b.answer)
	for _, r := range in.Runes {
		if r >= 0x20 && r < 0x100 && len(a) < messageMaxLen {
			a = append(a, byte(r))
		}
	}
	if in.Pressed(KeyBackspace) && len(a) > 0 {
		a = a[:len(a)-1]
	}
	b.answer = string(a)
	return false, "", false
}

// draw draws the box over the dialogue (or the game view).
//
// mm8: 0x442dc0
func (b *messageBox) draw(c *gfx.Canvas) {
	a := b.art
	a.drawFrame(c, a.topbar, 0x17, true)
	if d := b.d; d != nil {
		icon, name := "", ""
		if d.Kind == dialog.KindNPC {
			icon, name = d.PortraitIcon(), d.Name()
		} else {
			// The original draws the house's first portrait with the selected
			// resident's name.
			if len(d.Portraits) > 0 {
				icon = d.Portraits[0].Icon
			}
			name = d.NPCName(d.Resident())
		}
		x, y := 521, 38
		c.Blit(a.frame, x-4, y-4)
		if icon != "" {
			c.Blit(a.icon(icon), x, y)
		}
		a.arrus.DrawCentered(c, text.Rect{W: 630, H: gfx.ScreenH}, 0x1e3, 0x70, inkName, name, 3)
	}
	a.drawReply(c, b.text, 0x1cc, 0xc, func(th int) bool { return Viewport.Max.Y-1-th < 8 })
	if b.input {
		y := Viewport.Max.Y - 1 + 2
		full := text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}
		a.lucida.Draw(c, full, 0xd, y, 0, 0, b.answer, 0)
		if b.ticks%60 >= 30 { // the caret blinks: shown in the second half of each second
			a.lucida.Draw(c, full, 0xd+a.lucida.TextWidth(b.answer), y, 0, 0, "_", 0)
		}
	}
}
