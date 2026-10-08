package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// The character screen (GuiChar, g_screenMode 7): the stats, skills, awards and
// inventory pages beside the paper doll (re/notes/inventory.md#character-screen).

// Character screen messages (GuiChar_Build 0x4cc1f4) and its pages.
const (
	msgCharStats     = 0x73
	msgCharSkills    = 0x72
	msgCharInventory = 0x74
	msgCharAwards    = 0x75
	msgCharExit      = 0xa8
	msgCharPage      = 0x78 // a click on the page area
	msgCharDoll      = 0x85 // a click on the paper doll
	msgCharSkill     = 0x79 // a skill label: param skill, param2 its index
	msgCharMagnify   = 0x55
	msgAwardsUp      = 0xa9
	msgAwardsDown    = 0xaa
	msgAwardsBar     = 0xc0
	msgCharDismiss   = 0x1c7

	pageSkills    = 0
	pageStats     = 1
	pageAwards    = 2
	pageInventory = 3
	pageNone      = 4
	pageRefresh   = 5
)

// CharPage values (g_charPage 0x5184f8): which page draws.
const (
	CharPageStats     = 100
	CharPageSkills    = 0x65
	CharPageAwards    = 0x66
	CharPageInventory = 0x67
)

// The skills page's groups (0x4f590c weapons, 0x4f5960 magic, 0x4f58f8 armour, 0x4f5930
// misc).
const (
	vaSkillsWeapons = 0x4f590c
	vaSkillsMagic   = 0x4f5960
	vaSkillsArmor   = 0x4f58f8
	vaSkillsMisc    = 0x4f5930
	vaAwardColors   = 0x4f5990 // [6] RGB by award sort % 6
	vaStatRects     = 0x4f5820 // [26] {x, y, w, h} shorts: the stats page's pop-up spots
)

// Skill label colours.
var (
	inkSkill      = gfx.RGB16(0xff, 0xff, 0xff)
	inkSkillCan   = gfx.RGB16(0, 0xaf, 0xff) // enough points to raise it
	inkSkillNo    = gfx.RGB16(0xff, 0, 0)    // just raised, not enough for the next
	inkSkillAgain = gfx.RGB16(0, 0xff, 0)    // just raised, enough for the next
	inkGreen      = gfx.RGB16(0, 0xff, 0)
)

// skillRow is a skill's two labels on the skills page.
type skillRow struct {
	name, level *Label
	skill       int
}

// charScreen is the character screen over the game view.
type charScreen struct {
	r      *Resources
	g      *inGame
	ct     Container
	art    *popupArt
	da     *dialogArt
	doll   *paperDoll
	pick   pickBuffer
	ctx    *party.Ctx
	it     *tables.Items
	member int // 0-based slot shown (GuiChar +0x128, a g_players index in the original)
	// roster is the roster character shown instead of a member (the inn's view of one
	// not in the party), -1 for none; fromInn is the inn's mode 1 (no dismiss button).
	roster  int
	fromInn bool
	// dismiss is the dismiss button (but26, mode 0) and armed its first click
	// (GuiChar +0x2c0, +0x2c4).
	dismiss *Button
	armed   bool
	page    int // +0x124
	btn     [4]*Button
	fr      [4]*gfx.Sprite // fr_skill, fr_stats, fr_award, fr_inven by page
	groups  [4][]int32     // weapons, magic, armour, misc
	rows    []skillRow
	// awards: the sorted ids, the first shown, the number drawn last time and the
	// scroll requests (0x5db874, 0x517a4c, 0x517a44, 0x517a68/6c, 0x517a40).
	awards           []int
	awardTop, drawn  int
	awardDn, awardUp bool
	awardPage        int
	awardColors      [6][3]uint8
	awardWidgets     []Widget
	statRects        []image.Rectangle
	skillFont        *text.Font
	// The right button: speakOnce lets a pop-up's reaction speak once per press
	// (0x4f73d8); used latches a mix until the release (0x51e480).
	rightHeld, speakOnce, used bool
	scroll                     int32 // a message scroll shown (items.txt id), 0 none
	closed                     bool
	dt                         int
}

// newCharScreen opens the character screen on member (0-based) at the page CharPage
// says (stats when it says none).
//
// mm8: 0x4213c0 (Party_ClickPortrait: GuiChar_Ctor 0x4cc0ec, GuiStack_Push -> Build
// 0x4cc1f4, GuiChar_SetPlayer 0x4cc855, GuiChar_SetPage 0x4cca80)
func newCharScreen(g *inGame, member int) (*charScreen, error) {
	return newCharScreenFor(g, member, -1, false)
}

