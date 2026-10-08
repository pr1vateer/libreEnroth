package dialog

import (
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// learnHost is shopHost with the teaching houses: 27 a training hall (house 0x59's
// cap; Val 2, A 3), 21 the tavern (A 10), and a knight (class 2) whose class reaches
// the skills set below.
func learnHost(t *testing.T) *fakeHost {
	h := shopHost(t)
	h.t.Houses[0x59] = tables.House{Name: "Hall", Type: TypeTraining, Video: 160, Owner: "O", Title: "T", Val: 2, A: 3}
	h.t.HouseAnims[160] = tables.HouseAnim{Video: "v", Portrait: 1, Type: TypeTraining}
	h.t.Houses[21].A = 10
	l := &tables.Learning{}
	l.LevelCaps[0x59] = 5
	l.LostItems[0] = tables.LostItem{QBit: 50, Item: 3}
	l.LostItems[1] = tables.LostItem{QBit: 51, Item: 4}
	h.t.Learning = l
	c := h.ctx.Classes
	for s, r := range map[int]uint8{party.SkillSword: 4, party.SkillShield: 3, party.SkillIDItem: 2,
		party.SkillStealing: 4, party.SkillDisarm: 1, party.SkillBodybuilding: 3, party.SkillBlaster: 4,
		party.SkillPlate: 3, party.SkillDarkElf: 4, party.SkillIDMonster: 4} {
		c.SkillMax[2][s] = r
	}
	c.SkillMax[3][party.SkillShield] = 4 // the promotion reaches the shield's grandmaster
	p := &h.m.Players[0]
	p.Class, p.HP, p.LevelBase = 2, 10, 1
	h.glob = map[int]string{0x216: "Become %s in %s for %lu gold", 0x278: "not by %s",
		0x279: "promote to %s", 0x1b1: "Expert", 0x1b0: "Master", 0xe1: "GM",
		0x219: "Train to level %d for %d gold", 0x21a: "You need %d more experience to train to level %d",
		0x1ae: "%s is now Level %lu and has earned %lu Skill Points!"}
	return h
}

// TestLearnSkillsMenus: the skills each house type teaches; a weapon shop's from its
// stock records' kinds, an armour shop's from its four records, each once.
//
// mm8: 0x4b43ce (House_LearnSkillsMenu)
func TestLearnSkillsMenus(t *testing.T) {
	h := learnHost(t)
	for _, c := range []struct {
		house int
		want  []int
	}{
		{1, []int{party.SkillSword}},
		{15, []int{party.SkillLeather, party.SkillChain}},
		{29, []int{party.SkillIDItem, party.SkillRepair}},
		{42, []int{party.SkillAlchemy, party.SkillIDMonster}},
		{139, []int{12, 13, 15, 14, party.SkillLearning}},
		{21, []int{party.SkillStealing, party.SkillDisarm, party.SkillPerception}},
		{20, []int{party.SkillRegeneration, party.SkillMerchant}},
		{0x59, []int{party.SkillArmsmaster, party.SkillBodybuilding}},
	} {
		d := openProprietor(t, h, c.house)
		if got := d.LearnSkills(); !slices.Equal(got, c.want) {
			t.Errorf("house %d: %v, want %v", c.house, got, c.want)
		}
		clickService(d, SvcLearn)
		var want []int
		for _, s := range c.want {
			want = append(want, LearnCode0+s)
		}
		if d.Menu != SvcLearn || !slices.Equal(params(d.Buttons), want) {
			t.Errorf("house %d: menu %#x buttons %#x", c.house, d.Menu, params(d.Buttons))
		}
		if !d.Back() || d.Menu != 1 {
			t.Errorf("house %d: back to %#x", c.house, d.Menu)
		}
	}
	// a weapon shop teaching five kinds at most
	s := h.t.Shops
	for k, kind := range []int{0x17, 0x18, 0x19, 0x1a} {
		s.SetShort(0x50279a+10+2+uint32(2*k), kind)
		s.SetShort(0x5029ce+10+2+uint32(2*k), kind+4)
	}
	d := openProprietor(t, h, 1)
	want := []int{party.SkillSword, party.SkillDagger, party.SkillAxe, party.SkillSpear, party.SkillBow}
	if got := d.LearnSkills(); !slices.Equal(got, want) {
		t.Errorf("five kinds: %v", got)
	}
}

// TestLearnSkill: a skill costs A · 500 (merchant-adjusted) and is learnt at level 1 by
// a member whose class has it and who does not know it; without the gold nothing
// changes.
//
// mm8: 0x4bd028 (codes 0x24..0x4a), 0x4b5618 (menu 0x60)
func TestLearnSkill(t *testing.T) {
	h := learnHost(t)
	d := openProprietor(t, h, 21) // the tavern: A 10
	if got := d.LearnPrice(); got != 5000 {
		t.Errorf("price %d, want 5000", got)
	}
	h.m.Gold = 6000
	clickService(d, SvcLearn)
	p := &h.m.Players[0]
	if !d.CanLearn(p, party.SkillStealing) || d.CanLearn(p, party.SkillPerception) {
		t.Error("class gating")
	}
	clickService(d, LearnCode0+party.SkillPerception) // not the knight's
	if p.Skills[party.SkillPerception] != 0 || h.m.Gold != 6000 {
		t.Error("learnt a skill the class has not")
	}
	clickService(d, LearnCode0+party.SkillStealing)
	if p.Skills[party.SkillStealing] != 1 || h.m.Gold != 1000 || !d.Traded || h.spoke[len(h.spoke)-1] != 0x4e {
		t.Errorf("learn: skill %#x gold %d", p.Skills[party.SkillStealing], h.m.Gold)
	}
	if d.CanLearn(p, party.SkillStealing) {
		t.Error("known skill still learnable")
	}
	clickService(d, LearnCode0+party.SkillDisarm) // 1000 gold left
	if p.Skills[party.SkillDisarm] != 0 || h.m.Gold != 1000 || h.status[len(h.status)-1] != "g0x9b" {
		t.Errorf("no gold: skill %#x gold %d %q", p.Skills[party.SkillDisarm], h.m.Gold, h.status)
	}
	// a member who cannot act blocks the menu
	p.Conditions[party.CondAsleep] = 1
	h.m.Gold = 6000
	clickService(d, LearnCode0+party.SkillDisarm)
	if p.Skills[party.SkillDisarm] != 0 {
		t.Error("blocked member learnt")
	}
}

// TestLesson: a teacher's lesson for the selected member, the label it shows, its
// price and what taking it does.
//
// mm8: 0x4b31fe (Teacher_Offer), 0x4b2b74 (param 0x4f)
func TestLesson(t *testing.T) {
	h := learnHost(t)
	p := &h.m.Players[0]
	ev := func(skill, rank int) int { return 300 + 3*skill + rank - 2 }
	d := OpenHouse(h, 5)
	d.SelectResident(1)
	lesson := func(skill, rank int) Lesson { return d.Lesson(ev(skill, rank)) }
	text := h.t.Topics.Text
	h.m.Gold = 9000
	for _, c := range []struct {
		name        string
		skill, rank int
		set         func()
		label       string
		ok          bool
		price       int
	}{
		{"class can't", party.SkillIDItem, 3, nil, "not by g0x2a7", false, 0},
		{"promotion can", party.SkillShield, 4, nil, "promote to g0x2a8", false, 0},
		{"no skill", party.SkillSword, 2, nil, text[132], false, 0},
		{"level 3", party.SkillSword, 2, func() { p.Skills[party.SkillSword] = 3 }, text[128], false, 0},
		{"expert sword", party.SkillSword, 2, func() { p.Skills[party.SkillSword] = 4 }, "Become Expert in g0x110 for 2000 gold", true, 2000},
		{"master too soon", party.SkillSword, 3, nil, text[128], false, 0},
		{"has expert", party.SkillSword, 2, func() { p.Skills[party.SkillSword] = 4 | party.SkillMaster }, text[130-1+0], false, 0},
		{"expert shield", party.SkillShield, 2, func() { p.Skills[party.SkillShield] = 5 }, "Become Expert in g0x117 for 1000 gold", true, 1000},
		{"expert id item", party.SkillIDItem, 2, func() { p.Skills[party.SkillIDItem] = 4 }, "Become Expert in g0x124 for 500 gold", true, 500},
		{"master plate", party.SkillPlate, 3, func() { p.Skills[party.SkillPlate] = 7 | party.SkillExpert }, "Become Master in g0x11a for 3000 gold", true, 3000},
		{"bodybuilding end", party.SkillBodybuilding, 3, func() { p.Skills[party.SkillBodybuilding] = 7 | party.SkillExpert }, text[128], false, 0},
		{"bodybuilding", party.SkillBodybuilding, 3, func() { *p.BasePtr(party.StatEndurance) = 50 }, "Become Master in g0x127 for 2500 gold", true, 2500},
		{"gm id monster gold", party.SkillIDMonster, 4, func() { p.Skills[party.SkillIDMonster] = 10 | party.SkillMaster; h.m.Gold = 5999 }, text[125], false, 0},
		{"gm id monster", party.SkillIDMonster, 4, func() { h.m.Gold = 6000 }, "Become GM in g0x58 for 6000 gold", true, 6000},
		{"asleep", party.SkillIDMonster, 4, func() { p.Conditions[party.CondAsleep] = 1 }, text[123], false, 0},
	} {
		if c.set != nil {
			c.set()
		}
		l := lesson(c.skill, c.rank)
		if l.Label != c.label || l.OK != c.ok || c.ok && l.Price != c.price {
			t.Errorf("%s: %+v, want %q %v %d", c.name, l, c.label, c.ok, c.price)
		}
	}
	// "has expert": an expert-rank lesson for a master says NPCText 129
	p.Conditions[party.CondAsleep] = 0
	p.Skills[party.SkillSword] = 8 | party.SkillMaster
	if l := lesson(party.SkillSword, 2); l.Label != text[129] {
		t.Errorf("master asking for expert: %q", l.Label)
	}

	// Taking a lesson in a house: the gold goes, the rank replaces the old one; the
	// dark elf's rank brings its spell.
	d.teacher(ev(party.SkillShield, 2))
	if d.State != StateTeacher || d.Reply != text[ev(party.SkillShield, 2)] || d.Label(d.Buttons[0]) != "Become Expert in g0x117 for 1000 gold" {
		t.Fatalf("teacher topic: %#x %q %q", d.State, d.Reply, d.Label(d.Buttons[0]))
	}
	h.m.Gold = 1500
	d.Click(d.Buttons[0])
	if p.Skills[party.SkillShield] != 5|party.SkillExpert || h.m.Gold != 500 || h.spoke[len(h.spoke)-1] != 0x55 || !d.TakeBack() {
		t.Errorf("lesson: skill %#x gold %d", p.Skills[party.SkillShield], h.m.Gold)
	}
	p.Skills[party.SkillDarkElf] = 7 | party.SkillExpert
	h.m.Gold = 9000
	d.teacher(ev(party.SkillDarkElf, 3))
	d.Click(d.Buttons[0])
	if p.Skills[party.SkillDarkElf] != 7|party.SkillMaster || !p.Spells[101] || p.Spells[100] || !d.TakeBack() {
		t.Errorf("dark elf master: %#x %v", p.Skills[party.SkillDarkElf], p.Spells[99:103])
	}
	// nothing to learn: no gold taken, no back step
	h.m.Gold = 10
	d.teacher(ev(party.SkillSword, 4))
	d.Click(d.Buttons[0])
	if h.m.Gold != 10 || d.TakeBack() {
		t.Error("an impossible lesson acted")
	}
	// the topics missing from Teacher_SkillOfTopic (the blaster's, dodging's, unarmed's
	// and stealing's) answer skill 0 after the crash message
	if s, ok := teacherSkill(ev(party.SkillBlaster, 4)); ok || s != 0 {
		t.Error("blaster topic has a skill")
	}
	if l := lesson(party.SkillStealing, 4); l.Skill != 0 || h.notes[len(h.notes)-1] != "Crash in getSkillIndexFromEvent() (0x4b301c)" {
		t.Errorf("stealing topic: %+v", l)
	}
}

// TestTraining: the price is level · Val (twice for a promoted class), merchant-
// adjusted; the Train button opens only with the experience and below the hall's cap;
// training raises the level, gives level / 10 + 5 skill points and full HP/SP, and the
// first level of a member beyond the others' lets the clock run to 9 AM eight days on.
//
// mm8: 0x4b5573 (Training_Price), 0x4b555b, 0x4bd028 (menu 1), 0x4b5618 (menu 0x11)
func TestTraining(t *testing.T) {
	h := learnHost(t)
	p := &h.m.Players[0]
	if ExpForLevel(1) != 1000 || ExpForLevel(4) != 10000 {
		t.Errorf("exp: %d %d", ExpForLevel(1), ExpForLevel(4))
	}
	d := openProprietor(t, h, 0x59)
	if d.LevelCap() != 5 {
		t.Fatalf("cap %d", d.LevelCap())
	}
	if _, ok := d.TrainingPrice(p); ok {
		t.Error("price without the experience")
	}
	if got := d.TrainingLabel(p); got != "You need 1000 more experience to train to level 2" {
		t.Errorf("label %q", got)
	}
	clickService(d, SvcTrain)
	if d.Menu != 1 {
		t.Errorf("train opened without the experience")
	}
	p.Exp, p.LevelBase = 10000, 4
	if price, ok := d.TrainingPrice(p); !ok || price != 8 {
		t.Errorf("price %d %v, want 8", price, ok)
	}
	p.Class = 3
	if price, _ := d.TrainingPrice(p); price != 16 {
		t.Errorf("promoted price %d, want 16", price)
	}
	if got := d.TrainingLabel(p); got != "Train to level 5 for 16 gold" {
		t.Errorf("label %q", got)
	}
	h.m.Gold, h.m.Calendar.Hour, h.m.Calendar.Minute = 100, 10, 30
	h.m.Time = clock.Time(10*60+30) * clock.Minute
	start := h.m.Time
	clickService(d, SvcTrain)
	if !d.TakeBack() {
		t.Fatal("no back step")
	}
	d.Back()
	if p.LevelBase != 5 || p.SkillPoints != 5 || h.m.Gold != 84 || h.spoke[len(h.spoke)-1] != 0x57 {
		t.Errorf("trained: level %d points %d gold %d", p.LevelBase, p.SkillPoints, h.m.Gold)
	}
	if got := h.status[len(h.status)-1]; got != "Zed is now Level 5 and has earned 5 Skill Points!" {
		t.Errorf("status %q", got)
	}
	if want := start + clock.Time((19+172)*60-30)*clock.Minute; h.m.Time != want {
		t.Errorf("time %d, want %d", h.m.Time, want)
	}
	if c := h.m.Time.Calendar(); c.Hour != 9 || c.Minute != 0 {
		t.Errorf("arrived %d:%02d", c.Hour, c.Minute)
	}
	// at the cap: the teacher line, and the button does not open
	if got := d.TrainingLabel(p); got != "g0x218\n \ng0x211" {
		t.Errorf("cap label %q", got)
	}
	p.Exp = 1 << 30
	clickService(d, SvcTrain)
	if d.Menu != 1 || p.LevelBase != 5 {
		t.Error("trained past the cap")
	}
	// a second member's first level: the first member has one already, no week
	h.m.Players = append(h.m.Players, party.Player{Name: "B", Class: 2, LevelBase: 1, Exp: 1000})
	h.m.Selected = 2
	before := h.m.Time
	clickService(d, SvcTrain)
	if h.m.Players[1].LevelBase != 2 || h.m.Time != before {
		t.Errorf("second member: level %d, time moved %d", h.m.Players[1].LevelBase, h.m.Time-before)
	}
}

// TestLostItem: the lost-item topic gives back the first listed item whose quest bit is
// set and that nobody has, onto the mouse, with NPCText 852 naming it; otherwise
// NPCText 851.
//
// mm8: 0x4b2ac7 (NPC_LostItemTopic)
func TestLostItem(t *testing.T) {
	h := learnHost(t)
	text := h.t.Topics.Text
	text[852] = "Here is your %s."
	d := OpenHouse(h, 5)
	d.lostItem()
	if d.State != StateLostItem || d.Reply != text[851] || h.m.MouseItem.Number != 0 {
		t.Errorf("nothing lost: %q", d.Reply)
	}
	h.m.QBits.Set(50, true)
	h.m.QBits.Set(51, true)
	h.m.MouseItem.Number = 3 // item 3 is on the mouse: item 4 comes back
	d.lostItem()
	name := h.ctx.Items.Item(4).UnidentifiedName
	if want := "Here is your \f58980" + name + "\f00000."; d.Reply != want || h.m.MouseItem.Number != 4 {
		t.Errorf("lost item: %q (mouse %d), want %q", d.Reply, h.m.MouseItem.Number, want)
	}
	if h.m.MouseItem.Flags&1 == 0 {
		t.Error("returned item not identified")
	}
}
