package party

import "libre-enroth/internal/assets/desc"

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
	Face       int                  // +0x353: portrait 0..29
	Voice      int                  // +0x1be4
	Conditions [numConditions]int64 // +0x000: game time each began, 0 = not set
	Recovery   int                  // +0x1bf2: ticks until the player can act again
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

// Members are the party's characters and the selected one.
type Members struct {
	Players  []Player
	Selected int // 1-based, 0 = none (g_selectedPlayer 0x5192a0)
}

// NewGame resets the members for a new game: the first one is selected, conditions and
// recovery are cleared and every face starts on the normal expression.
//
// mm8: 0x49228d (Party_InitNewGame)
func (m *Members) NewGame(rng *Rand) {
	m.Selected = 1
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