// newCharScreenFor opens the character screen on member, or (roster >= 0) on roster
// character roster; fromInn is the Adventurer's Inn's view (GuiChar_Build mode 1: no
// dismiss button).
//
// mm8: 0x4cb402 (GuiInn_ViewCharacter: GuiStack_Push(.., 1))
func newCharScreenFor(g *inGame, member, roster int, fromInn bool) (*charScreen, error) {
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
	s := &charScreen{r: r, g: g, ctx: ctx, it: ctx.Items, da: da, art: loadPopupArt(l), page: pageNone, member: member,
		roster: roster, fromInn: fromInn}
	if i := r.Party.RosterSlot(roster); roster >= 0 && i >= 0 {
		s.member, s.roster = i, -1 // a member viewed from the inn
	}
	s.doll = newPaperDoll(l, ctx.Items)
	s.skillFont = l.font("lucida.fnt")
	for i, n := range [4]string{"fr_skill", "fr_stats", "fr_award", "fr_inven"} {
		s.fr[i] = l.icon(n, false)
	}
	for i, g := range []struct {
		va uint32
		n  int
	}{{vaSkillsWeapons, 9}, {vaSkillsMagic, 12}, {vaSkillsArmor, 5}, {vaSkillsMisc, 12}} {
		s.groups[i] = l.exeInts(g.va, g.n)
	}
	col := l.exeBytes(vaAwardColors, 18)
	for i := range s.awardColors {
		copy(s.awardColors[i][:], col[3*i:])
	}
	rects := l.exeBytes(vaStatRects, 26*8)
	for i := 0; i < 26; i++ {
		v := func(k int) int { return int(int16(uint16(rects[8*i+2*k]) | uint16(rects[8*i+2*k+1])<<8)) }
		s.statRects = append(s.statRects, image.Rect(v(0), v(1), v(0)+v(2), v(1)+v(3)))
	}
	// Build: the portrait panel first (its hotspots), then the page buttons.
	for _, h := range g.portraits.slots {
		s.ct.Add(h)
	}
	for i, b := range []struct {
		x, msg, page int
		icon         string
		key          Key
	}{
		{0xb, msgCharStats, pageStats, "but20", 'S'},
		{0x65, msgCharSkills, pageSkills, "but21", 'K'},
		{0xbf, msgCharInventory, pageInventory, "but22", 'I'},
		{0x119, msgCharAwards, pageAwards, "but23", 'A'},
	} {
		_ = i
		btn := l.button(b.x, 0x154, Msg{ID: b.msg}, b.icon+"u", b.icon+"d", b.icon+"h", true)
		btn.Hotkey = b.key
		s.btn[b.page] = btn
		s.ct.Add(btn)
	}
	exit := l.button(0x173, 0x154, Msg{ID: msgCharExit}, "but24u", "but24d", "but24h", true)
	exit.Hotkey = KeyEscape
	s.ct.Add(exit)
	pageArea := NewHotspot(0, 0x17, 0x1dc, 0x134, Msg{ID: msgCharPage})
	pageArea.activateOnPress = true
	dollArea := NewHotspot(0x1dc, 0x17, 0xa4, 0x158, Msg{ID: msgCharDoll})
	dollArea.activateOnPress = true
	s.ct.Add(pageArea)
	s.ct.Add(dollArea)
	// The magnifier, added last, sees clicks before the doll's hotspot under it (and
	// the original makes it the captured widget too, vt+0xb4).
	s.ct.Add(NewHotspot(0x25e, 300, 0x1e, 0x1e, Msg{ID: msgCharMagnify}))
	if !s.fromInn {
		// mm8: 0x4cc1f4 (mode 0: but26, msg 0x1c7, added unless member 1 is selected)
		s.dismiss = l.button(0x208, 0x1a4, Msg{ID: msgCharDismiss}, "but26u", "but26d", "but26h", true)
		if r.Party.Selected != 1 {
			s.ct.Add(s.dismiss)
		}
	}
	if l.err != nil {
		return nil, l.err
	}
	page := pageStats
	switch r.Party.CharPage {
	case CharPageSkills:
		page = pageSkills
	case CharPageInventory:
		page = pageInventory
	case CharPageAwards:
		page = pageAwards
	}
	s.setPage(page)
	return s, nil
}

// player is the member shown.
func (s *charScreen) player() *party.Player {
	if s.roster >= 0 {
		return s.r.Party.RosterPlayer(s.roster)
	}
	return &s.r.Party.Players[s.member]
}

// rosterView reports a roster character outside the party on show: what acts on a
// member (skill points, the pack, the doll, mixing) is left out for it.
func (s *charScreen) rosterView() bool {
	if s.roster >= 0 {
		s.r.note("roster-edit", "changing a roster character outside the party from the inn")
		return true
	}
	return false
}

// setPage leaves the page shown (its skill labels or award buttons go; its button's up
// and hover pictures swap back) and shows page (pageRefresh: rebuilds the same one).
//
// mm8: 0x4cca80 (GuiChar_SetPage; GuiButton_SwapUpHover 0x4c4dad)
func (s *charScreen) setPage(page int) {
	if s.page != pageNone {
		switch s.page {
		case pageSkills:
			s.clearSkills()
		case pageAwards:
			s.clearAwards()
		}
		if page != pageRefresh {
			s.swapHover(s.page)
		}
	}
	if page != pageRefresh {
		s.page = page
	}
	switch s.page {
	case pageSkills:
		s.buildSkills()
	case pageAwards:
		s.listAwards()
		s.buildAwards()
	}
	if page != pageRefresh {
		s.swapHover(s.page)
	}
}

func (s *charScreen) swapHover(page int) {
	if page < 0 || page >= len(s.btn) || s.btn[page] == nil {
		return
	}
	b := s.btn[page]
	b.Icons[StateUp], b.Icons[StateHover] = b.Icons[StateHover], b.Icons[StateUp]
}

func (s *charScreen) remove(ws []Widget) {
	kept := s.ct.Children[:0]
	for _, c := range s.ct.Children {
		drop := false
		for _, w := range ws {
			if c == w {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, c)
		}
	}
	s.ct.Children = kept
	if s.ct.hover != nil {
		for _, w := range ws {
			if s.ct.hover == w {
				s.ct.hover = nil
			}
		}
	}
}

// clearSkills removes the skill labels.
//
// mm8: 0x41a2a7 (GuiChar_ClearSkills)
func (s *charScreen) clearSkills() {
	var ws []Widget
	for _, r := range s.rows {
		ws = append(ws, r.name)
		if r.level != nil {
			ws = append(ws, r.level)
		}
	}
	s.remove(ws)
	s.rows = nil
}

