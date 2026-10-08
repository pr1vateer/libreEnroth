package ui

import (
	"fmt"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// The Adventurer's Inn (GuiInn, g_screenMode 0x1d): the roster characters waiting for
// the party, a sheet and the paper doll of the one picked, and Hire
// (re/notes/houses.md#the-adventurers-inn).

// InnOpener is a world whose events open the Adventurer's Inn (SpeakInHouse of a house
// whose clip is of type 0x23).
type InnOpener interface {
	// OpenedInn is the inn's house an event opened, 0 for none.
	OpenedInn() int
	// CloseInn closes it (the screen's Esc).
	CloseInn()
}

// Inn screen messages (GuiInn_Build 0x4cae6b).
const (
	msgInnExit   = 0x1c3
	msgInnPick   = 0x1c4 // param: the roster id
	msgInnHire   = 0x1c6
	msgInnUp     = 0x1c8
	msgInnDown   = 0x1c9
	innShown     = 8 // faces shown: four rows of two
	innFirstY    = 0x2e
	innFirstX    = 0x20
	innHLTicks   = 16 // the highlight's frames change after 250 ms (timeGetTime)
	innSheetX    = 0xc0
	innSheetX2   = 0x14c
	innSheetTopY = 0x2f
)

// innSheetRect is the sheet's text area (the rect GuiInn_DrawSheet draws in).
var innSheetRect = text.Rect{W: 0x1b3, H: 0x1e0}

// innScreen is the roster screen over the game view.
type innScreen struct {
	r        *Resources
	g        *inGame
	ct       Container
	da       *dialogArt
	doll     *paperDoll
	pick     pickBuffer
	ctx      *party.Ctx
	bg, bar8 *gfx.Sprite
	hl       [4]*gfx.Sprite // rost_HL1..4
	cellW    int            // npc2901's size: every face's place
	cellH    int
	spots    [innShown]*Hotspot
	ids      []int // GuiInn +0x188: the roster ids listed
	row      int   // +0x15c: the first row shown
	viewed   int   // +0x130: the roster id on the sheet and the doll
	// viewedSlot is the member on the sheet when it was picked by its portrait (the
	// debug party's members have no roster id), -1 otherwise.
	viewedSlot int
	hlFrame    int // +0x144
	hlTicks    int // ticks since the frame changed (+0x148)
	sheetFnt   *text.Font
	closed     bool
}

// newInnScreen opens the roster screen.
//
// mm8: 0x4caac9 (GuiInn_Ctor), 0x4cae6b (GuiInn_Build); the house's greeting sound
// (House_PlaySound 1) is M11's
func newInnScreen(g *inGame) (*innScreen, error) {
	r := g.r
	l := &loader{r: r}
	ctx, err := r.Ctx()
	if err != nil {
		return nil, err
	}
	da, err := loadDialogArt(r)
	if err != nil {
		return nil, err
	}
	s := &innScreen{r: r, g: g, ctx: ctx, da: da, viewed: -1, viewedSlot: -1}
	s.doll = newPaperDoll(l, ctx.Items)
	s.sheetFnt = l.font("smallnum.fnt")
	s.bg = l.icon("rost_bg", false)
	s.bar8 = l.icon("ib-8pxbar", false)
	for i := range s.hl {
		s.hl[i] = l.icon(fmt.Sprintf("rost_HL%d", i+1), false)
	}
	face := l.icon("npc2901", false)
	if l.err != nil {
		return nil, l.err
	}
	s.cellW, s.cellH = face.W, face.H
	for _, h := range g.portraits.slots {
		s.ct.Add(h)
	}
	exit := l.button(0x208, 0x1c2, Msg{ID: msgInnExit}, "but24u", "but24d", "but24h", true)
	exit.Hotkey = KeyEscape
	s.ct.Add(exit)
	hire := l.button(0x208, 0x17c, Msg{ID: msgInnHire}, "but25u", "but25d", "but25h", true)
	hire.Hotkey = 'H'
	s.ct.Add(hire)
	s.list()
	if len(s.ids) > 0 {
		s.viewed = s.ids[0]
	}
	s.rebuildSpots()
	s.ct.Add(l.button(8, 0x2d, Msg{ID: msgInnUp}, "ar_up_up", "ar_up_dn", "ar_up_ht", false))
	s.ct.Add(l.button(8, 0x144, Msg{ID: msgInnDown}, "ar_dn_up", "ar_dn_dn", "ar_dn_ht", false))
	if s.viewed == -1 {
		if m := r.Party; m.Selected >= 1 && m.Selected <= len(m.Players) {
			s.viewedSlot = m.Selected - 1
		}
	}
	if l.err != nil {
		return nil, l.err
	}
	return s, nil
}

// list fills the roster ids waiting at the inn: 1..49 not in the party whose quest bit
// 400 + id is set.
//
// mm8: 0x4cab24 (GuiInn_ListRoster)
func (s *innScreen) list() {
	m := s.r.Party
	s.ids = s.ids[:0]
	for id := 1; id < tables.NumRoster-1; id++ {
		if m.RosterSlot(id) < 0 && m.QBits.Get(400+id) {
			s.ids = append(s.ids, id)
		}
	}
}

// maxRow is the last first row: (count - 1) / 2.
func (s *innScreen) maxRow() int { return (len(s.ids) - 1) / 2 }

// facePos is where face place i (0..7) of the shown rows goes: the hotspots' x 0x20
// or the face width + 0x20, y (height + 2) * row + 0x2e.
func (s *innScreen) facePos(i int) (x, y int) {
	x = innFirstX
	if i%2 != 0 {
		x = s.cellW + 0x20
	}
	return x, (s.cellH+2)*(i/2) + innFirstY
}

// rebuildSpots makes the face hotspots again for the list from the first shown row.
//
// mm8: 0x4cb642 (GuiInn_Rebuild)
func (s *innScreen) rebuildSpots() {
	for _, h := range s.spots {
		if h != nil {
			s.remove(h)
		}
	}
	s.spots = [innShown]*Hotspot{}
	s.list()
	s.row = max(min(s.row, s.maxRow()), 0)
	for i := 0; i < innShown && 2*s.row+i < len(s.ids); i++ {
		id := s.ids[2*s.row+i]
		x, y := s.facePos(i)
		// The original also posts msg 0x1c5 when the mouse enters it: the hover text.
		h := NewHotspot(x, y, s.cellW, s.cellH, Msg{ID: msgInnPick, Param: id})
		s.spots[i] = h
		s.ct.Add(h)
	}
}

// remove takes w out of the screen's children.
func (s *innScreen) remove(w Widget) {
	for i, c := range s.ct.Children {
		if c == w {
			s.ct.Children = append(s.ct.Children[:i], s.ct.Children[i+1:]...)
			return
		}
	}
}

// player is roster character id (in the party or not).
func (s *innScreen) player(id int) *party.Player { return s.r.Party.RosterPlayer(id) }

// update runs one tick. It reports the screen closing.
//
// mm8: 0x42f877 (msgs 0x1c3..0x1c9 in g_screenMode 0x1d)
func (s *innScreen) update(in *Input) bool {
	m := s.r.Party
	s.ct.Update(in)
	s.r.Status.ClearHover()
	for _, h := range s.spots {
		if h != nil && h.hovered {
			s.hoverText(h.Msg.Param)
		}
	}
	for _, msg := range s.ct.Queue.Drain() {
		switch msg.ID {
		case msgInnExit:
			s.closed = true
		case msgInnPick:
			if err := s.view(msg.Param, false); err != nil {
				return true
			}
		case msgInnHire:
			s.hire()
		case msgInnUp:
			// mm8: 0x4cb62b (GuiInn_ScrollUp)
			if s.row > 0 {
				s.row--
				s.rebuildSpots()
			}
		case msgInnDown:
			// mm8: 0x4cb609 (GuiInn_ScrollDown)
			if s.row < s.maxRow() {
				s.row++
				s.rebuildSpots()
			}
		case msgSelectPlayer:
			// mm8: 0x4213c0 (g_screenMode 0x1d: select the member, then view it)
			if msg.Param >= 1 && msg.Param <= len(m.Players) {
				m.Selected = msg.Param
				if err := s.viewMember(msg.Param - 1); err != nil {
					return true
				}
			}
		}
	}
	return s.closed
}

// hoverText is a face's status line: "Name the Class: Condition (id)".
//
// mm8: 0x42f877 (msg 0x1c5)
func (s *innScreen) hoverText(id int) {
	p := s.player(id)
	if p == nil {
		return
	}
	g := s.r.GlobalText
	t := fmt.Sprintf(g(0x1ad), p.Name, g(tables.ClassNameGlobal+p.Class)) + ": "
	if c := int(p.MainCondition()); c < len(conditionNames) {
		t += g(conditionNames[c])
	}
	s.r.Status.SetHover(fmt.Sprintf("%s (%d)", t, id))
}

// view puts roster character id on the sheet; a second click on the one shown opens its
// character screen.
//
// mm8: 0x4cb402 (GuiInn_ViewCharacter)
func (s *innScreen) view(id int, open bool) error {
	if (s.viewed != id || s.viewedSlot >= 0) && !open {
		s.viewed, s.viewedSlot = id, -1
		return nil
	}
	cs, err := newCharScreenFor(s.g, -1, id, true)
	if err != nil {
		return err
	}
	s.g.char = cs
	return nil
}

// viewMember is a portrait clicked in the inn: member i on the sheet, its character
// screen on a second click.
//
// mm8: 0x4213c0 (g_screenMode 0x1d: GuiInn_ViewCharacter(g_partyRoster[slot]))
func (s *innScreen) viewMember(i int) error {
	if id := s.r.Party.Players[i].RosterID; id >= 0 {
		return s.view(id, false)
	}
	if s.viewedSlot != i {
		s.viewed, s.viewedSlot = -1, i
		return nil
	}
	cs, err := newCharScreenFor(s.g, i, -1, true)
	if err != nil {
		return err
	}
	s.g.char = cs
	return nil
}

// shown is the character on the sheet.
func (s *innScreen) shown() *party.Player {
	if m := s.r.Party; s.viewedSlot >= 0 && s.viewedSlot < len(m.Players) {
		return &m.Players[s.viewedSlot]
	}
	return s.player(s.viewed)
}

// hire has the character on the sheet join the party, which then selects it (or no one,
// when it could not join). The list loses it.
//
// mm8: 0x42f877 (msg 0x1c6: Party_AddRosterMember, GuiInn_Rebuild)
func (s *innScreen) hire() {
	m := s.r.Party
	if s.viewedSlot >= 0 {
		m.Selected = s.viewedSlot + 1 // a member already: AddRoster refuses it
		return
	}
	m.AddRoster(s.viewed)
	s.rebuildSpots()
	m.Selected = m.RosterSlot(s.viewed) + 1
}

// draw draws the screen: rost_bg, the bar, the doll and the sheet of the character
// picked, the topbar, the faces with the blinking frame around the one picked, the
// children (the portrait panel with its basebar first, then the buttons), then the
// status line and the gold and food.
//
// mm8: 0x4cab82 (GuiInn_Draw)
func (s *innScreen) draw(c *gfx.Canvas) {
	s.pick.clear()
	c.Blit(s.bg, 0, 0x17)
	c.Blit(s.bar8, 0x1cb, 0x15)
	if p := s.shown(); p != nil {
		s.doll.draw(c, p, &s.pick, false, 2)
		s.drawSheet(c, p)
	}
	c.Blit(s.da.topbar, 0, 0)
	for i := 0; i < innShown && 2*s.row+i < len(s.ids); i++ {
		id := s.ids[2*s.row+i]
		p := s.player(id)
		if p == nil {
			continue
		}
		x, y := s.facePos(i)
		if i%2 != 0 {
			x += 2
		}
		c.Blit(s.da.icon(fmt.Sprintf("npc29%02d", p.Face+1)), x, y)
		if id == s.viewed && s.viewedSlot < 0 {
			hx, hy := s.facePos(i)
			if i%2 == 0 {
				hx -= 2
			}
			c.BlitKeyed(s.hl[s.hlFrame], hx, hy-2)
			if s.hlTicks++; s.hlTicks >= innHLTicks {
				s.hlTicks = 0
				s.hlFrame = (s.hlFrame + 1) % len(s.hl)
			}
		}
	}
	g := s.g
	c.Blit(g.basebar, 0, 367)
	g.portraits.draw(c)
	s.ct.Draw(c)
	s.r.Status.Draw(c, g.lucida)
	drawGoldFood(c, s.da.scoreBG, s.da.smallnum, s.r.Party)
}

// drawSheet is the sheet beside the doll: name, class, HP and level, AC and spell
// points, attack and damage, shooting and damage, skills known and skill points, the
// condition, the quick spell (M9: none) and the biography.
//
// mm8: 0x4cb84a (GuiInn_DrawSheet)
func (s *innScreen) drawSheet(c *gfx.Canvas, p *party.Player) {
	f, g := s.sheetFnt, s.r.GlobalText
	e := s.r.Party.Env(s.ctx)
	r := innSheetRect
	step := f.Height + 2
	line := func(x, y, w int, t string) { f.DrawClipped(c, r, x, y, 0, t, w) }
	line(innSheetX, innSheetTopY, 0xe6, fmt.Sprintf("%s: %s", g(0x95), p.Name))
	y := innSheetTopY + step
	line(innSheetX, y, 0xfa, fmt.Sprintf("%s: %s", g(0x29), g(tables.ClassNameGlobal+p.Class)))
	y += step
	line(innSheetX, y, 0x96, fmt.Sprintf("%s: %d", g(0x6b), p.HP))
	line(innSheetX2, y, 0x50, fmt.Sprintf("%s: %d", g(0x83), uint32(p.Level(e))))
	y += step
	line(innSheetX, y, 0x96, fmt.Sprintf("%s: %d", g(0), p.AC(e)))
	line(innSheetX2, y, 0x50, fmt.Sprintf("%s: %d", g(0xd1), uint32(p.SP)))
	y += step
	line(innSheetX2, y, 0x96, fmt.Sprintf("%s: %s", g(0x42), p.MeleeDamageText(e, g)))
	line(innSheetX, y, 0x50, fmt.Sprintf("%s: %+d", g(0x12), p.MeleeAttack(e, false)))
	y += step
	line(innSheetX2, y, 0x96, fmt.Sprintf("%s: %s", g(0x42), p.RangedDamageText(e, g)))
	line(innSheetX, y, 0x50, fmt.Sprintf("%s: %+d", g(0xcb), p.RangedAttack(e)))
	y += step
	known := 0
	for _, sk := range p.Skills {
		if sk != 0 {
			known++
		}
	}
	line(innSheetX, y, 0x96, fmt.Sprintf("%s: %d", g(0xcd), known))
	line(innSheetX2, y, 0x50, fmt.Sprintf("%s: %d", g(0xa8), uint32(p.SkillPoints)))
	cond := p.MainCondition()
	line(innSheetX, y+step, 0x96, fmt.Sprintf("%s: %s", g(0x2d), g(conditionNames[cond])))
	y += 2 * step
	line(innSheetX, y, 0x96, fmt.Sprintf("%s: %s", g(0xaa), g(0x99)))
	f.Draw(c, r, innSheetX, y+3*step, 0, 0, p.Biography, 0)
}
