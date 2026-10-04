package party

import (
	"testing"

	"libre-enroth/internal/game/clock"
)

// countHooks counts the hook calls.
type countHooks struct {
	NoTimeHooks
	starved, daily, regen, regenPeriods int
}

func (h *countHooks) Starve(p *Player, days int) { h.starved++; p.HP = p.HP/int32(days+1) + 1 }
func (h *countHooks) DailyReset()                { h.daily++ }
func (h *countHooks) Regen(_ *Members, n int)    { h.regen++; h.regenPeriods += n }

func newTestParty(n int, seed uint32) (*Members, *Ctx, *countHooks) {
	m := &Members{Players: make([]Player, n)}
	h := &countHooks{}
	c := &Ctx{PFT: testPFT(), Speech: make(Speech, SpeechEntries), Rand: NewRand(seed), Hooks: h}
	m.NewGame(c.Rand)
	return m, c, h
}

// TestUpdateTimeRollover: the 3 AM rollover fires once a day, whatever the step size;
// the first day without rest only counts, the second makes everyone weak and eats a
// food.
//
// mm8: 0x494007 (Party_UpdateTime)
func TestUpdateTimeRollover(t *testing.T) {
	m, c, h := newTestParty(3, 1)
	if m.Calendar.Hour != 9 || m.Food != 7 || m.Gold != 200 {
		t.Fatalf("new game: %+v food %d gold %d", m.Calendar, m.Food, m.Gold)
	}
	// 9:00 day 1 -> 2:59 day 2: no rollover yet.
	for m.Time < clock.NewGame+18*clock.Hour-clock.Minute {
		m.UpdateTime(3, c)
	}
	if h.daily != 0 || m.DaysWithoutRest != 0 {
		t.Fatalf("rollover before 3 AM: %d", h.daily)
	}
	for m.Time < clock.NewGame+19*clock.Hour {
		m.UpdateTime(3, c)
	}
	if h.daily != 1 || m.DaysWithoutRest != 1 || m.Food != 7 || m.Players[0].Conditions[CondWeak] != 0 {
		t.Fatalf("first rollover: daily %d days %d food %d", h.daily, m.DaysWithoutRest, m.Food)
	}
	// One big step over the next 3 AM fires once.
	m.UpdateTime(int(clock.Day), c)
	if h.daily != 2 || m.DaysWithoutRest != 2 || m.Food != 6 {
		t.Fatalf("second rollover: daily %d days %d food %d", h.daily, m.DaysWithoutRest, m.Food)
	}
	for i := range m.Players {
		if m.Players[i].Conditions[CondWeak] != int64(m.Time) {
			t.Errorf("member %d not weak since now", i)
		}
	}
	// Five-minute regeneration periods: the whole run is (9 h + 2 days)/5 min.
	if want := int((m.Time.Minutes() - clock.NewGame.Minutes()) / 5); h.regenPeriods < want-1 || h.regenPeriods > want {
		t.Errorf("regen periods %d, want about %d", h.regenPeriods, want)
	}
}

// TestRolloverStarveAndRolls: without food the members starve; from the fourth day
// without rest each living member rolls Dead (days*5 %) then Insane (days*10 %).
func TestRolloverStarveAndRolls(t *testing.T) {
	// Find a seed where member 0 dies, member 1 goes insane and member 2 is spared at
	// 4 days: rolls r%100 < 20 die, else a second roll < 40 goes insane.
	seed := seedFor(t, func(r *Rand) bool {
		return r.Int()%100 < 20 && r.Int()%100 >= 20 && r.Int()%100 < 40 && r.Int()%100 >= 20 && r.Int()%100 >= 40
	})
	m, c, h := newTestParty(3, 0)
	for i := range m.Players {
		m.Players[i].HP = 50
	}
	m.Food, m.DaysWithoutRest = 0, 3
	c.Rand = NewRand(seed)
	m.dailyRollover(c)
	if h.starved != 3 || m.Players[2].HP != 50/5+1 {
		t.Errorf("starved %d, HP %d", h.starved, m.Players[2].HP)
	}
	p := m.Players
	if p[0].Conditions[CondDead] == 0 || p[1].Conditions[CondInsane] == 0 || p[1].Conditions[CondDead] != 0 ||
		p[2].Conditions[CondDead] != 0 || p[2].Conditions[CondInsane] != 0 {
		t.Errorf("rolls: dead %v insane %v", [3]bool{p[0].Conditions[CondDead] != 0, p[1].Conditions[CondDead] != 0, p[2].Conditions[CondDead] != 0},
			[3]bool{p[0].Conditions[CondInsane] != 0, p[1].Conditions[CondInsane] != 0, p[2].Conditions[CondInsane] != 0})
	}
	if p[0].HP != 0 {
		t.Errorf("dead member keeps HP %d", p[0].HP)
	}
}

