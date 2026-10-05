package party

import (
	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
)

// TimeHooks are the parts of the clock's per-frame work that belong to later
// milestones: HP and spell points, buffs and hazards (M7/M9), the NPC roster (M6) and
// the weather. NoTimeHooks does none of them.
type TimeHooks interface {
	// Starve runs at the 3 AM rollover for each member when there is no food:
	// HP = HP/(days+1) + 1 (M7: the HP formulas).
	Starve(p *Player, days int)
	// DailyReset is the rest of the 3 AM rollover: NPC daily flags, the per-player
	// daily field +0x1d24, the full heal of roster characters outside the party (M6/M7).
	DailyReset()
	// Weather re-rolls the outdoor fog at 3 AM (0x48a92a; outdoors only).
	Weather()
	// Hazards is the lava and drowning damage (Party +0x70c; M7).
	Hazards(m *Members)
	// Regen is Party_Regen's body for n five-minute periods (fly/water walk upkeep,
	// regeneration items and buffs; M7/M9).
	Regen(m *Members, n int)
	// Vitals is the per-member HP check (HP <= 0: Unconscious, or Dead below -End
	// without Preservation) and the expiry of the 27 player buffs (M7/M9).
	Vitals(m *Members, i int)
	// PartyBuffs expires the 20 party buffs and drops fly/water walk without a caster
	// (M9).
	PartyBuffs(m *Members)
	// Immune reports whether member i resists cond when it can be blocked (Protection
	// from Magic, items, buffs; M7/M9).
	Immune(m *Members, i int, cond Condition) bool
	// MaxHP and MaxSP are the members' full HP and spell points (M7).
	MaxHP(p *Player) int32
	MaxSP(p *Player) int32
}

// NoTimeHooks does nothing; MaxHP and MaxSP keep the current values.
type NoTimeHooks struct{}

func (NoTimeHooks) Starve(*Player, int)                  {}
func (NoTimeHooks) DailyReset()                          {}
func (NoTimeHooks) Weather()                             {}
func (NoTimeHooks) Hazards(*Members)                     {}
func (NoTimeHooks) Regen(*Members, int)                  {}
func (NoTimeHooks) Vitals(*Members, int)                 {}
func (NoTimeHooks) PartyBuffs(*Members)                  {}
func (NoTimeHooks) Immune(*Members, int, Condition) bool { return false }
func (NoTimeHooks) MaxHP(p *Player) int32                { return p.HP }
func (NoTimeHooks) MaxSP(p *Player) int32                { return p.SP }

// Speech is the table of what a member does when something happens to them
// (0x4ff670, 8 bytes per event): bytes 0..1 are voice sound groups (M11), bytes 3..7
// the portrait expressions to pick from.
type Speech [][8]byte

// SpeechEntries is the number of entries in the speech table (up to g_portraitX).
const SpeechEntries = (0x4ff9e0 - 0x4ff670) / 8

// Ctx is what the party's time and condition code needs besides the members.
type Ctx struct {
	PFT    desc.PFT
	Speech Speech
	Rand   *Rand
	Hooks  TimeHooks
	// Items and Classes are the tables the stat formulas read (Env).
	Items   *tables.Items
	Classes *tables.Classes
}

// TimeHooks returns the hooks in use (NoTimeHooks when none are set).
func (c *Ctx) TimeHooks() TimeHooks { return c.hooks() }

func (c *Ctx) hooks() TimeHooks {
	if c.Hooks == nil {
		return NoTimeHooks{}
	}
	return c.Hooks
}

// CanAct reports a member who is not asleep, paralysed, unconscious, dead, stoned or
// eradicated.
//
// mm8: 0x49323a
func (p *Player) CanAct() bool {
	for _, c := range [...]Condition{CondAsleep, CondParalyzed, CondUnconscious, CondDead, CondStoned, CondEradicated} {
		if p.Conditions[c] != 0 {
			return false
		}
	}
	return true
}

