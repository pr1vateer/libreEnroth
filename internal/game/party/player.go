package party

import (
	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/game/clock"
)

// Condition indexes Player.Conditions.
type Condition int

// The conditions (Player +0x000, int64 game time each began). Only some are set
// before M7; the rest are here for the expression table.
const (
	CondCursed      Condition = 0
	CondWeak        Condition = 1
	CondAsleep      Condition = 2
	CondAfraid      Condition = 3
	CondDrunk       Condition = 4
	CondInsane      Condition = 5
	CondPoison1     Condition = 6
	CondDisease1    Condition = 7
	CondPoison2     Condition = 8
	CondDisease2    Condition = 9
	CondPoison3     Condition = 10
	CondDisease3    Condition = 11
	CondParalyzed   Condition = 12
	CondUnconscious Condition = 13
	CondDead        Condition = 14
	CondStoned      Condition = 15
	CondEradicated  Condition = 16
	CondZombie      Condition = 17
	CondGood        Condition = 0x12 // no condition set
	numConditions             = 20
)

// conditionPriority is the order Player_GetMainCondition tries the conditions in.
//
// mm8: 0x4fffb0 (g_conditionPriority)
var conditionPriority = [...]Condition{
	CondEradicated, CondStoned, CondDead, CondZombie, CondUnconscious, CondAsleep,
	CondParalyzed, CondDisease3, CondPoison3, CondDisease2, CondPoison2, CondDisease1,
	CondPoison1, CondInsane, CondDrunk, CondAfraid, CondWeak, CondCursed,
}

// Portrait expressions (dpft.bin ids) the code picks itself.
const (
	ExprNormal  = 1
	ExprTalking = 0x15
	ExprDead    = 0x62 // not in dpft.bin: the DEAD face is drawn instead
	ExprErad    = 0x63
)

// conditionExpr is the expression each condition shows (Party_TickExpressions'
// switch); -1 keeps the current one (Zombie).
var conditionExpr = [numConditions]int{
	CondCursed: 2, CondWeak: 3, CondAsleep: 4, CondAfraid: 5, CondDrunk: 6, CondInsane: 7,
	CondPoison1: 8, CondPoison2: 8, CondPoison3: 8,
	CondDisease1: 9, CondDisease2: 9, CondDisease3: 9,
	CondParalyzed: 10, CondUnconscious: 11, CondDead: ExprDead, CondStoned: 12,
	CondEradicated: ExprErad, CondZombie: -1, 18: -1, 19: -1,
}

// Player is a party member: the parts of the original's Player (0x1d28 bytes, types.h)
// that libre-enroth uses so far.
type Player struct {
	Name       string               // +0x0a8
	Class      int                  // +0x352: class id (tables.ClassByName)
	Face       int                  // +0x353: portrait 0..29
	RosterID   int                  // its g_players index in g_partyRoster: 0 the hero, -1 none
	Voice      int                  // +0x1be4
	Conditions [numConditions]int64 // +0x000: game time each began, 0 = not set
	Recovery   int                  // +0x1bf2: ticks until the player can act again
	HP, SP     int32                // +0x1bf8, +0x1bfc: set by the stats (M7)
	Expr       uint16               // +0x1c86: dpft.bin expression
	ExprTime   uint16               // +0x1c88: ticks into it
	ExprLen    uint16               // +0x1c8a: when it ends
	TalkFrame  int                  // +0x1c94: dpft.bin record while talking
	TalkTime   int                  // +0x1c90
}

// MainCondition is the condition that shows: the first set one in priority order, else
// CondGood.
//
// mm8: 0x48fd30 (Player_GetMainCondition)
func (p *Player) MainCondition() Condition {
	for _, c := range conditionPriority {
		if p.Conditions[c] != 0 {
			return c
		}
	}
	return CondGood
}

// SetExpression starts expression expr for length ticks (0: the dpft.bin sequence's
// length). Condition faces only give way to 0x22..0x24, and Paralyzed and the
// dead/eradicated faces to nothing, except that Asleep and Paralyzed take 0x3a.
//
// mm8: 0x494b61 (Player_SetExpression)
func (p *Player) SetExpression(pft desc.PFT, expr, length int) {
	cur := p.Expr
	if !((cur == 4 || cur == 0xc) && expr == 0x3a) && cur > 1 {
		if cur < 0xc {
			if expr != 0x22 && expr != 0x23 && expr != 0x24 {
				return
			}
		} else if cur == 0xc || cur > 0x61 && cur < 100 {
			return
		}
	}
	p.ExprTime = 0
	p.ExprLen = uint16(length)
	if length == 0 {
		p.ExprLen = uint16(pft[pft.Find(expr)].Total << 3)
	}
	p.Expr = uint16(expr)
}

