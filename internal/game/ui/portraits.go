package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

// Portrait panel messages (GuiPortraits_Build 0x4ca6a8).
const (
	msgSelectPlayer = 0x6e // param: 1-based slot (Party_ClickPortrait 0x4213c0)
	msgHoverPlayer  = 0x5e // posted on mouse enter, param the slot (status line, not handled yet)
)

// Portrait panel layout.
const (
	portraitY      = 389 // 0x185
	portraitW      = 62
	portraitH      = 80
	portraitFrames = 56 // face icons per character: "<prefix>01".."<prefix>56"
	shieldY        = 388
	// PortraitFaces is the number of faces with portrait icons (pc01-..pc28-);
	// g_portraitNames has 30 entries.
	PortraitFaces = 28
)

// portraitX is the left edge of each slot.
//
// mm8: 0x4ff9e0 (g_portraitX)
var portraitX = [party.MaxMembers]int{19, 118, 213, 308, 405}

// The bars' tables: the x of the HP and SP bars per slot (a bar starts one pixel
// right of it), and the manafrm frames over them.
//
// mm8: 0x41b67a (Portraits_DrawBars), 0x4ca371 (GuiPortraits_Draw)
const (
	vaHPBarX = 0x4f59a4
	vaSPBarX = 0x4f59b8
	barY     = 0x1ae
	manafrmY = 0x1ad
)

// manafrmX is where the frames go (GuiPortraits_Draw's switch: the HP bar tables'
// values).
var manafrmX = [party.MaxMembers]int{0x54, 0xb5, 0x115, 0x175, 0x1d4}

// shieldX is where IBshield01..04 cover empty slots 2..5.
//
// mm8: 0x4ca371 (GuiPortraits_Draw)
var shieldX = [4]int{110, 208, 305, 403}

// portraits is the party panel under the viewport (GuiPortraits, a child of GuiGame):
// the animated faces, the selection ring and shields over the empty slots.
type portraits struct {
	m       *party.Members
	rng     *party.Rand
	pft     desc.PFT
	faces   [][portraitFrames]*gfx.Sprite // per member, frame t at [t-1]
	loaded  []int                         // the face each entry of faces is
	prefix  []string                      // g_portraitNames
	r       *Resources
	dead    *gfx.Sprite
	erad    *gfx.Sprite
	selring *gfx.Sprite
	shields [4]*gfx.Sprite
	slots   [party.MaxMembers]*Hotspot
	// The HP and SP bars: the frame, the green, yellow and red HP and the blue SP
	// pictures, and their x per slot.
	manafrm                *gfx.Sprite
	barG, barY, barR, barB *gfx.Sprite
	hpBarX, spBarX         []int32
	ctx                    *party.Ctx

	shown    []int // per member: the party.Player.Portrait result to draw
	subTicks int
}

// newPortraits loads the panel for the party in r and adds its slot hotspots to ct.
//
// mm8: 0x4ca6a8 (GuiPortraits_Build), 0x492573 (Portraits_Load), 0x4ca946
// (GuiPortraits_EnableSlots)
func newPortraits(l *loader, ct *Container) *portraits {
	r := l.r
	p := &portraits{
		r:       r,
		m:       r.Party,
		rng:     r.Rand,
		selring: l.icon("selring", false),
		dead:    l.icon("DEAD", false),
		erad:    l.icon("ERADCATE", false),
	}
	for i := range p.shields {
		p.shields[i] = l.icon(fmt.Sprintf("IBshield%02d", i+1), false)
	}
	p.manafrm = l.icon("manafrm", false)
	p.barG, p.barY, p.barR, p.barB = l.icon("manaG", false), l.icon("manaY", false), l.icon("manaR", false), l.icon("manaB", false)
	p.hpBarX = l.exeInts(vaHPBarX, party.MaxMembers)
	p.spBarX = l.exeInts(vaSPBarX, party.MaxMembers)
	ctx, err := r.Ctx()
	l.fail(err)
	p.ctx = ctx
	if ctx != nil {
		p.pft = ctx.PFT
	}
	p.prefix = l.exeStrings(vaPortraitNames, numFaces)
	for _, pl := range p.m.Players {
		if pl.Face < 0 || pl.Face >= PortraitFaces {
			l.fail(fmt.Errorf("portrait: face %d out of range", pl.Face))
		}
	}
	for i := range p.slots {
		s := NewHotspot(portraitX[i], portraitY, portraitW, portraitH, Msg{ID: msgSelectPlayer, Param: i + 1})
		s.Hotkey = Key('1' + i)
		s.onEnter = func(q *MsgQueue) { q.Post(Msg{ID: msgHoverPlayer, Param: i + 1}) }
		p.slots[i] = s
		ct.Add(s)
	}
	if l.err == nil {
		p.sync(l)
		p.pick(0)
	}
	return p
}

