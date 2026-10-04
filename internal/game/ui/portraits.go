package ui

import (
	"fmt"

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
	dead    *gfx.Sprite
	erad    *gfx.Sprite
	selring *gfx.Sprite
	shields [4]*gfx.Sprite
	slots   [party.MaxMembers]*Hotspot

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
		m:       r.Party,
		rng:     r.Rand,
		selring: l.icon("selring", false),
		dead:    l.icon("DEAD", false),
		erad:    l.icon("ERADCATE", false),
	}
	for i := range p.shields {
		p.shields[i] = l.icon(fmt.Sprintf("IBshield%02d", i+1), false)
	}
	pft, err := r.PFT()
	l.fail(err)
	p.pft = pft
	prefixes := l.exeStrings(vaPortraitNames, numFaces)
	for _, pl := range p.m.Players {
		var f [portraitFrames]*gfx.Sprite
		if pl.Face < 0 || pl.Face >= PortraitFaces {
			l.fail(fmt.Errorf("portrait: face %d out of range", pl.Face))
			continue
		}
		for t := range f {
			f[t] = l.icon(fmt.Sprintf("%s%02d", prefixes[pl.Face], t+1), false)
		}
		p.faces = append(p.faces, f)
	}
	for i := range p.slots {
		s := NewHotspot(portraitX[i], portraitY, portraitW, portraitH, Msg{ID: msgSelectPlayer, Param: i + 1})
		s.Hotkey = Key('1' + i)
		s.disabled = i >= len(p.m.Players)
		s.onEnter = func(q *MsgQueue) { q.Post(Msg{ID: msgHoverPlayer, Param: i + 1}) }
		p.slots[i] = s
		ct.Add(s)
	}
	if l.err == nil {
		p.shown = make([]int, len(p.m.Players))
		p.pick(0)
	}
	return p
}

// pick chooses each member's face for the next draw.
//
// mm8: 0x49286e (Portraits_Draw)
func (p *portraits) pick(ticks int) {
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

// draw draws the faces, the ring around the selected member and the shields over the
// empty slots. The ready gem, buff icon, mana frames and HP/SP bars come with the
// stats (M7).
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
			c.Blit(p.faces[i][face-1], portraitX[i], portraitY)
		}
	}
	if s := p.m.Selected; s >= 1 && s <= party.MaxMembers {
		c.BlitKeyed(p.selring, portraitX[s-1], portraitY)
	}
	for slot := len(p.m.Players) + 1; slot <= party.MaxMembers; slot++ {
		if slot >= 2 {
			c.BlitKeyed(p.shields[slot-2], shieldX[slot-2], shieldY)
		}
	}
}