// alive reports a member who is not dead, stoned or eradicated.
func (p *Player) alive() bool {
	return p.Conditions[CondDead] == 0 && p.Conditions[CondStoned] == 0 && p.Conditions[CondEradicated] == 0
}

// Speak plays member i's reaction to speech event id: a voice line (with sound on;
// libre-enroth runs like -nosound until M11, which draws no random numbers) and an
// expression picked at random from the event's list.
//
// mm8: 0x4949b1 (Player_Speak)
func (m *Members) Speak(i, id int, c *Ctx) {
	if i < 0 || i >= len(m.Players) || id < 0 || id >= len(c.Speech) {
		return
	}
	var exprs [5]int
	n := 0
	for _, e := range c.Speech[id][3:8] {
		if e != 0 {
			exprs[n] = int(e)
			n++
		}
	}
	if n == 0 {
		return
	}
	// A talking face (0x15) lasts as long as the voice line; without one, its sequence.
	m.Players[i].SetExpression(c.PFT, exprs[c.Rand.Int()%n], 0)
}

// conditionSpeech is the reaction to each newly set condition (-1 none).
//
// mm8: 0x4933b2 (Player_SetCondition)
var conditionSpeech = [numConditions]int{
	CondCursed: 0x1e, CondWeak: 0x19, CondAsleep: -1, CondAfraid: 0x1a, CondDrunk: 0x1f,
	CondInsane: 0x1d, CondPoison1: 0x1b, CondPoison2: 0x1b, CondPoison3: 0x1b,
	CondDisease1: 0x1c, CondDisease2: 0x1c, CondDisease3: 0x1c, CondParalyzed: -1,
	CondUnconscious: 0x20, CondDead: 0x21, CondStoned: 0x22, CondEradicated: 0x23,
	CondZombie: -1, 18: -1, 19: -1,
}

// SetCondition gives member i a condition at the current time, unless it is already
// set or, when blockable, the member is immune. Unconscious drops HP to 0, Dead and
// Eradicated HP and SP. When this leaves one member of two able to act, that one
// speaks (0x6b). Reports whether the condition was set.
//
// mm8: 0x4933b2 (Player_SetCondition)
func (m *Members) SetCondition(i int, cond Condition, blockable bool, c *Ctx) bool {
	p := &m.Players[i]
	if p.Conditions[cond] != 0 {
		return false
	}
	before := m.countCanAct()
	if blockable && c.hooks().Immune(m, i, cond) {
		return false
	}
	if s := conditionSpeech[cond]; s >= 0 {
		m.Speak(i, s, c)
	}
	switch cond {
	case CondUnconscious:
		p.HP = min(p.HP, 0)
	case CondDead, CondEradicated:
		p.HP, p.SP = min(p.HP, 0), min(p.SP, 0)
	}
	p.Conditions[cond] = int64(m.Time)
	if after, last := m.countCanActLast(); before == 2 && after == 1 {
		m.Speak(last, 0x6b, c)
	}
	return true
}

func (m *Members) countCanAct() int {
	n, _ := m.countCanActLast()
	return n
}

func (m *Members) countCanActLast() (n, last int) {
	for i := range m.Players {
		if m.Players[i].CanAct() {
			n, last = n+1, i
		}
	}
	return n, last
}

// TakeGold takes up to n gold (all of it when the party has less).
//
// mm8: 0x4931e5 (Party_TakeGold)
func (m *Members) TakeGold(n uint32) {
	if uint32(m.Gold) < n {
		m.Gold = 0
		return
	}
	m.Gold -= int32(n)
}

// EatFood takes n food, down to 0.
//
// mm8: 0x493132 (Party_EatFood; it also restarts the HUD food animation)
func (m *Members) EatFood(n int32) {
	m.Food = max(m.Food-n, 0)
}