// tickExpression advances one member's face by ticks: a healthy member returns to the
// normal face when an expression ends and now and then (1 in 5) picks an idle one; any
// other condition shows its own face.
//
// mm8: 0x4917dd (Party_TickExpressions, loop body)
func (p *Player) tickExpression(pft desc.PFT, ticks int, rng *Rand) {
	cond := p.MainCondition()
	if cond == CondGood {
		p.ExprTime += uint16(ticks)
		if p.ExprLen > p.ExprTime {
			return
		}
		if p.Expr == ExprNormal && rng.Int()%5 == 0 {
			p.Expr = uint16(idleExpr(rng.Int() % 100))
			p.ExprTime = 0
			p.ExprLen = uint16(pft[pft.Find(int(p.Expr))].Total << 3)
		} else {
			p.Expr = ExprNormal
			p.ExprTime = 0
			p.ExprLen = uint16(rng.Int()%0x100 + 0x20)
		}
		return
	}
	if p.Expr >= 0x22 && p.Expr <= 0x24 && int(p.ExprTime)+ticks < int(p.ExprLen) {
		p.ExprTime += uint16(ticks)
		return
	}
	p.ExprLen, p.ExprTime = 0, 0
	if e := conditionExpr[cond]; e >= 0 {
		p.Expr = uint16(e)
	}
}

// idleExpr maps rand() % 100 to an idle expression.
//
// mm8: 0x4917dd (Party_TickExpressions)
func idleExpr(r int) int {
	for _, s := range [...]struct{ below, expr int }{
		{25, 0xd}, {31, 0xe}, {37, 0xf}, {43, 0x10}, {46, 0x11}, {52, 0x12}, {58, 0x13},
		{64, 0x14}, {70, 0x36}, {76, 0x37}, {82, 0x38}, {88, 0x39}, {94, 0x1d},
	} {
		if r < s.below {
			return s.expr
		}
	}
	return 0x1e
}

// Portrait results besides a face texture number.
const (
	PortraitDead       = -1 // icons.lod "DEAD"
	PortraitEradicated = -2 // "ERADCATE"
)

// Portrait returns the face to draw: the texture number t (1..56, the "%02d" of the
// member's face icons) of the current expression frame, or PortraitDead or
// PortraitEradicated. A talking face advances by ticks.
//
// mm8: 0x49286e (Portraits_Draw, per member)
func (p *Player) Portrait(pft desc.PFT, ticks int, rng *Rand) int {
	switch {
	case p.Conditions[CondEradicated] != 0:
		return PortraitEradicated
	case p.Conditions[CondDead] != 0:
		return PortraitDead
	}
	i := pft.Find(int(p.Expr))
	if i == 0 {
		i = 1
	}
	if p.Expr == ExprTalking {
		i = pft.TalkFrame(&p.TalkFrame, &p.TalkTime, ticks, rng.Int)
	} else {
		i = pft.FrameAt(i, int(p.ExprTime))
	}
	return pft[i].Texture
}

// MaxMembers is the party size (g_partyCount at most 5).
const MaxMembers = 5

// Members are the party's characters, the selected one and the party-wide state of
// the original's Party struct (g_party 0xb20d90) that is not about movement.
type Members struct {
	Players  []Player
	Selected int // 1-based, 0 = none (g_selectedPlayer 0x5192a0)

	Time      clock.Time     // +0x2c: game time
	LastRegen clock.Time     // +0x34: when Party_Regen last ran
	Calendar  clock.Calendar // +0x71c..+0x734: derived from Time by UpdateTime
	Food      int32          // +0x738
	Gold      int32          // +0x744
	Bank      int32          // +0x748
	Deaths    int32          // +0x74c
	Prison    int32          // +0x754: prison terms
	Bounty    int32          // +0x758: total bounty collected
	// DaysWithoutRest counts 3 AM rollovers since the last rest (+0x77e).
	DaysWithoutRest uint8
	QBits           Bits             // +0x77f: quest bits, 1-based, MSB first
	ArenaWins       [4]uint8         // +0x7d0
	Autonotes       Bits             // +0x818
	Counters        [10]clock.Time   // +0x4ec: event counters 0xf7..0x100
	Stamps          [20]clock.Time   // +0x624: event timestamps 0x101..0x114
	History         [29]clock.Time   // +0x53c: history entries 0x116..0x132
	TurnBased       bool             // +0x898
	Resting         bool             // the rest screen is open (g_screenMode 5)
	visited         [MaxMembers]bool // 0xbb2ff8: NextSelectable's round robin
	// Roster holds the roster characters (g_players 0xb2177c) while they are not in the
	// party; a member who leaves is copied back. Index 0 is the hero.
	Roster []Player
}

