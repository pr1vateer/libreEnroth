package ui

import (
	"fmt"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Party creation messages (Menu_ProcessMessages 0x433bbd).
const (
	msgStatPlus     = 0x3e
	msgStatMinus    = 0x3f
	msgChooseSkill  = 0x40 // param: choosable skill 0..8
	msgDropSkill2   = 0x1cd
	msgDropSkill3   = 0x1ce
	msgCreateOK     = 0x42
	msgCreateClear  = 0x43
	msgVoiceNext    = 0x90
	msgVoicePrev    = 0x91
	msgFaceNext     = 0xab
	msgFacePrev     = 0xac
	msgDefaultVoice = 0x1ee
	msgCancel       = 0x71
)

// Tables compiled into MM8-Rel.exe, read at run time (re/notes/ui.md#party-creation).
const (
	vaPortraitFmt   = 0x500db0 // "%s01"
	vaPortraitNames = 0x4feb88 // "pc01-".."pc30-"
	vaBackDollNames = 0x4feb10
	vaBodyNames     = 0x4fede0
	vaLeftHandNames = 0x4fec00
	vaRightHandName = 0x4feed0
	vaDollBase      = 0x4f7858 // {467, 23}
	vaBodyOffsets   = 0x4f8120 // [face]{x, y}
	vaLeftHandOffs  = 0x4f8300 // [pose]{x, y}
	vaRightHandOffs = 0x4f83a0 // [pose]{x, y, ?, ?}
	numFaces        = 30
	selectableFaces = 24 // face wraps at 0x17; the dragons (24, 25) are not offered
	numClasses      = 16
	numSkills       = 39
	skillNone       = 39
)

// Stat names: global.txt indices of Might, Intellect, Personality, Endurance, Accuracy,
// Speed, Luck, as the build function references them (0x6015a0, 0x601530, ...).
//
// mm8: 0x4c7a10 (GuiPartyCreate_Build)
var statNameGlobal = [7]int{144, 116, 163, 75, 1, 211, 136}

// Skill names: global.txt index per skill id, as Txt_LoadGlobal copies them into the
// skill name table (0xbb2f58); id 39 is "None".
//
// mm8: 0x45181f (Txt_LoadGlobal)
var skillNameGlobal = [numSkills + 1]int{
	0x10f, 0x110, 0x111, 0x112, 0x113, 0x114, 0x115, 0x116, 0x117, 0x118,
	0x119, 0x11a, 0x11b, 0x11c, 0x11d, 0x11e, 0x121, 0x122, 0x123, 0x11f,
	0x120, 0x2b5, 0x2b6, 0x2b7, 0x124, 0x125, 0x126, 0x127, 0x128, 0x129,
	0xea, 300, 0x32, 0x4d, 0x58, 0x59, 0x5a, 0x5f, 0x12d, 0x99,
}

// Class names are global.txt 0x2a5 + class id (Txt_LoadGlobal 0x45181f).
const classNameGlobal = 0x2a5

// ClassForFace maps a portrait to its class: faces come in groups per race.
//
// mm8: 0x49119b (Player_ClassForFace)
func ClassForFace(face int) int {
	switch {
	case face >= 4 && face <= 7:
		return 2 // Cleric
	case face >= 8 && face <= 11:
		return 0 // Necromancer
	case face >= 12 && face <= 15:
		return 12 // Vampire
	case face >= 16 && face <= 19:
		return 10 // Dark Elf
	case face == 20 || face == 21:
		return 8 // Minotaur
	case face == 22 || face == 23:
		return 6 // Troll
	case face == 24 || face == 25:
		return 14 // Dragon
	case face == 26 || face == 27:
		return 1
	}
	return 4 // Knight
}

// IsFemale reports whether face (or a voice id) is female.
//
// mm8: 0x491217 (Player_IsFemale)
func IsFemale(face int) bool { return party.IsFemale(face) }

type partyCreate struct {
	r        *Resources
	ct       Container
	bg       *gfx.Sprite
	selring  *gfx.Sprite
	portrait [numFaces]*gfx.Sprite
	backDoll [numFaces]*gfx.Sprite
	body     [numFaces]*gfx.Sprite
	lhand    [numFaces]*gfx.Sprite
	rhand    [numFaces]*gfx.Sprite

	dollBase  [2]int32
	bodyOff   []int32
	lhandOff  []int32
	rhandOff  []int32
	cls       *tables.Classes
	statNames [7]string
	skillName [numSkills + 1]string
	className [numClasses]string

	// hero is the character being made (the original edits g_players[0] in place).
	hero party.Player

	name                    *Edit
	classLbl, pointsLbl     *Label
	statLbl, statValLbl     [7]*Label
	classSkillLbl, extraLbl [2]*Label
	choiceLbl               [9]*Label
}

// newPartyCreate builds the creation screen.
//
// mm8: 0x4c7a10 (GuiPartyCreate_Build)
func newPartyCreate(r *Resources) (*partyCreate, error) {
	l := &loader{r: r}
	p := &partyCreate{r: r, bg: l.pcx("makeme.pcx"), selring: l.icon("selring", false)}

	// Buttons, in Build order (which is also the event order).
	add := func(b *Button, hotkey Key) {
		b.Hotkey = hotkey
		p.ct.Add(b)
	}
	add(l.button(373, 440, Msg{ID: msgCreateClear}, "c_clr_up", "c_clr_dn", "c_clr_ht", true), 'C')
	add(l.button(459, 440, Msg{ID: msgCancel}, "c_cncl_up", "c_cncl_dn", "c_cncl_ht", true), KeyEscape)
	add(l.button(546, 440, Msg{ID: msgCreateOK}, "c_ok_up", "c_ok_dn", "c_ok_ht", true), KeyEnter)
	add(l.button(69, 165, Msg{ID: msgFacePrev}, "cc_up_l", "cc_dn_l", "cc_ht_l", true), 0)
	add(l.button(161, 165, Msg{ID: msgFaceNext}, "cc_up_r", "cc_dn_r", "cc_ht_r", false), 0)
	add(l.button(69, 189, Msg{ID: msgVoicePrev}, "cc_up_l", "cc_dn_l", "cc_ht_l", false), 0)
	add(l.button(161, 189, Msg{ID: msgVoiceNext}, "cc_up_r", "cc_dn_r", "cc_ht_r", false), 0)
	add(l.button(93, 214, Msg{ID: msgDefaultVoice}, "c_dft_up", "c_dft_dn", "c_dft_ht", true), 0)

	// Tables and per-face art.
	pfmt := l.exeString(vaPortraitFmt) // "%s01"
	prefixes := l.exeStrings(vaPortraitNames, numFaces)
	back := l.exeStrings(vaBackDollNames, numFaces)
	body := l.exeStrings(vaBodyNames, numFaces)
	lh := l.exeStrings(vaLeftHandNames, numFaces)
	rh := l.exeStrings(vaRightHandName, numFaces)
	// Only the selectable faces have complete paper-doll art (no pc25LHd, ...).
	for i := 0; i < selectableFaces; i++ {
		p.portrait[i] = l.icon(strings.Replace(pfmt, "%s", prefixes[i], 1), false)
		p.backDoll[i] = l.icon(back[i], false)
		p.body[i] = l.icon(body[i], false)
		p.lhand[i] = l.icon(lh[i], false)
		p.rhand[i] = l.icon(rh[i], false)
	}
	base := l.exeInts(vaDollBase, 2)
	p.dollBase = [2]int32{base[0], base[1]}
	p.bodyOff = l.exeInts(vaBodyOffsets, 2*numFaces)
	p.lhandOff = l.exeInts(vaLeftHandOffs, 2*5)
	p.rhandOff = l.exeInts(vaRightHandOffs, 4*5)
	p.cls = l.classes()
	for i, g := range statNameGlobal {
		p.statNames[i] = l.global(g)
	}
	for i, g := range skillNameGlobal {
		p.skillName[i] = l.global(g)
	}
	for i := range p.className {
		p.className[i] = l.global(classNameGlobal + i)
	}

	// Labels, all in create.fnt; h is its height.
	f := l.font("create.fnt")
	if l.err != nil {
		return nil, l.err
	}
	h := f.Height
	p.name = NewEdit(text.Rect{X: 70, Y: 77, W: 120, H: h}, f, "", 31)
	p.ct.Add(p.name)
	p.classLbl = NewLabel(text.Rect{X: 70, Y: 121, W: 120, H: h}, f, "")
	p.ct.Add(p.classLbl)
	p.pointsLbl = NewLabel(text.Rect{X: 152, Y: 261, W: 40, H: h}, f, "")
	p.pointsLbl.Align = AlignCenter
	p.ct.Add(p.pointsLbl)
	for i := 0; i < 7; i++ {
		p.statLbl[i] = NewLabel(text.Rect{X: 4, Y: 296 + h*i, W: 90, H: h}, f, p.statNames[i])
		p.ct.Add(p.statLbl[i])
		p.statValLbl[i] = NewLabel(text.Rect{X: 128, Y: 298 + h*i, W: 35, H: h}, f, "")
		p.statValLbl[i].Align = AlignCenter
		p.ct.Add(p.statValLbl[i])
		p.ct.Add(l.button(105, 298+h*i, Msg{ID: msgStatMinus, Param: i}, "cminup", "cmindn", "cminht", false))
		p.ct.Add(l.button(167, 298+h*i, Msg{ID: msgStatPlus, Param: i}, "cplusup", "cplusdn", "cplusht", false))
	}
	// Class skills (slots 0, 1) and the two extra skills (slots 2, 3).
	for i := 0; i < 2; i++ {
		y := 106 + 36*i
		left := NewLabel(text.Rect{X: 226, Y: y, W: 100, H: 28}, f, "")
		right := NewLabel(text.Rect{X: 330, Y: y, W: 100, H: 28}, f, "")
		for _, lb := range []*Label{left, right} {
			lb.Align = AlignCenter
			if i == 0 {
				lb.Color = gfx.RGB16(0xff, 0xff, 0)
			} else {
				lb.Color = gfx.RGB16(0, 200, 0xff)
			}
			p.ct.Add(lb)
		}
		if i == 0 {
			p.classSkillLbl = [2]*Label{left, right}
		} else {
			left.Msg, right.Msg = Msg{ID: msgDropSkill2}, Msg{ID: msgDropSkill3}
			p.extraLbl = [2]*Label{left, right}
		}
	}
	// The nine choosable skills: four rows of two, then one centred.
	for k := 4; k <= 12; k++ {
		var rct text.Rect
		switch {
		case k == 12:
			rct = text.Rect{X: 274, Y: 370, W: 100, H: 28}
		case k%2 == 0:
			rct = text.Rect{X: 228, Y: 234 + 34*((k-4)/2), W: 100, H: 28}
		default:
			rct = text.Rect{X: 328, Y: 234 + 34*((k-4)/2), W: 100, H: 28}
		}
		lb := NewLabel(rct, f, "")
		lb.Align = AlignCenter
		lb.Msg = Msg{ID: msgChooseSkill, Param: k - 4}
		p.choiceLbl[k-4] = lb
		p.ct.Add(lb)
	}
	p.setFace(0)
	return p, nil
}

func (p *partyCreate) class() int { return p.hero.Class }

// setFace selects a portrait: class, voice and the stats follow it.
//
// mm8: 0x433bbd (Menu_ProcessMessages 0xab/0xac -> 0x492094)
func (p *partyCreate) setFace(face int) {
	p.hero.Face, p.hero.Voice = face, face
	p.hero.Class = ClassForFace(face)
	p.reset()
}

// reset puts the stats and skills back to the class's.
//
// mm8: 0x433bbd (msg 0x43 -> 0x492094)
func (p *partyCreate) reset() {
	p.hero.ResetCreation(p.cls)
	p.refresh()
}

// CreationSkill returns the skill shown in creation slot n (party.Player.CreationSkill).
func (p *partyCreate) CreationSkill(n int) int { return p.hero.CreationSkill(p.cls, n) }

// PointsLeft is the bonus pool (party.Player.PointsLeft).
func (p *partyCreate) PointsLeft() int { return p.hero.PointsLeft(p.cls) }

// stat is the creation row i's value (rows in stat code order).
func (p *partyCreate) stat(i int) int { return p.hero.Base(party.Stat(i)) }

// refresh updates every label from the model.
//
// mm8: 0x4c7a10 (GuiPartyCreate_Build label setup), 0x4c94ba (refresh after changes)
func (p *partyCreate) refresh() {
	p.classLbl.Text = p.className[p.class()]
	p.pointsLbl.Text = fmt.Sprintf("%d", p.PointsLeft())
	for i := 0; i < 7; i++ {
		e := p.cls.Stats[p.class()][i]
		switch {
		case e.UpCost == 2:
			p.statLbl[i].Color = gfx.RGB16(0xff, 0, 0)
		case e.UpStep == 2:
			p.statLbl[i].Color = gfx.RGB16(0, 0xff, 0)
		default:
			p.statLbl[i].Color = gfx.RGB16(0xff, 0xff, 0xff)
		}
		p.statValLbl[i].Text = fmt.Sprintf("%d", p.stat(i))
	}
	for i := 0; i < 2; i++ {
		p.classSkillLbl[i].Text = p.skillName[p.CreationSkill(i)]
		p.extraLbl[i].Text = p.skillName[p.CreationSkill(2+i)]
	}
	for k := 4; k <= 12; k++ {
		s := p.CreationSkill(k)
		lb := p.choiceLbl[k-4]
		lb.Text = p.skillName[s]
		lb.Color = gfx.RGB16(0xff, 0xff, 0xff)
		if s != skillNone && (s == p.CreationSkill(2) || s == p.CreationSkill(3)) {
			lb.Color = gfx.RGB16(0, 200, 0xff)
		}
	}
}

// Name is the typed character name.
func (p *partyCreate) Name() string { return p.name.Text }

// Face is the selected portrait.
func (p *partyCreate) Face() int { return p.hero.Face }

// Hero is the character being made.
func (p *partyCreate) Hero() *party.Player { return &p.hero }

// done reports the character complete: every point spent and two skills chosen. (The
// original shows global.txt 412 for 4 s in a pop-up box otherwise; that box comes with
// M7b's pop-ups.)
//
// mm8: 0x433bbd (msg 0x42: PartyCreate_PointsLeft() == 0 && HasTwoExtraSkills())
func (p *partyCreate) done() bool {
	return p.PointsLeft() == 0 && p.hero.HasTwoExtraSkills()
}

func (p *partyCreate) Update(in *Input) Transition {
	p.name.Type(in)
	p.ct.Update(in)
	for _, m := range p.ct.Queue.Drain() {
		switch m.ID {
		case msgFaceNext:
			p.setFace((p.hero.Face + 1) % selectableFaces)
		case msgFacePrev:
			p.setFace((p.hero.Face + selectableFaces - 1) % selectableFaces)
		case msgVoiceNext, msgVoicePrev:
			// Cycle through voices of the portrait's sex (heard in M11). The right
			// arrow (0x90) steps down, the left one (0x91) up.
			step := 1
			if m.ID == msgVoiceNext {
				step = 23
			}
			for {
				p.hero.Voice = (p.hero.Voice + step) % 24
				if IsFemale(p.hero.Voice) == IsFemale(p.hero.Face) {
					break
				}
			}
		case msgDefaultVoice:
			p.hero.Voice = p.hero.Face
		case msgStatPlus:
			p.hero.StatUp(p.cls, party.Stat(m.Param)) // the error sound 0x1b on failure (M11)
			p.refresh()
		case msgStatMinus:
			p.hero.StatDown(p.cls, party.Stat(m.Param))
			p.refresh()
		case msgChooseSkill:
			p.hero.ChooseSkill(p.cls, m.Param)
			p.refresh()
		case msgDropSkill2, msgDropSkill3:
			p.hero.DropSkill(p.cls, 2+m.ID-msgDropSkill2)
			p.refresh()
		case msgCreateClear:
			p.reset()
		case msgCreateOK:
			if !p.done() {
				continue
			}
			// The hero becomes member 1 (level 1, as roster.txt's row 0 has it); any
			// further members come from -party. PartyCreation_Run's ending gives the
			// starting items.
			hero := p.hero
			hero.Name, hero.LevelBase, hero.RosterID, hero.Expr = p.Name(), 1, 0, party.ExprNormal
			m := p.r.Party
			if len(m.Players) == 0 {
				m.Players = []party.Player{hero}
			} else {
				m.Players[0] = hero
			}
			ctx, err := p.r.Ctx()
			if err != nil {
				return Transition{err: err}
			}
			m.FinishCreation(ctx)
			return goTo(StateInGame)
		case msgCancel:
			return goTo(StateTitle)
		}
	}
	return Transition{}
}

// Draw draws background, portrait, widgets and the paper doll, in the original order.
//
// mm8: 0x4c7726 (GuiPartyCreate_Draw)
func (p *partyCreate) Draw(c *gfx.Canvas) {
	c.Blit(p.bg, 0, 0)
	c.Blit(p.portrait[p.hero.Face], 4, 162)
	c.BlitKeyed(p.selring, 4, 162)
	p.ct.Draw(c)

	c.Blit(p.backDoll[p.hero.Face], 454, 54)
	bx, by := int(p.dollBase[0])-13, int(p.dollBase[1])+31
	c.BlitKeyed(p.body[p.hero.Face], int(p.bodyOff[2*p.hero.Face])+bx, int(p.bodyOff[2*p.hero.Face+1])+by)
	pose := 0
	switch p.hero.Face {
	case 22, 23:
		pose = 3
	case 20, 21:
		pose = 2
	case 24, 25:
		pose = 4
	default:
		if IsFemale(p.hero.Face) {
			pose = 1
		}
	}
	c.BlitKeyed(p.lhand[p.hero.Face], int(p.lhandOff[2*pose])+bx, int(p.lhandOff[2*pose+1])+by)
	c.BlitKeyed(p.rhand[p.hero.Face], int(p.rhandOff[4*pose])+bx, int(p.rhandOff[4*pose+1])+by)
}