// buildSkills makes a label pair per skill with a level, by group: weapons and magic
// on the left, armour and misc on the right, a "None" label for an empty group; light
// blue when the member has the points to raise it. The left column takes magic skills
// only while fewer than 15 labels are made.
//
// mm8: 0x4192e4 (GuiChar_BuildSkills)
func (s *charScreen) buildSkills() {
	p := s.player()
	f := s.skillFont
	lh := f.Height - 3
	idx := 0
	add := func(x, w, lx, y int, skill int32, swapRanks bool) {
		v := p.Skills[skill]
		lvl := int(v & party.SkillLevel)
		ink := inkSkill
		if lvl+1 <= int(p.SkillPoints) {
			ink = inkSkillCan
		}
		name := s.r.GlobalText(tables.SkillNameGlobal[skill])
		rank := ""
		switch {
		case v&party.SkillGM != 0:
			rank = s.r.GlobalText(0x60)
		case !swapRanks && v&party.SkillMaster != 0, swapRanks && v&party.SkillExpert == 0 && v&party.SkillMaster != 0:
			rank = s.r.GlobalText(0x1b0)
		case v&party.SkillExpert != 0:
			rank = s.r.GlobalText(0x1b1)
		}
		if rank != "" {
			name += " " + rank
		}
		nl := NewLabel(text.Rect{X: x, Y: y, W: w, H: lh}, f, name)
		nl.Color, nl.Msg = ink, Msg{ID: msgCharSkill, Param: int(skill), Param2: idx}
		ll := NewLabel(text.Rect{X: lx, Y: y, W: 0x1e, H: lh}, f, fmt.Sprint(lvl))
		ll.Color, ll.Msg, ll.Align = ink, nl.Msg, AlignCenter
		s.rows = append(s.rows, skillRow{name: nl, level: ll, skill: int(skill)})
		s.ct.Add(nl)
		s.ct.Add(ll)
		idx++
	}
	none := func(x, y int) {
		nl := NewLabel(text.Rect{X: x, Y: y, W: 0x9b, H: lh}, f, s.r.GlobalText(0x99))
		s.rows = append(s.rows, skillRow{name: nl, skill: -1})
		s.ct.Add(nl)
		idx++
	}
	column := func(x, lx int, first, second []int32, limit bool) {
		y := 2*f.Height + 0x1c
		n := 0
		for _, sk := range first {
			if p.Skills[sk]&party.SkillLevel != 0 {
				y += lh
				add(x, 0xa3, lx, y, sk, !limit)
				n++
			}
		}
		if n == 0 {
			y += lh
			none(x, y)
		}
		y += 2*f.Height - 6
		n = 0
		for _, sk := range second {
			if p.Skills[sk]&party.SkillLevel != 0 && (!limit || idx < 0xf) {
				y += lh
				add(x, 0xa3, lx, y, sk, !limit)
				n++
			}
		}
		if n == 0 {
			y += lh
			none(x, y)
		}
	}
	column(0x18, 0xbc, s.groups[0], s.groups[1], true)
	column(0xf6, 0x199, s.groups[2], s.groups[3], false)
}

// refreshSkills recolours the labels after a skill was raised: the raised one green
// while the points allow another level, else red; the others light blue or white.
//
// mm8: 0x4cc869 (GuiChar_RefreshSkill)
func (s *charScreen) refreshSkills(index int) {
	p := s.player()
	k := 0
	for _, r := range s.rows {
		if r.skill < 0 {
			k++
			continue
		}
		can := int(p.Skills[r.skill]&party.SkillLevel)+1 <= int(p.SkillPoints)
		ink := inkSkill
		switch {
		case k == index && can:
			ink = inkSkillAgain
		case k == index:
			ink = inkSkillNo
		case can:
			ink = inkSkillCan
		}
		r.name.Color, r.level.Color = ink, ink
		if k == index {
			r.level.Text = fmt.Sprint(p.Skills[r.skill] & party.SkillLevel)
		}
		k++
	}
}