// Quest and autonote bit array sizes (Party +0x77f..+0x7cf, +0x818..+0x897).
const (
	QBitBytes     = 0x51
	AutonoteBytes = 0x80
)

// Bits is a 1-based bit array, most significant bit first, like the quest bits.
//
// mm8: 0x44836d (bit test), 0x448394 (bit set)
type Bits []byte

// Get reports bit n (1-based); out of range bits are clear.
func (b Bits) Get(n int) bool {
	n--
	return n >= 0 && n/8 < len(b) && b[n/8]&(0x80>>(n%8)) != 0
}

// Set sets or clears bit n (1-based); out of range bits are ignored.
func (b Bits) Set(n int, on bool) {
	n--
	if n < 0 || n/8 >= len(b) {
		return
	}
	if on {
		b[n/8] |= 0x80 >> (n % 8)
	} else {
		b[n/8] &^= 0x80 >> (n % 8)
	}
}

// NewGame resets the members for a new game: the first one is selected, conditions and
// recovery are cleared and every face starts on the normal expression. The clock
// starts at 9:00 AM on day 1, with 7 food and 200 gold.
//
// mm8: 0x49228d (Party_InitNewGame)
func (m *Members) NewGame(rng *Rand) {
	m.Selected = 1
	m.Time, m.LastRegen = clock.NewGame, clock.NewGame
	m.Calendar = m.Time.Calendar()
	m.Food, m.Gold, m.Bank = 7, 200, 0
	m.Deaths, m.Prison, m.Bounty, m.DaysWithoutRest = 0, 0, 0, 0
	m.QBits, m.Autonotes = make(Bits, QBitBytes), make(Bits, AutonoteBytes)
	m.ArenaWins = [4]uint8{}
	m.Counters, m.Stamps, m.History = [10]clock.Time{}, [20]clock.Time{}, [29]clock.Time{}
	m.TurnBased, m.Resting = false, false
	for i := range m.Players {
		p := &m.Players[i]
		p.Recovery = 0
		p.Conditions = [numConditions]int64{}
	}
	for i := range m.Players {
		p := &m.Players[i]
		p.Expr, p.ExprTime = ExprNormal, 0
		p.ExprLen = uint16(rng.Int()%0x100 + 0x80)
	}
}

// ClickPortrait handles a click on (or the hotkey of) the portrait in 1-based slot, in
// the game view with nothing on the mouse: it selects that member unless they are
// recovering. Clicking the selected member opens the character screen (M7).
//
// mm8: 0x4213c0 (Party_ClickPortrait, g_screenMode 0)
func (m *Members) ClickPortrait(slot int) {
	if slot == m.Selected || slot < 1 || slot > len(m.Players) {
		return
	}
	if m.Players[slot-1].Recovery == 0 {
		m.Selected = slot
	}
}

// TickExpressions advances every member's face by ticks; the original runs it once per
// drawn frame with g_miscTimer's ticks.
//
// mm8: 0x4917dd (Party_TickExpressions)
func (m *Members) TickExpressions(pft desc.PFT, ticks int, rng *Rand) {
	for i := range m.Players {
		m.Players[i].tickExpression(pft, ticks, rng)
	}
}

// Rand is the MSVC runtime's rand(): a 32-bit LCG returning bits 16..30. The game
// shares one generator for everything; libre-enroth passes a *Rand to what needs it so
// that runs are reproducible.
type Rand struct{ state uint32 }

// NewRand seeds a generator like srand(seed); the runtime starts with seed 1.
func NewRand(seed uint32) *Rand { return &Rand{state: seed} }

// Int returns the next value, 0..0x7fff.
//
// mm8: 0x4dc832 (_rand)
func (r *Rand) Int() int {
	r.state = r.state*0x343fd + 0x269ec3
	return int(r.state >> 16 & 0x7fff)
}