// reduceRecovery takes ticks off a member's recovery (plus the recovery bonus, M7);
// a member who recovers is selected when nobody is.
//
// mm8: 0x48fb86 (Player_ReduceRecovery)
func (m *Members) reduceRecovery(i int, ticks int) {
	p := &m.Players[i]
	if p.Recovery <= ticks {
		p.Recovery = 0
		if m.Selected == 0 {
			m.Selected = m.NextSelectable()
		}
		return
	}
	p.Recovery -= ticks
}

// NextSelectable is the member to select next (1-based, 0 for none): the first one
// able to act, not recovering and not yet picked in this round; once all five slots
// were picked the round starts over. When that finds nobody (or only the first member,
// a quirk of the original's zero test) it takes the able member with the highest
// speed bonus (Player +0x366, 0 until M7), the first on a tie. Turn-based combat picks
// from the combat queue instead (M8).
//
// mm8: 0x4937a7 (Party_NextSelectable)
func (m *Members) NextSelectable() int {
	if m.TurnBased {
		return 0
	}
	pick := 0
	for i := range m.Players {
		p := &m.Players[i]
		if p.CanAct() && p.Recovery == 0 {
			if !m.visited[i] {
				m.visited[i] = true
				pick = i
				break
			}
		} else {
			m.visited[i] = true
		}
	}
	if m.visited == [MaxMembers]bool{true, true, true, true, true} {
		m.visited = [MaxMembers]bool{}
	}
	if pick != 0 {
		return pick + 1
	}
	for i := range m.Players {
		if p := &m.Players[i]; p.CanAct() && p.Recovery == 0 {
			return i + 1 // every speed bonus is 0 until M7: the first able member
		}
	}
	return 0
}

// UpdateTime advances the clock by ticks and does what time brings: the 3 AM rollover,
// hazards, regeneration, recovery, conditions and buffs. It reports the party's death
// (nobody can act and nobody sleeps; not while resting). The game calls it every frame
// with the timer's ticks unless the timer is paused or stopped (turn-based).
//
// mm8: 0x494007 (Party_UpdateTime)
func (m *Members) UpdateTime(ticks int, c *Ctx) (dead bool) {
	h := c.hooks()
	prev := m.Calendar
	m.Time += clock.Time(ticks)
	m.Calendar = m.Time.Calendar()
	if cal := m.Calendar; cal.Hour >= 3 && (prev.Hour < 3 || prev.Day < cal.Day) {
		m.dailyRollover(c)
	}
	h.Hazards(m)
	m.regen(c)
	active := len(m.Players)
	for i := range m.Players {
		p := &m.Players[i]
		if p.Recovery != 0 {
			m.reduceRecovery(i, ticks)
		}
		h.Vitals(m, i)
		if !p.CanAct() {
			active--
		}
	}
	h.PartyBuffs(m)
	if active == 0 && !m.Resting {
		for i := range m.Players {
			if p := &m.Players[i]; p.Conditions[CondAsleep] != 0 {
				p.Conditions[CondAsleep] = 0
				active = 1
				break
			}
		}
		if active == 0 {
			dead = true
		}
	}
	if s := m.Selected; s != 0 && !m.Resting && s <= len(m.Players) && !m.Players[s-1].CanAct() {
		m.Selected = m.NextSelectable()
	}
	return dead
}

// dailyRollover runs once a day at 3 AM: another day without rest makes everyone weak
// from the second on and eats a food (or, without food, starves); from the fourth on
// each living member may die (days*5 %) or else go insane (days*10 %).
//
// mm8: 0x494007 (inline in Party_UpdateTime)
func (m *Members) dailyRollover(c *Ctx) {
	h := c.hooks()
	m.DaysWithoutRest++
	if days := int(m.DaysWithoutRest); days > 1 {
		for i := range m.Players {
			m.SetCondition(i, CondWeak, false, c)
		}
		if m.Food == 0 {
			for i := range m.Players {
				h.Starve(&m.Players[i], days)
			}
		} else {
			m.EatFood(1)
		}
		if days > 3 {
			for i := range m.Players {
				if !m.Players[i].alive() {
					continue
				}
				switch {
				case c.Rand.Int()%100 < days*5:
					m.SetCondition(i, CondDead, false, c)
				case c.Rand.Int()%100 < days*10:
					m.SetCondition(i, CondInsane, false, c)
				}
			}
		}
	}
	h.Weather()
	h.DailyReset()
}