// listAwards collects the member's awards that have a text and sorts them by their
// sort value (drawing an unused rand() % 16 for each first).
//
// mm8: 0x418ffb (GuiChar_ListAwards)
func (s *charScreen) listAwards() {
	p := s.player()
	defs := s.awardDefs()
	s.awards = s.awards[:0]
	for id := range defs {
		if p.Awards.Get(id) && defs[id].Text != "" {
			s.awards = append(s.awards, id)
		}
	}
	s.awardTop, s.drawn, s.awardDn, s.awardUp, s.awardPage = 0, 0, false, false, 0
	for range s.awards {
		s.ctx.Rand.Int()
	}
	a := s.awards
	for i := range a {
		for j := i + 1; j < len(a); j++ {
			if defs[a[j]].Sort < defs[a[i]].Sort {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func (s *charScreen) awardDefs() []tables.Award { return s.r.Awards() }

// buildAwards adds the awards page's scroll arrows and bar.
//
// mm8: 0x419131 (GuiChar_BuildAwards)
func (s *charScreen) buildAwards() {
	l := &loader{r: s.r}
	up := l.button(0x1b2, 0x40, Msg{ID: msgAwardsUp}, "ar_up_up", "ar_up_dn", "ar_up_ht", false)
	dn := l.button(0x1b2, 0x137, Msg{ID: msgAwardsDown}, "ar_dn_up", "ar_dn_dn", "ar_dn_ht", false)
	bar := NewHotspot(0x1b2, 0x4e, 0x10, 0xe5, Msg{ID: msgAwardsBar})
	s.awardWidgets = []Widget{up, dn, bar}
	for _, w := range s.awardWidgets {
		s.ct.Add(w)
	}
}

// clearAwards removes them.
//
// mm8: 0x4192a5 (GuiChar_ClearAwards)
func (s *charScreen) clearAwards() {
	s.remove(s.awardWidgets)
	s.awardWidgets = nil
}

// notes implements party.Notes for the screen.
type charNotes struct{ s *charScreen }

func (n charNotes) Global(i int) string             { return n.s.r.GlobalText(i) }
func (n charNotes) Status(text string, seconds int) { n.s.r.Status.Show(text, seconds) }
func (n charNotes) Stub(key, what string)           { n.s.r.note(key, what) }
func (n charNotes) AutonoteText(i int) bool {
	if w, ok := n.s.g.world.(interface{ AutonoteText(int) bool }); ok {
		return w.AutonoteText(i)
	}
	return false
}

// update runs one tick of the screen; it reports the screen closing.
//
// mm8: 0x42f877 (the screen's messages), 0x41697c (Mouse_RightClick while the right
// button is held), 0x433b68 (its release)
func (s *charScreen) update(in *Input) bool {
	s.dt = 2 // ticks of the game timer a frame (2/2/3 at 60 Hz; the sparkle only)
	m := s.r.Party
	s.ct.Update(in)
	for _, msg := range s.ct.Queue.Drain() {
		switch msg.ID {
		case msgCharSkills:
			s.setPage(pageSkills)
			m.CharPage = CharPageSkills
		case msgCharStats:
			s.setPage(pageStats)
			m.CharPage = CharPageStats
		case msgCharInventory:
			s.setPage(pageInventory)
			m.CharPage = CharPageInventory
		case msgCharAwards:
			s.setPage(pageAwards)
			m.CharPage = CharPageAwards
		case msgCharExit:
			s.closed = true
		case msgCharPage:
			s.clickPage(in)
		case msgCharDoll:
			s.clickDoll(in)
		case msgCharSkill:
			if msg.Param < 0 || msg.Param >= party.NumSkills || s.rosterView() {
				continue
			}
			if refuse := m.SpendSkillPoint(s.member, msg.Param, s.ctx); refuse != 0 {
				s.r.Status.Show(s.r.GlobalText(refuse), 2)
				continue
			}
			s.refreshSkills(msg.Param2)
		case msgCharMagnify:
			m.CharMagnify = !m.CharMagnify
		case msgAwardsUp:
			s.awardUp = true
		case msgAwardsDown:
			s.awardDn = true
		case msgAwardsBar:
			s.awardPage = 1
			if in.Y > 0xb2 {
				s.awardPage = -1
			}
		case msgSelectPlayer:
			s.clickPortrait(msg.Param)
		case msgCharDismiss:
			s.dismissMember()
		}
	}
	if in.Right {
		if !s.rightHeld {
			s.rightHeld, s.speakOnce, s.used = true, true, false
		}
	} else if s.rightHeld {
		s.rightHeld, s.used, s.scroll = false, false, 0
	}
	if s.rightHeld {
		s.rightAction(in)
	}
	return s.closed
}

// clickPortrait is Party_ClickPortrait in mode 7: an item on the cursor goes to that
// member's pack; then the screen shows that member.
//
// mm8: 0x4213c0
func (s *charScreen) clickPortrait(slot int) {
	m := s.r.Party
	if slot < 1 || slot > len(m.Players) {
		return
	}
	if m.DropOnPortrait(slot, s.ctx) {
		return
	}
	s.member, s.roster = slot-1, -1
	s.setPage(s.page)
	m.Selected = slot
	s.updateDismiss()
}

// dismissShown reports the dismiss button among the children.
func (s *charScreen) dismissShown() bool {
	for _, w := range s.ct.Children {
		if w == Widget(s.dismiss) && s.dismiss != nil {
			return true
		}
	}
	return false
}

// updateDismiss shows the dismiss button for members 2..5 only and disarms it.
//
// mm8: 0x4cca26 (GuiChar_UpdateDismiss)
func (s *charScreen) updateDismiss() {
	if s.dismiss == nil {
		return
	}
	s.remove([]Widget{s.dismiss})
	if s.r.Party.Selected != 1 {
		s.ct.Add(s.dismiss)
	}
	s.armed = false
}

// dismissMember is the dismiss button: refused in turn-based mode, a first click arms it,
// the second sends the selected member back to the roster (the inn lists it again) and
// shows member 1.
//
// mm8: 0x42f877 (msg 0x1c7: sounds 0x1b and 0xcd are M11's), 0x4cc9fd, 0x4cca15
func (s *charScreen) dismissMember() {
	m := s.r.Party
	if m.Selected == 1 {
		return
	}
	if m.TurnBased {
		s.r.Status.SetHover(s.r.GlobalText(0x2e4))
		return
	}
	if !s.armed {
		s.armed = true
		s.r.Status.SetHover(s.r.GlobalText(0x2e2))
		return
	}
	s.armed = false
	m.RemoveMember(m.Selected - 1)
	m.Selected = 1
	s.member, s.roster = 0, -1
	s.setPage(pageRefresh)
	s.updateDismiss()
}

// clickPage is a click on the page area: on the inventory page, the pack.
//
// mm8: 0x421764 (CharScreen_ClickInventory)
func (s *charScreen) clickPage(in *Input) {
	if s.r.Party.CharPage != CharPageInventory || s.rosterView() {
		return
	}
	hit := s.pick.at(in.X, in.Y)
	cell := -1
	if hit == 0 {
		cell = ((in.Y-0x20)>>5)*items.GridW + (in.X-6)>>5
		if in.X > 0x1c6 || in.X < 5 || cell < 0 {
			return
		}
	}
	s.r.Party.ClickPack(s.member, s.it, hit, cell, s.ctx)
}

// clickDoll is a click on the paper doll.
//
// mm8: 0x468a4b (PaperDoll_Click)
func (s *charScreen) clickDoll(in *Input) {
	m := s.r.Party
	if s.rosterView() {
		return
	}
	sel := m.Selected - 1
	if sel < 0 || sel >= len(m.Players) {
		return
	}
	switch m.ClickDoll(sel, s.pick.at(in.X, in.Y), in.X, false, s.ctx, charNotes{s}) {
	case party.DollUse:
		s.use(sel)
	}
}

// use is Player_UseItem on member i from this screen: a taken potion closes it.
func (s *charScreen) use(i int) {
	res := s.r.Party.UseItem(i, s.ctx, charNotes{s})
	if res.Scroll != 0 {
		s.scroll = res.Scroll
	}
	if res.Close {
		s.closed = true
	}
}

// rightAction is what the held right button does besides the pop-up: with an item on
// the cursor over a portrait, use it on that member; over the pack or the doll, mix.
//
// mm8: 0x41697c (Mouse_RightClick), 0x415c6d (Inventory_RightClick)
func (s *charScreen) rightAction(in *Input) {
	m := s.r.Party
	if s.used {
		return
	}
	if m.MouseItem.Number != 0 {
		for i := range m.Players {
			if in.X >= portraitUseX[i][0] && in.X <= portraitUseX[i][1] && in.Y > 0x184 && in.Y < 0x1d6 {
				s.use(i)
				s.used = true
				return
			}
		}
	}
	if m.MouseItem.Number == 0 || !s.overItems(in) {
		return
	}
	slot := s.itemUnder(in)
	if slot == 0 || s.rosterView() || !m.Players[s.member].CanAct() {
		return
	}
	switch m.MixPotion(s.member, slot, s.speakOnce, s.ctx, charNotes{s}) {
	case party.MixDone, party.MixFailed:
		s.used, s.speakOnce = true, false
	case party.MixExplode:
		s.used, s.speakOnce = true, false
		s.closed = true
	}
}

// portraitUseX are the portraits' x ranges for using an item (0x4f5808, 0x4f5814).
var portraitUseX = [party.MaxMembers][2]int{{19, 81}, {118, 180}, {213, 275}, {308, 370}, {405, 467}}

// overItems reports the mouse over what Inventory_RightClick looks at: the doll, or the
// inventory page's area.
func (s *charScreen) overItems(in *Input) bool {
	if in.X >= 0x1d4 {
		return true
	}
	return in.Y <= 0x158 && s.r.Party.CharPage == CharPageInventory
}

// itemUnder is the item slot under the mouse: the pick buffer, else on the pack the
// cell's item.
//
// mm8: 0x415c6d
func (s *charScreen) itemUnder(in *Input) int32 {
	if n := s.pick.at(in.X, in.Y); n != 0 {
		return n
	}
	if in.X > 5 && in.X < 0x1c6 && s.r.Party.CharPage == CharPageInventory {
		cell := ((in.Y-0x20)>>5)*items.GridW + (in.X-6)>>5
		if cell < 0 || cell > items.PackSlots {
			return 0
		}
		n, _ := s.player().ItemAtCell(cell)
		return n
	}
	return 0
}

// draw draws the screen: topbar2, the page and the doll, the buttons and labels, the
// food and gold. The portraits, the status line and the pop-ups follow in inGame.Draw.
//
// mm8: 0x4cc165 (GuiChar_Draw), 0x41b32b (CharScreen_DrawPage)
func (s *charScreen) draw(c *gfx.Canvas) {
	c.Blit(s.da.topbar2, 0, 0)
	s.pick.clear()
	switch s.r.Party.CharPage {
	case CharPageStats:
		s.drawStats(c)
	case CharPageSkills:
		s.drawSkills(c)
	case CharPageAwards:
		s.drawAwards(c)
	case CharPageInventory:
		s.drawInventory(c)
	}
	if s.r.Party.CharMagnify {
		s.doll.drawJewelry(c, s.player(), &s.pick, s.dt)
	} else {
		s.doll.draw(c, s.player(), &s.pick, false, s.dt)
	}
	s.ct.Draw(c)
	drawGoldFood(c, s.da.scoreBG, s.da.smallnum, s.r.Party)
}

// drawOver draws the right-click pop-up and a message scroll over everything.
func (s *charScreen) drawOver(c *gfx.Canvas, in *Input) {
	if s.scroll != 0 {
		s.drawScroll(c)
	}
	if !s.rightHeld || s.used {
		return
	}
	m := s.r.Party
	if in.X < 0x1d4 {
		if in.Y > 0x158 {
			return
		}
		switch m.CharPage {
		case CharPageStats:
			s.statPopup(c, in)
			return
		case CharPageSkills:
			s.skillPopup(c, in)
			return
		case CharPageInventory:
		default:
			return
		}
	}
	slot := s.itemUnder(in)
	p := s.player()
	if slot == 0 {
		return
	}
	if !p.CanAct() {
		b := popupBox{W: 0x180, H: 0xb4, Y: 0x28, Text: cfmt2(s.r.GlobalText(0x1ab), p.Name, s.r.GlobalText(0x21d))}
		if in.X < 0x141 {
			b.X = in.X + 0x1e
		} else {
			b.X = in.X - 0x1e - b.W
		}
		s.art.draw(c, &b, in.Y)
		return
	}
	info := &itemInfo{t: s.it, m: m, ctx: s.ctx, notes: charNotes{s}, speakOnce: &s.speakOnce}
	s.art.showItem(c, p.Item(slot), in.X, in.Y, info, s.r)
}

// drawScroll is a message scroll's text box at the top left.
//
// mm8: 0x467972 (the scroll window, type 0x1e)
func (s *charScreen) drawScroll(c *gfx.Canvas) {
	body := s.r.ScrollText(int(s.scroll) - 700)
	b := popupBox{X: 1, Y: 1, W: 0x1d4}
	b.H = textHeight(s.art.smallnum, body, b.W, 0) + 0x18 + 2*s.art.create.Height
	if b.H+b.Y > 0x1df {
		b.H = 0x1df - b.Y
	}
	s.art.draw(c, &b, 1)
	r := text.Rect{X: b.X + 0xc, Y: b.Y + 0xc, W: b.W - 0x18, H: b.H - 0xc}
	title := fmt.Sprintf("\f%05d%s\f00000", uint16(inkTitle), s.it.Item(s.scroll).Name)
	s.art.create.DrawCentered(c, r, 0, 1, 0, title, 3)
	s.art.smallnum.Draw(c, r, 1, s.art.create.Height-3, 0, 0, body, 0)
}

// cfmt2 fills the first two "%s" of a C format.
func cfmt2(f, a, b string) string { return cfmt1(cfmt1(f, a), b) }

var fullRect = text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}

// statColor is a value's colour against its base: green above, the default at it, red
// below a quarter, else yellow.
//
// mm8: 0x4175b9 (Stat_Color)
func statColor(v, base int) gfx.Color16 {
	switch {
	case v > base:
		return gfx.RGB16(0, 0xff, 0)
	case v == base:
		return 0
	case base != 0 && v*100/base < 0x19:
		return gfx.RGB16(0xff, 0, 0)
	}
	return gfx.RGB16(0xff, 0xff, 100)
}

// conditionColor is a condition's colour on the stats page.
//
// mm8: 0x4175f9 (Condition_Color)
func conditionColor(c party.Condition) gfx.Color16 {
	switch c {
	case 0, 1, 3, 4, 5, 6, 7:
		return gfx.RGB16(0, 0xff, 0)
	case 2, 8, 9, 0xc, 0xd:
		return gfx.RGB16(0xe1, 0xcd, 0x23)
	case 10, 11, 0xe, 0xf, 0x10:
		return gfx.RGB16(0xff, 0x23, 0)
	}
	return 0xffff
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// drawStats is the stats page.
//
// mm8: 0x418280 (CharScreen_DrawStats)
func (s *charScreen) drawStats(c *gfx.Canvas) {
	p := s.player()
	e := s.r.Party.Env(s.ctx)
	f := s.da.arrus
	g := s.r.GlobalText
	h := f.Height
	c.Blit(s.fr[pageStats], 0, 0x17)
	f.DrawClipped(c, fullRect, 0x10, 0x21, 0, fmt.Sprintf("\f%05d%s", uint16(inkTitle), p.Name), 300)
	pc := gfx.Color16(0xffff)
	if p.SkillPoints != 0 {
		pc = inkGreen
	}
	f.Draw(c, fullRect, 0x10, 0x21, 0, 0, fmt.Sprintf("\f00000\r190%s: \f%05d%d\f00000\n\n\n", g(0xcf), uint16(pc), p.SkillPoints), 0)
	const fmtA = "%s\f%05d\r428%d\f00000 /\t185%d\n"
	const fmtB = "%s\f%05d\r428%d\f00000 /\t185%d\n\n"
	const fmtBig = "%s\f%05d\r388%d\f00000 / %d\n"
	line := func(x, y int, format, name string, v, base int) {
		f.Draw(c, fullRect, x, y, 0, 0, fmt.Sprintf(format, name, uint16(statColor(v, base)), v, base), 0)
	}
	y := 0x44
	for i, st := range []struct {
		s    party.Stat
		name int
	}{{party.StatMight, 0x90}, {party.StatIntellect, 0x74}, {party.StatPersonality, 0xa3}, {party.StatEndurance, 0x4b},
		{party.StatAccuracy, 1}, {party.StatSpeed, 0xd3}, {party.StatLuck, 0x88}} {
		format := fmtA
		if i == 6 {
			format = fmtB
		}
		line(0x10, y, format, g(st.name), p.ActualStat(e, st.s), p.BaseStat(e, st.s))
		y += h - 2
	}
	y += 5 + 2*h - (h - 2)
	maxHP, maxSP := int(p.MaxHP(e)), int(p.MaxSP(e))
	format := fmtA
	if maxHP > 999 {
		format = fmtBig
	}
	line(0x10, y, format, g(0x6c), int(p.HP), maxHP)
	y += h - 2
	format = fmtA
	if maxSP > 999 {
		format = fmtBig
	}
	line(0x10, y, format, g(0xd4), int(p.SP), maxSP)
	y += h - 2
	line(0x10, y, fmtB, g(0xc), p.AC(e), p.BaseAC(e))
	y += 2*h - 2
	cond := p.MainCondition()
	f.DrawClipped(c, fullRect, 0x10, y, 0, fmt.Sprintf("%s: \f%05d%s\n", g(0x2f), uint16(conditionColor(cond)), g(conditionNames[cond])), 0xe2)
	// The quick spell (+0x1c45) is M9's: none.
	f.DrawClipped(c, fullRect, 0x10, y+h-1, 0, fmt.Sprintf("%s: %s", g(0xac), g(0x99)), 0xe2)
	// The right column.
	const fmtAge = "%s\f%05d\t100%d\f00000 / %d\n"
	line(0x108, 0x41, fmtAge, g(5), p.Age(e), p.BaseAge(e))
	format = fmtAge
	if int(p.LevelBase) > 99 {
		format = "%s\f%05d\r180%d\f00000 / %d\n"
	}
	line(0x108, h+0x3f, format, g(0x83), p.Level(e), int(p.LevelBase))
	y = h + 0x3d + h
	exp := g(0x53)
	if p.Exp >= 10000000 {
		exp = g(0x11)
	}
	var tc gfx.Color16
	if p.CanTrain() {
		tc = inkGreen
	}
	f.Draw(c, fullRect, 0x108, y, 0, 0, fmt.Sprintf("%s\r190\f%05d%d\f00000\n\n", exp, uint16(tc), uint32(p.Exp)), 0)
	y += 2 * h
	f.Draw(c, fullRect, 0x108, y, 0, 0, fmt.Sprintf("%s\t100%+d\n", g(0x12), p.MeleeAttack(e, false)), 0)
	y += h - 2
	f.Draw(c, fullRect, 0x108, y, 0, 0, fmt.Sprintf("%s\t100 %s\n", g(0x35), p.MeleeDamageText(e, g)), 0)
	y += h - 2
	f.Draw(c, fullRect, 0x108, y, 0, 0, fmt.Sprintf("%s\t100%+d\n", g(0xcb), p.RangedAttack(e)), 0)
	y += h - 2
	f.Draw(c, fullRect, 0x108, y, 0, 0, fmt.Sprintf("%s\t100 %s\n\n", g(0x35), p.RangedDamageText(e, g)), 0)
	y += 2*h - 1
	for i, rs := range []struct {
		s    party.Stat
		name int
	}{{party.StatFireRes, 0x57}, {party.StatAirRes, 6}, {party.StatWaterRes, 0xf0}, {party.StatEarthRes, 0x46},
		{party.StatMindRes, 0x8e}, {party.StatBodyRes, 0x1d}} {
		if i > 0 {
			y += h - 2
		}
		v, b := p.Resist(e, rs.s), p.BaseResist(e, rs.s)
		wide := iabs(v) > 99 || iabs(b) > 99
		if rs.s == party.StatEarthRes { // the original tests the value twice here
			wide = iabs(v) > 99
		}
		format := "%s\f%05d\t110%d\f00000 / %d\n"
		if wide {
			format = "%s\f%05d\r180%d\f00000 / %d\n"
		}
		t := fmt.Sprintf(format, g(rs.name), uint16(statColor(v, b)), v, b)
		if b == 65000 {
			t = fmt.Sprintf("%s\f%05d\r180%s", g(rs.name), uint16(statColor(v, 65000)), g(0x271))
		}
		f.Draw(c, fullRect, 0x108, y, 0, 0, t, 0)
	}
}

// drawSkills is the skills page: the headers (the labels are widgets).
//
// mm8: 0x41a314 (CharScreen_DrawSkills)
func (s *charScreen) drawSkills(c *gfx.Canvas) {
	p := s.player()
	f, lf := s.da.arrus, s.skillFont
	g := s.r.GlobalText
	c.Blit(s.fr[pageSkills], 0, 0x17)
	f.Draw(c, fullRect, 0x10, 0x22, 0, 0, g(0xce)+" ", 0)
	f.DrawClipped(c, fullRect, 0x6a, 0x22, 0, fmt.Sprintf("\f%05d%s", uint16(inkTitle), p.Name), 0xdc)
	pc := gfx.Color16(0xffff)
	if p.SkillPoints != 0 {
		pc = inkGreen
	}
	f.Draw(c, fullRect, 0x10, 0x22, 0, 0, fmt.Sprintf("\f00000\r190%s: \f%05d%d\f00000", g(0xcf), uint16(pc), p.SkillPoints), 0)
	header := func(x, y int, name int, right int) {
		f.Draw(c, fullRect, x, y, inkTitle, 0, fmt.Sprintf("%s\r%03d%s", g(name), right, g(0x83)), 0)
	}
	group := func(sk []int32) int {
		n := 0
		for _, k := range sk {
			if p.Skills[k]&party.SkillLevel != 0 {
				n++
			}
		}
		return max(n, 1) * (lf.Height - 3)
	}
	y := 2*lf.Height + 0x1c
	header(0x10, y, 0xf2, 408)
	header(0x10, y+group(s.groups[0])-10+2*lf.Height, 0x8a, 408)
	header(0xf1, y, 0xb, 185)
	header(0xf1, y+group(s.groups[2])-10+2*lf.Height, 0x8f, 185)
}

// drawAwards is the awards page: the list from the first shown, coloured by sort, as
// many as fit (the original compares the y with the box's height, not its bottom).
//
// mm8: 0x41a670 (CharScreen_DrawAwards)
func (s *charScreen) drawAwards(c *gfx.Canvas) {
	p := s.player()
	f := s.da.arrus
	c.Blit(s.fr[pageAwards], 0, 0x17)
	f.DrawClipped(c, fullRect, 0x10, 0x22, 0, fmt.Sprintf("%s \f%05d%s", s.r.GlobalText(0x17), uint16(inkTitle), p.Name), 0x1a4)
	n := len(s.awards)
	if s.awardDn && s.drawn+s.awardTop < n {
		s.awardTop++
	}
	if s.awardUp && s.awardTop != 0 {
		s.awardTop--
	}
	if s.awardPage < 0 {
		s.awardTop += s.drawn
		if n < s.drawn+s.awardTop {
			s.awardTop = n - s.drawn
		}
	} else if s.awardPage > 0 {
		if s.awardTop -= s.drawn; s.awardTop < 0 {
			s.awardTop = 0
		}
	}
	s.awardDn, s.awardUp, s.awardPage, s.drawn = false, false, 0, 0
	defs := s.awardDefs()
	m := s.r.Party
	r := text.Rect{X: 0x10, Y: 0x3f, W: 0x19f, H: 0x122}
	for k := s.awardTop; k < n; k++ {
		id := s.awards[k]
		var v uint32
		switch id {
		case 0x2d:
			v = uint32(m.Prison)
		case 0x2b:
			v = uint32(m.Deaths)
		case 0x2c:
			v = uint32(m.Bounty)
		case 0x2e, 0x2f, 0x30, 0x31:
			v = uint32(m.ArenaWins[id-0x2e])
		}
		t := cfmtLu(defs[id].Text, v)
		s.drawn++
		col := s.awardColors[defs[id].Sort%6]
		f.Draw(c, r, 0, 0, gfx.RGB16(col[0], col[1], col[2]), 0, t, 0)
		r.Y += textHeight(f, t, r.W, 0) + 4
		if r.H < r.Y {
			return
		}
	}
}

// cfmtLu fills a C format's first "%lu" (or "%d"/"%u").
func cfmtLu(f string, v uint32) string {
	for _, verb := range []string{"%lu", "%d", "%u"} {
		for i := 0; i+len(verb) <= len(f); i++ {
			if f[i:i+len(verb)] == verb {
				return f[:i] + fmt.Sprint(v) + f[i+len(verb):]
			}
		}
	}
	return f
}

// drawInventory is the inventory page: the pack's items in the 14 × 9 grid from (6, 32),
// a picture narrower than a cell centred in it, broken items red; each writes its slot
// into the pick buffer.
//
// mm8: 0x41a914 (CharScreen_DrawInventory, mode 7)
func (s *charScreen) drawInventory(c *gfx.Canvas) {
	c.Blit(s.fr[pageInventory], 0, 0x17)
	drawPack(c, s.r, s.it, s.player(), s.doll, &s.pick, false, s.dt)
}

// drawPack draws member p's pack items (doll draws them with the sparkle state);
// unidentified items green when green (in houses, g_screenMode 0xd).
func drawPack(c *gfx.Canvas, r *Resources, t *tables.Items, p *party.Player, doll *paperDoll, pick *pickBuffer, green bool, dt int) {
	full := image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH)
	for cell, n := range p.Grid {
		if n <= 0 {
			continue
		}
		it := p.Item(n)
		if it == nil || it.Number == 0 {
			continue
		}
		pic := r.dialogIcon(it.Def(t).Picture)
		x, y := cell%items.GridW*32+6, (cell/items.GridW+1)*32
		if tables.Cells(pic.W) == 1 && pic.W < 32 {
			x += (32 - pic.W) / 2
		}
		doll.drawItem(c, pic, x, y, it, green, false, dt)
		pick.keyed(pic, x, y, n, false, full)
	}
}

// statPopup is the stats page's right-click description: the stat under the mouse.
//
// mm8: 0x417d80 (CharScreen_StatPopup)
func (s *charScreen) statPopup(c *gfx.Canvas, in *Input) {
	k := -1
	for i, r := range s.statRects {
		if hit(r, in.X, in.Y) {
			k = i
			break
		}
	}
	if k < 0 {
		return
	}
	g := s.r.GlobalText
	desc := s.it.StatDesc
	var title, body string
	switch {
	case k <= 6:
		title, body = g(statNameGlobal[k]), desc[k]
	case k == 7:
		title, body = g(0x6c), desc[7]
	case k == 8:
		title, body = g(0xd4), desc[8]
	case k == 9:
		title, body = g(0xc), desc[9]
	case k == 10:
		title, body = g(0x2f), s.conditionText()
	case k == 11:
		title, body = g(0xac), desc[11]
	case k == 12:
		title, body = g(5), desc[12]
	case k == 13:
		title, body = g(0x83), desc[13]
	case k == 14:
		title, body = g(0x53), s.expText()
	case k >= 15 && k <= 18:
		title, body = g(0x24b+k-15), desc[k]
	case k >= 19 && k <= 24:
		title, body = g([]int{0x57, 6, 0xf0, 0x46, 0x8e, 0x1d}[k-19]), desc[k]
	case k == 25:
		title, body = g(0xcf), desc[25]
	}
	if title == "" || body == "" {
		return
	}
	s.art.drawText(c, title, body, in.Y)
}

// conditionText is the condition pop-up: the description, then each condition set in
// priority order with how long ago it began (days and hours).
func (s *charScreen) conditionText() string {
	p := s.player()
	t := s.it.StatDesc[10] + "\n"
	for _, cd := range party.ConditionPriority() {
		began := p.Conditions[cd]
		if began == 0 {
			continue
		}
		t += " \n"
		secs := int64(float64(int64(s.r.Party.Time)-began) * 0.234375)
		hours := secs / 60 / 60
		days, hrs := hours/24, hours%24
		t += fmt.Sprintf("\f%05d%s\f00000 - ", uint16(conditionColor(cd)), s.r.GlobalText(conditionNames[cd]))
		hw := s.r.GlobalText(0x6e)
		if hrs != 0 && hrs < 2 {
			hw = s.r.GlobalText(0x6d)
		}
		dw := s.r.GlobalText(0x39)
		if days == 1 {
			dw = s.r.GlobalText(0x38)
		}
		t += fmt.Sprintf("%d %s, %d %s", uint32(days), dw, uint32(hrs), hw)
	}
	return t
}

// expText is the experience pop-up: the description, the level the member could
// train to, and the experience the next level needs.
func (s *charScreen) expText() string {
	p := s.player()
	lvl := int(p.LevelBase)
	for p.Exp >= 0 && p.Exp >= int64(trainingExp(lvl)) {
		if lvl++; lvl > 10000 {
			break
		}
	}
	t := ""
	if int(p.LevelBase) < lvl {
		t = cfmtLu(s.r.GlobalText(0x93), uint32(lvl))
	}
	next := fmt.Sprintf(cDecimal(s.r.GlobalText(0x21a)), int64(trainingExp(lvl))-p.Exp, lvl+1)
	return fmt.Sprintf("%s\n \n%s", s.it.StatDesc[14], t+"\n"+next)
}

// trainingExp is the experience level lvl needs to train to lvl + 1.
//
// mm8: 0x4b555b (Training_ExpForLevel)
func trainingExp(lvl int) uint32 { return uint32(lvl*(lvl+1)/2) * 1000 }

// cDecimal turns a C format's %lu/%u into Go's %d.
func cDecimal(f string) string {
	out := []byte{}
	for i := 0; i < len(f); i++ {
		if f[i] == '%' && i+2 < len(f) && f[i+1] == 'l' && f[i+2] == 'u' {
			out = append(out, '%', 'd')
			i += 2
			continue
		}
		if f[i] == '%' && i+1 < len(f) && f[i+1] == 'u' {
			out = append(out, '%', 'd')
			i++
			continue
		}
		out = append(out, f[i])
	}
	return string(out)
}

// skillPopup is the skills page's right-click description: the skill points line, or
// the skill under the mouse with its rank texts coloured by what the class reaches.
//
// mm8: 0x417cbe (CharScreen_SkillPopup), 0x417828 (Skill_DescriptionText)
func (s *charScreen) skillPopup(c *gfx.Canvas, in *Input) {
	if in.X >= 0x18 && in.X <= 0x1c7 && in.Y >= 0x21 && in.Y <= 0x33 {
		s.art.drawText(c, s.r.GlobalText(0xcf), s.it.StatDesc[25], in.Y)
		return
	}
	for _, r := range s.rows {
		if r.skill < 0 || r.name.Text == s.r.GlobalText(0x99) || !hit(r.name.Rect(), in.X, in.Y) {
			continue
		}
		s.art.drawText(c, s.r.GlobalText(tables.SkillNameGlobal[r.skill]), s.skillText(r.skill), in.Y)
		return
	}
}

// skillText is a skill's description: the text, then the four ranks' texts after their
// names (tabbed past the widest name), each coloured by Skill_RankColor; with items
// raising it, the bonus in white.
//
// mm8: 0x417828 (Skill_DescriptionText)
func (s *charScreen) skillText(skill int) string {
	p := s.player()
	g := s.r.GlobalText
	sm := s.art.smallnum
	w := max(sm.TextWidth(g(0x1af)), sm.TextWidth(g(0x1b1)), sm.TextWidth(g(0x1b0)), sm.TextWidth(g(0x60)))
	desc := s.it.SkillDesc[skill]
	ranks := [4]int{0x1af, 0x1b1, 0x1b0, 0x60}
	t := desc[0] + "\n\n"
	cls := s.r.classes
	for k := 0; k < 4; k++ {
		col := gfx.RGB16(0xff, 0xff, 0xff)
		if cls != nil {
			if _, yellow, red := cls.RankColor(p.Class, skill, k+1); red {
				col = gfx.RGB16(0xff, 0, 0)
			} else if yellow {
				col = gfx.RGB16(0xff, 0xff, 0)
			}
		}
		t += fmt.Sprintf("\f%05d%s\t%03d:\t%03d%s\t000\n", uint16(col), g(ranks[k]), w+3, w+10, desc[k+1])
	}
	e := s.r.Party.Env(s.ctx)
	if bonus := int(p.Skill(e, skill)&party.SkillLevel) - int(p.Skills[skill]&party.SkillLevel); bonus != 0 {
		t += fmt.Sprintf("\n\f%05d%s: %+d", uint16(gfx.RGB16(0xff, 0xff, 0xff)), g(0x26f), bonus)
	}
	return t
}
