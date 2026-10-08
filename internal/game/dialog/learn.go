package dialog

import (
	"fmt"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// Learning skills in the houses, the teachers' lessons, training to the next level and
// the return of lost quest items (re/notes/houses.md#skills-m7d).

// Learn Skills buttons: the code of skill s is LearnCode0 + s (msg 0x195 in menu 0x60).
// The dark elf, vampire and dragon skills (codes 0x39..0x3b) are never taught.
const (
	LearnCode0    = 0x24
	learnCodeLast = 0x4a
	maxLearnSkill = 5 // House_LearnSkillsMenu's list
)

// learnSkillsByType are the skills a house type teaches (the shops' depend on their
// stock records instead).
//
// mm8: 0x4b43ce
var learnSkillsByType = map[int][]int{
	TypeMagic:    {party.SkillIDItem, party.SkillRepair},
	TypeAlchemy:  {party.SkillAlchemy, party.SkillIDMonster},
	0xc:          {party.SkillFire + 7, party.SkillMeditation}, // light
	0xd:          {party.SkillFire + 8, party.SkillMeditation}, // dark
	0xe:          {party.SkillFire, party.SkillFire + 1, party.SkillFire + 3, party.SkillFire + 2, party.SkillLearning},
	0xf:          {party.SkillFire + 6, party.SkillFire + 5, party.SkillFire + 4, party.SkillMeditation},
	TypeTavern:   {party.SkillStealing, party.SkillDisarm, party.SkillPerception},
	TypeTemple:   {party.SkillRegeneration, party.SkillMerchant},
	TypeTraining: {party.SkillArmsmaster, party.SkillBodybuilding},
}

// weaponKindSkill and armourKindSkill are the skills a weapon or armour shop teaches
// for the item kinds of its stock records.
var (
	weaponKindSkill = map[int]int{0x17: party.SkillSword, 0x18: party.SkillDagger, 0x19: party.SkillAxe,
		0x1a: party.SkillSpear, 0x1b: party.SkillBow, 0x1c: party.SkillMace, 0x1e: party.SkillStaff}
	armourKindSkill = map[int]int{0x1f: party.SkillLeather, 0x20: party.SkillChain, 0x21: party.SkillPlate,
		0x22: party.SkillShield}
)

// LearnSkills are the skills the house teaches, in button order: a type's own, or for a
// weapon shop the skills of the item kinds its standard and special records stock, for
// an armour shop those of its four records (five at most, each once).
//
// mm8: 0x4b43ce (House_LearnSkillsMenu, with 0x4be643 adding each kind once)
func (d *Dialog) LearnSkills() []int {
	if s, ok := learnSkillsByType[d.Type]; ok {
		return s
	}
	t := d.h.Tables().Shops
	if t == nil {
		return nil
	}
	var out []int
	add := func(kinds [4]int, skill map[int]int) {
		for _, k := range kinds {
			s, ok := skill[k]
			if !ok || len(out) >= maxLearnSkill {
				continue
			}
			dup := false
			for _, o := range out {
				dup = dup || o == s
			}
			if !dup {
				out = append(out, s)
			}
		}
	}
	switch d.Type {
	case TypeWeapons:
		add(t.StdWeapons(d.House).Kinds, weaponKindSkill)
		add(t.SpcWeapons(d.House).Kinds, weaponKindSkill)
	case TypeArmour:
		for _, shelf := range []func(int, int) tables.Shelf{t.StdArmour, t.SpcArmour} {
			for row := range 2 {
				add(shelf(d.House, row).Kinds, armourKindSkill)
			}
		}
	}
	return out
}

// learnMenu is the Learn Skills sub-menu: a button per skill the house teaches.
//
// mm8: 0x4b43ce
func (d *Dialog) learnMenu() []Button {
	var out []Button
	for _, s := range d.LearnSkills() {
		out = append(out, Button{MsgService, LearnCode0 + s})
	}
	return out
}

// LearnPrice is what a skill costs here: the house's A column times 500,
// merchant-adjusted.
//
// mm8: 0x4bd028 (0x4bd50d), 0x4b5618 (0x4b5720) and the other draws of menu 0x60
func (d *Dialog) LearnPrice() int {
	return d.merchant(int(float64(d.Def.A) * float64(float32(500))))
}

// CanLearn reports p able to learn skill: its class has the skill and p does not.
//
// mm8: 0x4bd028, 0x4b5618 (g_classSkillMax[class][skill] != 0, skills[skill] == 0)
func (d *Dialog) CanLearn(p *party.Player, skill int) bool {
	c := d.h.Ctx().Classes
	if p == nil || c == nil || p.Class < 0 || p.Class >= tables.NumClasses || skill < 0 || skill >= party.NumSkills {
		return false
	}
	return c.SkillMax[p.Class][skill] != 0 && p.Skills[skill] == 0
}

// learn is a Learn Skills button: the selected member pays and knows the skill at level
// 1 (speech 0x4e). Without the gold, "You don't have enough gold" (and the house's
// sound 4 in training halls and taverns, else 2: M11).
//
// mm8: 0x4bd028 (codes 0x24..0x38, 0x3c..0x4a)
func (d *Dialog) learn(code int) {
	if code < LearnCode0 || code > learnCodeLast || code >= LearnCode0+party.SkillDarkElf && code <= LearnCode0+party.SkillDragon {
		return
	}
	p, skill := d.selected(), code-LearnCode0
	if !d.CanLearn(p, skill) {
		return
	}
	if !d.pay(d.LearnPrice()) {
		return
	}
	d.Traded = true
	p.Skills[skill] = 1
	d.speak(0x4e)
}

// ---- teachers ------------------------------------------------------------------------

// Lesson is what a teacher's topic offers the selected member: the Learn button's label
// (the offer, or why there is none) and, when OK, the skill, the rank and the price.
type Lesson struct {
	Label              string
	OK                 bool
	Skill, Rank, Price int
}

// teacherSkill is the skill of teacher topic ev (300..0x1a0, three per skill: expert,
// master, grandmaster). The blaster's, dodging's, unarmed's and stealing's topics are
// missing from the original's switch, which reports "Crash in
// getSkillIndexFromEvent()" and answers 0.
//
// mm8: 0x4b301c (Teacher_SkillOfTopic)
func teacherSkill(ev int) (skill int, ok bool) {
	s := (ev - 300) / 3
	switch s {
	case party.SkillBlaster, party.SkillDodge, party.SkillUnarmed, party.SkillStealing:
		return 0, false
	}
	return s, s >= 0 && s < party.NumSkills
}

// Lesson works out teacher topic ev's lesson for the selected member: rank k + 2 of
// the skill for 2000, 5000 or 8000 gold (k = (ev - 300) % 3), less for some skills (the
// blaster's grandmaster is free). The class must reach the rank; the member must be able
// to act, know the skill one rank below with level 4, 7 or 10 (a master in
// bodybuilding, merchant or learning also needs 50 base endurance, personality or
// intellect) and have the gold.
//
// mm8: 0x4b31fe (Teacher_Offer)
func (d *Dialog) Lesson(ev int) Lesson {
	p := d.selected()
	c := d.h.Ctx().Classes
	text := d.TopicText
	if p == nil || c == nil || p.Class < 0 || p.Class >= tables.NumClasses {
		return Lesson{}
	}
	skill, ok := teacherSkill(ev)
	if !ok {
		d.h.Note("Crash in getSkillIndexFromEvent() (0x4b301c)")
	}
	k := (ev - 300) % 3
	l := Lesson{Skill: skill, Rank: k + 2, Price: [3]int{2000, 5000, 8000}[k]}
	g := d.h.Global
	if int(c.SkillMax[p.Class][skill]) < l.Rank {
		base := p.Class - p.Class%2
		if base+1 < tables.NumClasses && int(c.SkillMax[base+1][skill]) >= l.Rank {
			l.Label = cfmt(g(0x279), g(tables.ClassNameGlobal+base+1))
		} else {
			l.Label = cfmt(g(0x278), g(tables.ClassNameGlobal+p.Class))
		}
		return l
	}
	raw := p.Skills[skill]
	level := int(raw & party.SkillLevel)
	switch {
	case !p.CanAct():
		l.Label = text(123)
		return l
	case level == 0:
		l.Label = text(132)
		return l
	case party.Mastery(raw) > k+1:
		l.Label = text(129 + k)
		return l
	}
	e := d.h.Members().Env(d.h.Ctx())
	notReady := false
	switch l.Rank {
	case 2:
		notReady = level < 4
		switch {
		case skill < party.SkillShield, skill == party.SkillMerchant:
		case skill <= party.SkillDragon:
			l.Price = 1000
		case skill == party.SkillIDItem, skill >= party.SkillRepair && skill <= party.SkillDisarm,
			skill == party.SkillIDMonster, skill == party.SkillAlchemy:
			l.Price = 500
		}
	case 3:
		notReady = party.Mastery(raw) < 2 || level < 7
		switch {
		case skill < party.SkillShield:
		case skill <= party.SkillPlate:
			l.Price = 3000
		case skill <= party.SkillDragon:
			l.Price = 4000
		case skill == party.SkillIDItem, skill == party.SkillRepair, skill == party.SkillMeditation,
			skill == party.SkillPerception, skill == party.SkillDisarm, skill == party.SkillIDMonster,
			skill == party.SkillAlchemy:
			l.Price = 2500
		case skill == party.SkillBodybuilding:
			l.Price = 2500
			notReady = notReady || p.BaseStat(e, party.StatEndurance) < 50
		case skill == party.SkillMerchant:
			notReady = notReady || p.BaseStat(e, party.StatPersonality) < 50
		case skill == party.SkillLearning:
			notReady = notReady || p.BaseStat(e, party.StatIntellect) < 50
		}
	case 4:
		notReady = party.Mastery(raw) < 3 || level < 10
		switch {
		case skill == party.SkillBlaster:
			l.Price = 0
		case skill >= party.SkillShield && skill <= party.SkillPlate:
			l.Price = 7000
		case skill == party.SkillIDItem, skill >= party.SkillRepair && skill <= party.SkillPerception,
			skill == party.SkillDisarm, skill == party.SkillIDMonster, skill == party.SkillStealing,
			skill == party.SkillAlchemy:
			l.Price = 6000
		}
	}
	if notReady {
		l.Label = text(128)
		return l
	}
	if l.Price != 0 && uint32(l.Price) > uint32(d.h.Members().Gold) {
		l.Label = text(125)
		return l
	}
	rank := map[int]int{2: 0x1b1, 3: 0x1b0, 4: 0xe1}[l.Rank]
	l.OK = true
	l.Label = cfmt(g(0x216), g(rank), g(tables.SkillNameGlobal[skill]), l.Price)
	return l
}

// lessonRanks are the rank bits a lesson sets (the level's six bits stay); the dark elf,
// vampire and dragon skills also give their racial spell of that rank.
var (
	lessonRanks  = map[int]uint16{2: party.SkillExpert, 3: party.SkillMaster, 4: party.SkillGM}
	lessonSpells = map[int]int{party.SkillDarkElf: 100, party.SkillVampire: 0x6f, party.SkillDragon: 0x7a}
)

// takeLesson is the Learn button of a teacher's topic (msg 0x4f): when the lesson is
// possible the party pays, the selected member's skill takes the new rank and the member
// is pleased (speech 0x55); then back to the topics.
//
// mm8: 0x4b2b74 (param 0x4f)
func (d *Dialog) takeLesson() {
	if d.Kind != KindHouse {
		return // an NPC's own dialogue has no Learn action (0x4bcbf2)
	}
	l := d.Lesson(d.teacherEvent)
	if !l.OK {
		return
	}
	m := d.h.Members()
	m.TakeGold(uint32(l.Price))
	if p := d.selected(); p != nil {
		p.Skills[l.Skill] = p.Skills[l.Skill]&party.SkillLevel | lessonRanks[l.Rank]
		if sp, ok := lessonSpells[l.Skill]; ok {
			p.Spells[sp+l.Rank-2] = true
		}
		d.speak(0x55)
	}
	d.post()
}

// ---- training ------------------------------------------------------------------------

// ExpForLevel is the experience level needs to train to level + 1.
//
// mm8: 0x4b555b (Training_ExpForLevel)
func ExpForLevel(level int) int64 { return int64(uint32(level*(level+1)/2) * 1000) }

// LevelCap is the level this house trains to (0 for none).
//
// mm8: 0x502c4a (g_trainingLevelCap[house])
func (d *Dialog) LevelCap() int {
	if l := d.h.Tables().Learning; l != nil {
		return l.LevelCap(d.House)
	}
	return 0
}

// TrainingPrice is the price of p's next level here, when p has the experience and is
// below the hall's cap: level · Val, twice that for a promoted class, merchant-adjusted.
//
// mm8: 0x4b5573 (Training_Price)
func (d *Dialog) TrainingPrice(p *party.Player) (price int, ok bool) {
	lvl := int(p.LevelBase)
	if p.Exp < ExpForLevel(lvl) || lvl >= d.LevelCap() {
		return 0, false
	}
	f := float64(lvl) * float64(d.val()) * float64(p.Class%2+1)
	return d.merchant(int(f)), true
}

// TrainingLabel is the Train button's text: the offer, the experience still needed, or
// (at the cap) "you should be working here as a teacher".
//
// mm8: 0x4b5618 (menu 1)
func (d *Dialog) TrainingLabel(p *party.Player) string {
	g := d.h.Global
	lvl := int(p.LevelBase)
	if lvl >= d.LevelCap() {
		return g(0x218) + "\n \n" + g(0x211)
	}
	if need := ExpForLevel(lvl); p.Exp < need {
		return cfmt(g(0x21a), int32(need-p.Exp), lvl+1)
	}
	price, _ := d.TrainingPrice(p)
	return cfmt(g(0x219), lvl+1, int(int16(price)))
}

// canTrain is the main menu's check on the Train button: it opens only when the member
// has the experience and is below the cap.
//
// mm8: 0x4bd028 (menu 1, house type 0x1e)
func (d *Dialog) canTrain() bool {
	p := d.selected()
	if p == nil {
		return false
	}
	_, ok := d.TrainingPrice(p)
	return ok
}

// train is the Train menu: the selected member pays (or the house's sound 4, M11) and
// rises a level, gaining level / 10 + 5 skill points and full hit and spell points
// (speech 0x57 and "%s is now Level %lu and has earned %lu Skill Points!"). A level
// that takes the member's count of levels trained this visit beyond every member's
// count so far lets a week pass: the clock runs to 9 AM eight days on (the hours to 5 AM
// plus 172), outdoors with the weather rolled again. Then back to the menu.
//
// mm8: 0x4b5618 (menu 0x11)
func (d *Dialog) train() {
	defer d.post()
	p := d.selected()
	if p == nil {
		return
	}
	m := d.h.Members()
	if p.Exp < ExpForLevel(int(p.LevelBase)) {
		d.h.Note("the training hall's sound 3 (M11)")
		return
	}
	price, _ := d.TrainingPrice(p)
	if !d.pay(price) {
		d.h.Note("the training hall's sound 4 (M11)")
		return
	}
	p.LevelBase++
	gain := int32(p.LevelBase/10 + 5)
	p.SkillPoints += gain
	h := d.h.Ctx().TimeHooks()
	p.HP, p.SP = h.MaxHP(p), h.MaxSP(p)
	most := 0
	for i := 1; i <= len(m.Players) && i < len(d.trained); i++ {
		most = max(most, d.trained[i])
	}
	if s := m.Selected; s >= 0 && s < len(d.trained) {
		d.trained[s]++
		if d.trained[s] > most {
			min := (clock.HoursTo5AM(m.Calendar.Hour)+0xac)*60 - m.Calendar.Minute
			m.AdvanceTime(min, 0, d.h.Ctx())
			if strings.HasSuffix(strings.ToLower(d.h.MapName()), ".odm") {
				h.Weather()
			}
		}
	}
	d.speak(0x57)
	d.h.Status(cfmt(d.h.Global(0x1ae), p.Name, int(p.LevelBase), int(gain)), 2)
}

// ---- lost items ----------------------------------------------------------------------

// inkLostItem is the colour of the returned item's name (Color16(0xe1, 0xcd, 0x23)).
const inkLostItem = 0xe664

// lostItem is the lost-item return (topic 0x2c1): NPCText 851; or, for the first listed
// quest item whose quest bit is set and that no member has (nor the mouse), NPCText 852
// with the item's name, and the item goes to the mouse.
//
// mm8: 0x4b2ac7 (NPC_LostItemTopic), 0x4b363b (the %s of 852: the unidentified name in
// "\f%05d%s\f00000")
func (d *Dialog) lostItem() {
	d.State = StateLostItem
	d.Reply = d.TopicText(851)
	l := d.h.Tables().Learning
	if l == nil {
		return
	}
	m := d.h.Members()
	for _, li := range l.LostItems {
		if !m.QBits.Get(int(int16(li.QBit))) {
			continue
		}
		held := false
		for i := range m.Players {
			held = held || m.HasItem(i, li.Item)
		}
		if held {
			continue
		}
		name := ""
		if t := d.h.Ctx().Items; t != nil {
			name = t.Item(li.Item).UnidentifiedName
		}
		d.Reply = cfmt(d.TopicText(852), fmt.Sprintf("\f%05d%s\f00000", inkLostItem, name))
		if len(m.Players) > 0 {
			m.GiveItem(li.Item, true, d.h.Ctx())
		}
		return
	}
}