// TestAdvanceMinutes: 256 ticks a minute, plus the frame's ticks from UpdateTime.
//
// mm8: 0x4939cb
func TestAdvanceMinutes(t *testing.T) {
	m, c, _ := newTestParty(2, 1)
	m.Players[1].Recovery = 300
	m.AdvanceMinutes(6, 2, c)
	if m.Time != clock.NewGame+6*256+2 || m.Calendar.Minute != 6 {
		t.Errorf("time %#x minute %d", int64(m.Time), m.Calendar.Minute)
	}
	if m.Players[1].Recovery != 0 {
		t.Errorf("recovery %d", m.Players[1].Recovery)
	}
}

// TestSetCondition: a set condition keeps its time; Unconscious and Dead zero HP; when
// one of two able members drops, the other speaks (0x6b) with an expression from the
// table.
//
// mm8: 0x4933b2
func TestSetCondition(t *testing.T) {
	m, c, _ := newTestParty(2, 1)
	c.Speech[0x6b] = [8]byte{3: 0x30}
	c.Speech[0x21] = [8]byte{3: 0x31}
	m.Players[0].HP, m.Players[1].HP = 10, 10
	if !m.SetCondition(0, CondDead, false, c) || m.SetCondition(0, CondDead, false, c) {
		t.Fatal("set twice")
	}
	if p := m.Players[0]; p.HP != 0 || p.Conditions[CondDead] != int64(m.Time) {
		t.Errorf("dead: HP %d time %d", p.HP, p.Conditions[CondDead])
	}
	if m.Players[1].Expr != 0x30 {
		t.Errorf("survivor's expression %#x, want 0x30", m.Players[1].Expr)
	}
}

// TestRestHeal clears the rest conditions and divides by the worst poison/disease.
func TestRestHeal(t *testing.T) {
	m, c, _ := newTestParty(3, 1)
	c.Hooks = struct {
		NoTimeHooks
	}{}
	for i := range m.Players {
		m.Players[i].HP, m.Players[i].SP = 120, 60
		m.Players[i].Conditions[CondWeak] = 5
		m.Players[i].Conditions[CondAsleep] = 5
	}
	m.Players[1].Conditions[CondPoison2] = 1
	m.Players[2].Conditions[CondDead] = 1
	m.DaysWithoutRest = 3
	m.RestHeal(2, c)
	p := m.Players
	if p[0].Conditions[CondWeak] != 0 || p[0].Conditions[CondAsleep] != 0 || p[0].HP != 120 {
		t.Errorf("member 0: %+v", p[0].Conditions)
	}
	if p[1].HP != 40 || p[1].SP != 20 {
		t.Errorf("poisoned: HP %d SP %d", p[1].HP, p[1].SP)
	}
	if p[2].Conditions[CondWeak] == 0 {
		t.Errorf("the dead were healed")
	}
	if m.DaysWithoutRest != 0 {
		t.Errorf("days %d", m.DaysWithoutRest)
	}
}

// TestRoundEnd: a turn-based round is 0xd5 ticks.
func TestRoundEnd(t *testing.T) {
	m, c, _ := newTestParty(1, 1)
	m.TurnBased = true
	m.RoundEnd(c)
	if m.Time != clock.NewGame+0xd5 {
		t.Errorf("time %#x", int64(m.Time))
	}
}

func TestBits(t *testing.T) {
	b := make(Bits, 2)
	b.Set(1, true)
	b.Set(10, true)
	if b[0] != 0x80 || b[1] != 0x40 || !b.Get(1) || !b.Get(10) || b.Get(2) || b.Get(0) || b.Get(17) {
		t.Errorf("bits %x", []byte(b))
	}
	b.Set(1, false)
	b.Set(99, true)
	if b[0] != 0 {
		t.Errorf("clear: %x", []byte(b))
	}
}