// sync loads the faces of members whose face changed (a member hired or dismissed, a
// lich's new face) and enables the slots of the members there are.
//
// mm8: 0x49269b (Portraits_ReloadSlot), 0x4ca946 (GuiPortraits_EnableSlots)
func (p *portraits) sync(l *loader) {
	n := len(p.m.Players)
	for i := range p.slots {
		p.slots[i].disabled = i >= n
	}
	if len(p.shown) != n {
		p.shown = make([]int, n)
	}
	p.faces, p.loaded = p.faces[:min(len(p.faces), n)], p.loaded[:min(len(p.loaded), n)]
	for i := range p.m.Players {
		face := p.m.Players[i].Face
		if i < len(p.loaded) && p.loaded[i] == face {
			continue
		}
		var f [portraitFrames]*gfx.Sprite
		if face >= 0 && face < PortraitFaces && face < len(p.prefix) {
			if l == nil {
				l = &loader{r: p.r}
			}
			for t := range f {
				f[t] = l.icon(fmt.Sprintf("%s%02d", p.prefix[face], t+1), false)
			}
		}
		if i < len(p.faces) {
			p.faces[i], p.loaded[i] = f, face
		} else {
			p.faces, p.loaded = append(p.faces, f), append(p.loaded, face)
		}
	}
}

// pick chooses each member's face for the next draw.
//
// mm8: 0x49286e (Portraits_Draw)
func (p *portraits) pick(ticks int) {
	p.sync(nil)
	for i := range p.m.Players {
		p.shown[i] = p.m.Players[i].Portrait(p.pft, ticks, p.rng)
	}
}

// update runs one 60 Hz tick: the faces are picked for this frame's draw, then the
// expressions advance, as the original draws before Party_TickExpressions. The ticks
// are the game timer's 128 per second, 2/2/3 per tick like World's.
//
// mm8: 0x4917dd (Party_TickExpressions)
func (p *portraits) update() {
	p.subTicks += 128
	ticks := p.subTicks / 60
	p.subTicks %= 60
	p.pick(ticks)
	p.m.TickExpressions(p.pft, ticks, p.rng)
}

// draw draws the faces, the ring around the selected member, the bars' frames, the
// shields over the empty slots and the HP and SP bars. The ready gem (0x4ca970) and the
// buff icon are M8's and M9's.
//
// mm8: 0x4ca371 (GuiPortraits_Draw)
func (p *portraits) draw(c *gfx.Canvas) {
	for i, face := range p.shown {
		switch face {
		case party.PortraitDead:
			c.Blit(p.dead, portraitX[i], portraitY)
		case party.PortraitEradicated:
			c.Blit(p.erad, portraitX[i], portraitY)
		default:
			if s := p.faces[i][face-1]; s != nil {
				c.Blit(s, portraitX[i], portraitY)
			}
		}
	}
	if s := p.m.Selected; s >= 1 && s <= party.MaxMembers {
		c.BlitKeyed(p.selring, portraitX[s-1], portraitY)
	}
	for i := range p.m.Players {
		c.BlitKeyed(p.manafrm, manafrmX[i], manafrmY)
	}
	for slot := len(p.m.Players) + 1; slot <= party.MaxMembers; slot++ {
		if slot >= 2 {
			c.BlitKeyed(p.shields[slot-2], shieldX[slot-2], shieldY)
		}
	}
	p.drawBars(c)
}

// drawBars draws each member's HP bar (green above half, yellow above a quarter, red
// above 0) and SP bar (blue), showing the bottom part of the picture for the share of
// the maximum: the clip starts ftol((1 - share) * h) below the top, h being manaG's
// height for every bar.
//
// mm8: 0x41b67a (Portraits_DrawBars)
func (p *portraits) drawBars(c *gfx.Canvas) {
	if p.ctx == nil {
		return
	}
	e := p.m.Env(p.ctx)
	h := float64(p.barG.H)
	bar := func(s *gfx.Sprite, x int, share float64) {
		share = min(share, 1)
		top := barY + int((1-share)*h)
		c.SetClip(image.Rect(x, top, x+s.W, barY+s.H))
		c.Blit(s, x, barY)
		c.ResetClip()
	}
	for i := range p.m.Players {
		pl := &p.m.Players[i]
		if pl.HP > 0 {
			share := float64(pl.HP) / float64(pl.MaxHP(e))
			switch {
			case share > 0.5:
				bar(p.barG, int(p.hpBarX[i])+1, share)
			case share > 0.25:
				bar(p.barY, int(p.hpBarX[i])+1, share)
			case share > 0:
				bar(p.barR, int(p.hpBarX[i])+1, share)
			}
		}
		if pl.SP > 0 {
			bar(p.barB, int(p.spBarX[i])+1, float64(pl.SP)/float64(pl.MaxSP(e)))
		}
	}
}