// regen runs Party_Regen's body once five game minutes have passed since the last
// time, for every five-minute period.
//
// mm8: 0x493a34 (Party_Regen)
func (m *Members) regen(c *Ctx) {
	now, last := m.Time.Minutes(), m.LastRegen.Minutes()
	if last+5 > now {
		return
	}
	c.hooks().Regen(m, int((now-last)/5))
	m.LastRegen = m.Time
}

// AdvanceMinutes moves the clock on by min minutes (256 ticks each), takes as much off
// every member's recovery, then runs UpdateTime, which adds the frame's ticks on top
// (as the original's does with g_timer.dt).
//
// mm8: 0x4939cb (Party_AdvanceMinutes)
func (m *Members) AdvanceMinutes(min, ticks int, c *Ctx) bool {
	d := clock.Time(min) * clock.Minute
	m.Time += d
	for i := range m.Players {
		m.reduceRecovery(i, int(d))
	}
	return m.UpdateTime(ticks, c)
}

// AdvanceTime is a journey's time passing at once (stables, boats, the map edges): min
// minutes go by without UpdateTime (so no 3 AM rollover runs), then a full rest:
// RestHeal, recovery cleared and the faces ticked once more.
//
// mm8: 0x4b2758
func (m *Members) AdvanceTime(min, ticks int, c *Ctx) {
	m.Time += clock.Time(min) * clock.Minute
	m.Calendar = m.Time.Calendar()
	m.RestHeal(ticks, c)
	for i := range m.Players {
		m.Players[i].Recovery = 0
	}
	m.TickExpressions(c.PFT, ticks, c.Rand)
}

// RoundEnd is the end of a turn-based round: 0xd5 ticks (50 game seconds) pass.
//
// mm8: 0x406bb3 (TurnBased_RoundEnd)
func (m *Members) RoundEnd(c *Ctx) bool {
	m.Time += 0xd5
	return m.UpdateTime(0, c)
}

// RestHeal is a full night's rest: party and player buffs end (M9) and every member who
// is not dead, stoned or eradicated loses Weak, Asleep, Afraid, Drunk, Unconscious and
// recovery and gets full HP and spell points, a quarter of them with the third poison
// or disease, a third with the second, half with the first; the insane get no spell
// points. Each healed member also ticks every face (ticks: this frame's).
//
// mm8: 0x491b34 (Party_RestHeal)
func (m *Members) RestHeal(ticks int, c *Ctx) {
	h := c.hooks()
	for i := range m.Players {
		p := &m.Players[i]
		if !p.alive() {
			continue
		}
		for _, cond := range [...]Condition{CondWeak, CondAsleep, CondAfraid, CondDrunk, CondUnconscious} {
			p.Conditions[cond] = 0
		}
		p.Recovery = 0
		p.HP, p.SP = h.MaxHP(p), h.MaxSP(p)
		switch {
		case p.Conditions[CondPoison3] != 0 || p.Conditions[CondDisease3] != 0:
			p.HP, p.SP = p.HP/4, p.SP/4
		case p.Conditions[CondPoison2] != 0 || p.Conditions[CondDisease2] != 0:
			p.HP, p.SP = p.HP/3, p.SP/3
		case p.Conditions[CondPoison1] != 0 || p.Conditions[CondDisease1] != 0:
			p.HP, p.SP = p.HP/2, p.SP/2
		}
		if p.Conditions[CondInsane] != 0 {
			p.SP = 0
		}
		m.TickExpressions(c.PFT, ticks, c.Rand)
	}
	m.DaysWithoutRest = 0
}
