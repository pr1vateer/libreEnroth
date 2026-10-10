package monsters

import (
	"math"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// The AI (re/notes/monsters.md, "AI"): Actors_UpdateAI 0x401ad8 runs once per frame
// before the world tick. It lists the actors near the party, finds each one's target,
// and when an actor's action is over (or a hostile one is ready and in reach) picks
// its next state: stand, wander, pursue, flee or attack. The movers (move.go) then
// walk the actors along their yaw, and the melee blows queued here land at the end
// of the world tick (hit.go).

// Pick id kinds (id << 3 | kind).
const (
	KindObject     = 2
	KindActor      = 3
	KindParty      = 4
	KindDecoration = 5
	KindFace       = 6
)

// PID is the pick id of actor i.
func PID(i int) uint32 { return uint32(i)<<3 | KindActor }

// partyPID is the party as a target (member 0).
const partyPID = KindParty

// More actor flags the AI keeps (Actor.Flags).
const (
	flagBit4       = 0x4       // cleared when an actor turns hostile (Actor_Stun)
	FlagInAIList   = 0x400     // processed by the AI this frame
	FlagNearParty  = 0x4000    // within the AI's reach of the party
	FlagActive     = 0x8000    // has been in the AI list (indoors: seen the party)
	FlagFledHit    = 0x20000   // hit while fleeing: no more thoughts of flight
	flagBit40000   = 0x40000   // cleared by every attack decision
	FlagHostileNow = 0x1000000 // hostile to the party and in the AI list
)

// More actor buffs.
const (
	BuffSummoned        = 2 // removed when it runs out
	BuffAfraid          = 4
	BuffSlowed          = 7  // half speed, double recovery
	BuffFate            = 10 // its power adds to the next to-hit roll
	BuffDayOfProtection = 12
	BuffHourOfPower     = 13 // to-hit, damage and resistance bonus
	BuffShield          = 14 // halves the second attack's damage
	BuffStoneskin       = 15
	BuffBless           = 16
	BuffHeroism         = 17
	BuffHaste           = 18
	BuffPainReflection  = 19
	BuffHammerhands     = 20
	BuffNoSpells        = 22 // only the first attack
	BuffHalfDamage      = 23
	BuffNoFarAttacks    = 24 // no attacks beyond 0x400
	buffsExpired        = 0x1a
)

// AI constants.
const (
	aiListMax      = 30
	aiReachOutdoor = 0x1600 // Actors_ListOutdoor
	aiReachIndoor  = 0x2800 // Actors_ListIndoor
	meleeRange     = 307.2  // 0x4ea430
	fleeRange      = 0x2800
	attackReach    = 0x1400
	shortReach     = 0x400
	// recovery ticks per monsters.txt Rec point (0x4ea438; the ini's [debug] recmod2,
	// "1.0" unless set, multiplies it: 0x6f3900).
	recoveryScale = 2.1333333333333333
	recMod2       = float32(1.0)
)

// relationRadius is how far an actor looks for a target of each hostility level.
//
// mm8: 0x4f2210
var relationRadius = [5]int32{0, 0x400, 0xa00, 0x1400, 0x2800}

// Dir is the direction and distance from one object to another (AIDirection, 9 ints).
type Dir struct {
	V            [3]int32 // +0x00: 16.16 unit vector
	Dist, DistXZ int32    // +0x0c, +0x10
	Yaw, Pitch   int32    // +0x14, +0x18
	From, To     uint32   // +0x1c, +0x20
}

// Party is the party as the monsters see it; the world fills it before each tick.
type Party struct {
	X, Y, Z                  int32
	Height, EyeLevel, Radius int32
	Dir                      int32
	Flying, TurnBased        bool
	Members                  *party.Members
	Ctx                      *party.Ctx
}

// Host is what the AI hands to other parts of the game.
type Host interface {
	// Stub notes, once per key, a part of the original that a later milestone ports.
	Stub(key, what string)
}

// Brain is the AI's state between frames and what it reads: the tables, the map's
// geometry (Indoor or Outdoor), the actors and objects, and the party.
type Brain struct {
	Tables  *tables.Monsters
	Spells  tables.Spells
	SFT     *desc.SFT
	Sprites Sprites
	Rand    items.Rand
	Indoor  *physics.IndoorGeo
	Outdoor *physics.OutdoorGeo
	Party   Party
	Host    Host

	Actors  []Actor
	Objects []Object

	// Near and Close are g_partyFlags 0x20 and 0x10: an actor hostile to the party is
	// within 0x1400 (the rest screen's "monsters nearby") or within melee reach.
	Near, Close bool

	targets [MaxActors]uint32 // 0x508198: each actor's target pid
	list    []int             // 0x50913c: the AI list, nearest first
	attacks attackList        // 0x5214c8
}

// Target is actor i's target pid as of the last tick.
func (b *Brain) Target(i int) uint32 { return b.targets[i] }

// List is the AI list of the last tick.
func (b *Brain) List() []int { return b.list }

func (b *Brain) rand() int { return b.Rand.Int() }

func (b *Brain) stub(key, what string) {
	if b.Host != nil {
		b.Host.Stub(key, what)
	}
}

// Sound is Actor_PlaySound: sound kind 0 attack, 1 die, 2 got hit, 3 fidget (M11).
//
// mm8: 0x402e43 (Actor_PlaySound)
func (b *Brain) sound(i, kind int) {}

// frameTicks is a sprite sequence's length in timer ticks (SFT length << 3).
func (b *Brain) frameTicks(frame int16) uint16 {
	if b.SFT == nil || frame < 0 || int(frame) >= len(b.SFT.Frames) {
		return 0
	}
	return uint16(int16(b.SFT.Frames[frame].Length) << 3)
}

// approxDist is the AI's distance estimate: max + mid * 11/32 + min / 4.
//
// mm8: 0x46155d (and inline in 0x40151a / 0x40172e)
func approxDist(dx, dy, dz int32) int32 {
	a, b, c := abs32(dx), abs32(dy), abs32(dz)
	hi, mid, lo := a, b, c
	if hi < mid {
		hi, mid = mid, hi
	}
	if hi < lo {
		hi, lo = lo, hi
	}
	if mid < lo {
		mid, lo = lo, mid
	}
	return hi + int32(uint32(lo)>>2) + int32(uint32(mid)*11>>5)
}

// ftol is __ftol: truncation toward zero.
func ftol(v float64) int32 { return int32(v) }

// recoveryTicks is the recovery an action takes: monsters.txt's Rec * recmod2 * 2.1333.
func recoveryTicks(rec int32) int32 {
	return ftol(float64(recMod2) * float64(rec) * recoveryScale)
}

// standTicks is how long the decision stands an actor that waits for its recovery
// (ftol of the actor's recovery left * 2.1333).
func standTicks(a *Actor) int32 { return ftol(float64(a.Info.Recovery) * recoveryScale) }

// centreZ is the height the AI aims at on an actor: z + 3/4 of its height
// (ftol(height * -0.75) subtracted).
func (a *Actor) centreZ() int32 {
	return int32(a.Pos[2]) - ftol(float64(int16(a.Height))*-0.75)
}

// aimZ is the same as the attack states compute it: ftol(height * 0.75 + z).
func (a *Actor) aimZ() int32 {
	return ftol(float64(int16(a.Height))*0.75 + float64(a.Pos[2]))
}

// IsKind is Monster_IsKind: whether monsters.txt id belongs to group kind (1..8; 3 are
// the water monsters that do not sink, 4 those that never move).
//
// mm8: 0x436fdc (Monster_IsKind)
func IsKind(id, kind int) bool {
	in := func(lo, hi int) bool { return id >= lo && id <= hi }
	switch kind {
	case 1:
		return in(49, 54) || in(163, 165) || in(172, 174)
	case 2:
		return in(70, 72) || in(136, 138) || in(163, 165) || in(112, 114) || in(190, 192)
	case 3:
		return in(178, 180) || in(76, 78) || in(136, 138)
	case 4:
		return in(142, 144)
	case 5:
		return in(1, 3) || in(19, 21) || in(22, 24) || in(28, 30) || in(49, 51) || in(61, 63) ||
			in(67, 69) || in(166, 168)
	case 6:
		return IsKind(id, 4) || IsKind(id, 3) || IsKind(id, 5)
	case 7:
		return in(31, 33) || in(28, 30) || in(106, 108) || in(58, 60) || in(61, 63) || in(91, 93)
	case 8:
		return in(73, 75) || in(76, 78) || in(79, 81) || in(82, 84) || in(157, 159) ||
			in(130, 132) || in(115, 117) || in(127, 129) || in(103, 105)
	}
	return false
}

// allyClass is the hostile.txt class an actor goes by: its ally, else its monster's.
func (a *Actor) allyClass() int {
	if a.Ally != 0 {
		return int(a.Ally)
	}
	return tables.Class(int(a.Info.ID))
}

// IsPeasant reports the peasant classes (the party walks through them unless they are
// hostile).
//
// mm8: 0x436f99 (Actor_IsPeasantKind)
func (a *Actor) IsPeasant() bool {
	switch a.allyClass() {
	case 1, 7, 8, 10, 0x11, 0x15, 0x17, 0x38:
		return true
	}
	return false
}

// IsAlly reports actors of one kind: the same class, or both of one of the peasant
// families.
//
// mm8: 0x4391ca (Actor_IsAlly)
func IsAlly(a, b *Actor) bool {
	ca, cb := a.allyClass(), b.allyClass()
	both := func(lo, hi int) bool { return ca >= lo && ca <= hi && cb >= lo && cb <= hi }
	return both(1, 1) || both(7, 8) || both(16, 16) || both(21, 21) || both(23, 23) ||
		both(56, 56) || both(60, 60) || ca == cb
}

// expireBuff is SpellBuff_Expire: a buff whose time has passed is cleared (its caster
// and flags stay).
//
// mm8: 0x4572dc (SpellBuff_Expire)
func (bf *Buff) expire(now int64) {
	if bf.Expires != 0 && now > bf.Expires {
		bf.Expires, bf.Power, bf.Skill, bf.Overlay = 0, 0, 0, 0
	}
}

// Clear is SpellBuff_Clear.
//
// mm8: 0x457297 (SpellBuff_Clear)
func (bf *Buff) Clear() { *bf = Buff{} }

// now is the game time the buffs expire by.
func (b *Brain) now() int64 {
	if b.Party.Members == nil {
		return 0
	}
	return int64(b.Party.Members.Time)
}

// expireBuffs ticks an actor's first 26 buffs but mass distortion; a shrink that ran
// out restores the height and a charm that ran out the hostility.
//
// mm8: 0x401ad8 (both loops)
func (b *Brain) expireBuffs(a *Actor) (summonGone bool) {
	shrunk, charmed, summoned := a.Active(BuffShrink), a.Active(BuffCharm), a.Active(BuffSummoned)
	now := b.now()
	for k := range buffsExpired {
		if k != BuffMassDistortion {
			a.Buffs[k].expire(now)
		}
	}
	if shrunk && !a.Active(BuffShrink) {
		if k := int(a.Info.ID) - 1; k >= 0 && k < len(b.Tables.MonList) {
			a.Height = b.Tables.MonList[k].Height
		}
	}
	if charmed {
		a.Info.Hostility = 0
		if !a.Active(BuffCharm) {
			if in := b.Tables.Info(int(a.Info.ID)); in != nil {
				a.Info.Hostility = in.Hostility
			}
		}
	}
	return summoned && !a.Active(BuffSummoned)
}

// listOutdoor builds the AI list outdoors: the actors able to act within 0x1600 of
// the party (their radius off), nearest first, at most 30. It also notes whether an
// actor hostile to the party is near (0x1400) or close (307.2).
//
// mm8: 0x40151a (Actors_ListOutdoor)
func (b *Brain) listOutdoor() {
	b.Near, b.Close = false, false
	var cand []int
	var dist []int32
	for i := range b.Actors {
		a := &b.Actors[i]
		a.Flags &^= FlagInAIList
		if !a.CanAct() {
			a.Flags &^= FlagNearParty
			continue
		}
		d := b.partyDist(a)
		if d >= aiReachOutdoor {
			a.Flags &^= FlagNearParty
			continue
		}
		b.markHostile(a, d)
		a.Flags |= FlagNearParty
		cand, dist = append(cand, i), append(dist, d)
	}
	exchangeSort(cand, dist)
	b.list = append(b.list[:0], cand[:min(len(cand), aiListMax)]...)
	for _, i := range b.list {
		b.Actors[i].Flags |= FlagInAIList
	}
}

// partyDist is the AI's distance from the party to an actor's surface (>= 0).
func (b *Brain) partyDist(a *Actor) int32 {
	p := &b.Party
	d := approxDist(p.X-int32(a.Pos[0]), p.Y-int32(a.Pos[1]), p.Z-int32(a.Pos[2])) - int32(int16(a.Radius))
	return max(d, 0)
}

// markHostile sets FlagHostileNow on an actor hostile to the party and notes the
// party flags.
func (b *Brain) markHostile(a *Actor, d int32) {
	a.Flags &^= FlagHostileNow
	if a.Flags&FlagHostile != 0 || Relation(b.Tables, a, nil) != 0 {
		a.Flags |= FlagHostileNow
		if float64(d) < meleeRange {
			b.Close = true
		}
		if d < attackReach {
			b.Near = true
		}
	}
}

// exchangeSort is the lists' sort: each slot takes the smallest distance after it,
// swapping as it goes (not stable).
func exchangeSort(idx []int, dist []int32) {
	for i := range idx {
		for j := i + 1; j < len(idx); j++ {
			if dist[j] < dist[i] {
				idx[i], idx[j] = idx[j], idx[i]
				dist[i], dist[j] = dist[j], dist[i]
			}
		}
	}
}

// listIndoor builds the AI list indoors: of the actors able to act within 0x2800,
// nearest first, those that see the party (or have been active before), up to 30;
// then every actor in the party's sector; then the active ones of the rest; at most 30.
//
// mm8: 0x40172e (Actors_ListIndoor)
func (b *Brain) listIndoor() {
	b.Near, b.Close = false, false
	p := &b.Party
	sector := b.Indoor.SectorAt(p.X, p.Y, p.Z)
	var cand []int
	var dist []int32
	for i := range b.Actors {
		a := &b.Actors[i]
		a.Flags &^= FlagInAIList
		if !a.CanAct() {
			a.Flags &^= FlagNearParty
			continue
		}
		d := b.partyDist(a)
		if d >= aiReachIndoor {
			a.Flags &^= FlagNearParty
			continue
		}
		b.markHostile(a, d)
		cand, dist = append(cand, i), append(dist, d)
	}
	exchangeSort(cand, dist)
	list := b.list[:0]
	for _, i := range cand {
		a := &b.Actors[i]
		if a.Flags&FlagActive != 0 || b.CanSee(PID(i), partyPID) {
			a.Flags |= FlagActive
			list = append(list, i)
			if len(list) >= aiListMax {
				break
			}
		}
	}
	in := func(l []int, i int) bool {
		for _, j := range l {
			if j == i {
				return true
			}
		}
		return false
	}
	seen := len(list)
	for i := range b.Actors {
		a := &b.Actors[i]
		if a.CanAct() && int(a.Sector) == sector && !in(list[:seen], i) {
			a.Flags |= FlagNearParty
			list = append(list, i)
		}
	}
	seen = len(list)
	for _, i := range cand {
		a := &b.Actors[i]
		if a.Flags&(FlagNearParty|FlagActive) != 0 && a.CanAct() && !in(list[:seen], i) {
			list = append(list, i)
		}
	}
	b.list = list[:min(len(list), aiListMax)]
	for _, i := range b.list {
		b.Actors[i].Flags |= FlagInAIList
	}
}

// Tick runs the AI for a frame of dt timer ticks (not in turn-based mode, which is
// M9's). Actors outside the AI list only play out their action; those in it think.
//
// mm8: 0x401ad8 (Actors_UpdateAI)
func (b *Brain) Tick(dt int32) {
	if b.Indoor != nil {
		b.listIndoor()
	} else {
		b.listOutdoor()
	}
	// 0xbb2d04 (Armageddon shaking the actors and the party) is M9's.
	for i := range b.Actors {
		a := &b.Actors[i]
		b.targets[i] = partyPID
		s := a.AIState
		if s == Dead || s == Removed || s == Disabled || a.Flags&FlagInAIList != 0 {
			continue
		}
		if a.HP == 0 && s != Dying {
			b.Die(i)
		}
		b.expireBuffs(a)
		if a.Active(BuffStoned) || a.Active(BuffParalysed) {
			continue
		}
		a.ActionTime += dt
		b.tickRecovery(a, dt)
		if int32(int16(a.ActionLength)) > a.ActionTime {
			continue
		}
		switch a.AIState {
		case Dying:
			a.AIState = Dead
		case Summoned:
			a.AIState = Stand
		default:
			b.StandOrBored(i, partyPID, 0x100, nil)
			continue
		}
		a.ActionTime, a.ActionLength = 0, 0
		a.UpdateAnimation(b.Sprites)
	}
	for _, i := range b.list {
		b.think(i, dt)
	}
}

func (b *Brain) tickRecovery(a *Actor, dt int32) {
	if a.Info.Recovery > 0 {
		a.Info.Recovery -= dt
	}
	if a.Info.Recovery < 0 {
		a.Info.Recovery = 0
	}
}

// think is the AI list's part of Actors_UpdateAI for actor i: its target, a finished
// action's effect (the melee blow, a missile, a spell), the hostility, then the next
// state.
//
// mm8: 0x401ad8 (the second loop)
func (b *Brain) think(i int, dt int32) {
	a := &b.Actors[i]
	b.targets[i] = b.FindTarget(i, true)
	target := b.targets[i]
	if a.Info.Hostility != 0 && target == 0 {
		a.Info.Hostility = 0
	}
	kind := target & 7
	scale := 1.0
	if kind == KindActor {
		scale = 0.5
	}
	melee := scale * meleeRange
	switch a.AIState {
	case Dying, Dead, Removed, Disabled, Summoned:
		return
	}
	if a.HP == 0 {
		b.Die(i)
	}
	if b.expireBuffs(a) {
		a.AIState = Removed
		return
	}
	if a.Active(BuffStoned) || a.Active(BuffParalysed) {
		return
	}
	a.ActionTime += dt
	b.tickRecovery(a, dt)
	a.Flags |= FlagActive
	pid := PID(i)
	dir := b.Direction(pid, target, 0)
	s := a.AIState
	ready := a.Info.Hostility != 0 && a.Info.Recovery <= 0 &&
		(float64(dir.Dist) <= melee && (s == Pursue || s == Stand || s == Tethered || s == Fidget) ||
			a.Info.Attack1.Missile != 0 && s == Stunned)
	if !ready {
		if a.ActionTime < int32(int16(a.ActionLength)) {
			return
		}
		switch s {
		case Melee:
			atk := b.ChooseAttack(i, dir.Dist)
			b.attacks.add(pid, attackReach, int32(a.Pos[0]), int32(a.Pos[1]),
				int32(int16(a.Height)>>1)+int32(a.Pos[2]), atk, 1)
		case Ranged1:
			b.launch(i, &dir, a.Info.Attack1.Missile, false)
		case Ranged2:
			b.launch(i, &dir, a.Info.Attack2.Missile, true)
		case Ranged3:
			b.cast(i, &dir, a.Info.Spell1, 2, a.Info.Spell1Skill)
		case Ranged4:
			b.cast(i, &dir, a.Info.Spell2, 3, a.Info.Spell2Skill)
		}
	}
	b.updateHostility(a, target, &dir)
	b.decide(i, target, &dir, melee)
}

// launch and cast are the ranged and spell attacks' release: the projectile is M9's.
//
// mm8: 0x404bc6 (Actor_LaunchMissile), 0x404e2f (Actor_CastSpell)
func (b *Brain) launch(i int, dir *Dir, missile uint8, second bool) {
	b.stub("launch", "Actor_LaunchMissile 0x404bc6: monster missiles (M9)")
}

func (b *Brain) cast(i int, dir *Dir, spell uint8, slot int, skill uint16) {
	b.stub("cast", "Actor_CastSpell 0x404e2f: monster spells (M9)")
}

// updateHostility: an actor not hostile yet turns hostile (4) when its target is near
// enough for its feelings: hostile.txt between the classes for an actor target (an
// unallied actor against any but its own kind, or against an unallied one hostile to
// the party, at 4), 4 for the party; 1 always, 2 within 0x400, 3 within 0xa00, 4
// within 0x1400.
//
// mm8: 0x401ad8 (0x40224e)
func (b *Brain) updateHostility(a *Actor, target uint32, dir *Dir) {
	if a.Info.Hostility != 0 {
		return
	}
	rel := 4
	if target&7 == KindActor {
		t := &b.Actors[target>>3]
		rel = int(b.Tables.HostileAt(tables.Class(int(a.Info.ID)), tables.Class(int(t.Info.ID))))
		if a.Ally == 9999 {
			if !IsAlly(a, t) {
				rel = 4
			}
			if t.Ally == 9999 && t.Flags&FlagHostile != 0 {
				rel = 4
			}
		}
	}
	switch {
	case rel == 1, rel == 2 && dir.Dist < 0x400, rel == 3 && dir.Dist < 0xa00, rel == 4 && dir.Dist < attackReach:
		a.Info.Hostility = 4
	}
}

// decide picks the next state of an actor in the AI list (0x402307 on).
func (b *Brain) decide(i int, target uint32, dir *Dir, melee float64) {
	a := &b.Actors[i]
	if a.Active(BuffAfraid) {
		if dir.Dist < fleeRange {
			b.Flee(i, target, 0, dir)
		} else {
			b.RandomMove(i, target, 0x400, 0)
		}
		return
	}
	if a.Info.Hostility != 4 || target == 0 {
		b.wander(i)
		return
	}
	if a.Flags&FlagFledHit == 0 || a.Info.AI == tables.AIWimp {
		flee := false
		switch a.Info.AI {
		case tables.AIWimp:
			if a.Info.Move != tables.MoveStationary {
				flee = true
			} else {
				b.Stand(i, target, standTicks(a), dir) // and it still decides below
			}
		case tables.AINormal:
			flee = float64(a.HP) < float64(a.Info.HP)*float64(float32(0.2)) && dir.Dist < fleeRange
		case tables.AIAggressive:
			flee = float64(a.HP) < float64(a.Info.HP)*float64(float32(0.1)) && dir.Dist < fleeRange
		}
		if flee {
			b.Flee(i, target, 0, dir)
			return
		}
	}
	d := dir.Dist - int32(int16(a.Radius))
	if target&7 == KindActor {
		d -= int32(int16(b.Actors[target>>3].Radius))
	}
	d = max(d, 0)
	b.rand()
	canAttack := a.Info.Recovery <= 0
	a.Flags &^= flagBit40000
	stationary := a.Info.Move == tables.MoveStationary
	if d >= attackReach || a.Active(BuffNoFarAttacks) && d > shortReach {
		b.wander(i)
		return
	}
	inMelee := float64(d) < melee
	standRec := func() { b.Stand(i, target, standTicks(a), dir) }
	meleeOrWait := func() { // 0x402632
		if canAttack {
			b.MeleeAttack(i, target, dir)
		} else {
			standRec()
		}
	}
	approach := func(far int32) { // 0x402654 / 0x40253b / 0x402677
		switch {
		case stationary:
			standRec()
		case d < shortReach:
			b.PursueClose(i, target, 0, dir, ftol(melee))
		default:
			b.Pursue(i, target, far, dir)
		}
	}
	rangedWait := func() { // 0x4025d4
		if stationary || float64(d) < melee {
			standRec()
		} else {
			b.Circle(i, target, i, standTicks(a), dir)
		}
	}
	switch atk := b.ChooseAttack(i, d); atk {
	case 0:
		switch {
		case a.Info.Attack1.Missile != 0 && canAttack:
			b.RangedAttack1(i, target, dir)
		case a.Info.Attack1.Missile != 0:
			rangedWait()
		case inMelee:
			meleeOrWait()
		default:
			approach(0)
		}
	case 1:
		switch {
		case a.Info.Attack2.Missile != 0 && canAttack:
			b.RangedAttack2(i, target, dir)
		case a.Info.Attack2.Missile != 0:
			rangedWait()
		case inMelee:
			meleeOrWait()
		default:
			approach(0x100)
		}
	case 2, 3:
		spell := a.Info.Spell1
		if atk == 3 {
			spell = a.Info.Spell2
		}
		switch {
		case spell == 0 && inMelee:
			meleeOrWait()
		case spell == 0:
			approach(0x100)
		case canAttack && atk == 2:
			b.SpellAttack1(i, target, dir)
		case canAttack:
			b.SpellAttack2(i, target, dir)
		default:
			rangedWait()
		}
	default:
		b.wander(i)
	}
}

// wander is the decision without a foe: walk about the start point as far as the
// monster's Move allows, or stand facing the party when stationary.
//
// mm8: 0x401ad8 (0x402696)
func (b *Brain) wander(i int) {
	a := &b.Actors[i]
	switch a.Info.Move {
	case tables.MoveShort:
		b.RandomMove(i, partyPID, 0x400, 0)
	case tables.MoveMedium:
		b.RandomMove(i, partyPID, 0xa00, 0)
	case tables.MoveLong:
		b.RandomMove(i, partyPID, 0x1400, 0)
	case tables.MoveFree:
		b.RandomMove(i, partyPID, 0x2800, 0)
	case tables.MoveStationary:
		dir := b.Direction(PID(i), partyPID, 0)
		b.Stand(i, partyPID, standTicks(a), &dir)
	}
}

// FindTarget is the nearest foe actor i sees within the radius of its feeling (the
// monster's hostility once it is hostile), or the party when it is nearer and
// canParty (an invisible party is never a target); an actor that hit it last is a
// foe unless it is gone, of its group or of its kind. 0: none.
//
// mm8: 0x401237 (Actor_FindTarget)
func (b *Brain) FindTarget(i int, canParty bool) uint32 {
	a := &b.Actors[i]
	lastHit := uint32(a.LastHitBy)
	best, bestJ := uint32(0xffffffff), 0
	var target uint32
	feeling := func(rel int) int32 {
		if a.Info.Hostility != 0 {
			if in := b.Tables.Info(int(a.Info.ID)); in != nil {
				rel = int(in.Hostility)
			}
		}
		if rel < 0 || rel >= len(relationRadius) {
			return 0
		}
		return relationRadius[rel]
	}
	for j := range b.Actors {
		t := &b.Actors[j]
		switch t.AIState {
		case Dead, Dying, Removed, Summoned, Disabled:
			continue
		}
		if j == i {
			continue
		}
		var rel int
		if lastHit == 0 || lastHit != PID(j) {
			if rel = Relation(b.Tables, a, t); rel == 0 {
				continue
			}
		} else {
			switch {
			case t.Gone(true):
				lastHit, a.LastHitBy = 0, 0
				if rel = Relation(b.Tables, a, t); rel == 0 {
					continue
				}
			case (t.Group != 0 || a.Group != 0) && t.Group == a.Group:
				continue
			case IsAlly(a, t):
				lastHit, a.LastHitBy = 0, 0
				if rel = Relation(b.Tables, a, t); rel == 0 {
					continue
				}
			default:
				rel = 4
			}
		}
		r := feeling(rel)
		dx := abs32(int32(a.Pos[0]) - int32(t.Pos[0]))
		dy := abs32(int32(a.Pos[1]) - int32(t.Pos[1]))
		dz := abs32(int32(a.Pos[2]) - int32(t.Pos[2]))
		if dx <= r && dy <= r && dz <= r && b.CanSee(PID(j), PID(i)) {
			if d := uint32(dz*dz + dy*dy + dx*dx); d < best {
				best, bestJ = d, j
			}
		}
	}
	if best != 0xffffffff {
		target = PID(bestJ)
	}
	if b.Party.Members != nil && b.Party.Members.Buffs[party.PartyBuffInvisibility].Active() {
		canParty = false
	}
	if !canParty {
		return target
	}
	rel := Relation(b.Tables, a, nil)
	if a.Flags&FlagHostile != 0 && !a.Active(BuffEnslaved) && !a.Active(BuffCharm) && !a.Active(BuffSummoned) {
		rel = 4
	}
	if rel == 0 {
		return target
	}
	r := int32(0x2800)
	if a.Info.Hostility == 0 {
		r = relationRadius[min(rel, len(relationRadius)-1)]
	}
	p := &b.Party
	dx := abs32(int32(a.Pos[0]) - p.X)
	dy := abs32(int32(a.Pos[1]) - p.Y)
	dz := abs32(int32(a.Pos[2]) - p.Z)
	if dx <= r && dy <= r && dz <= r && uint32(dz*dz+dy*dy+dx*dx) < best {
		target = partyPID
	}
	return target
}

// memberOffsets place a party member's pid around the party centre: by party slot,
// a distance and a side (Party_MemberPos 0x4390b3 with the party's direction).
var memberOffsets = [5]struct{ dist, side int32 }{{0, 0}, {0x18, 1}, {8, 1}, {8, -1}, {0x18, -1}}

// offsetPoint is the point dist away from (x, y, z) at yaw and pitch.
//
// mm8: 0x4390b3
func offsetPoint(dist, yaw, pitch, x, y, z int32) (int32, int32, int32) {
	h := physics.Mul16(dist, physics.Cos(pitch))
	return x + physics.Mul16(h, physics.Cos(yaw)), y + physics.Mul16(h, physics.Sin(yaw)),
		z + physics.Mul16(dist, physics.Sin(pitch))
}

// fromPos is where a direction starts from object pid: an actor at 3/4 of its height,
// the party (or a member beside it) at a third of its height, an indoor face's
// bounding box centre.
func (b *Brain) fromPos(pid uint32) (x, y, z int32) {
	n := int(pid >> 3)
	p := &b.Party
	switch pid & 7 {
	case KindObject:
		if n < len(b.Objects) {
			o := &b.Objects[n]
			return o.Pos[0], o.Pos[1], o.Pos[2]
		}
	case KindActor:
		if n < len(b.Actors) {
			a := &b.Actors[n]
			return int32(a.Pos[0]), int32(a.Pos[1]), a.centreZ()
		}
	case KindParty:
		slot := b.memberSlot(n)
		z := p.Height/3 + p.Z
		if slot <= 0 || slot >= len(memberOffsets) {
			return p.X, p.Y, z
		}
		o := memberOffsets[slot]
		return offsetPoint(o.dist, p.Dir+o.side*physics.QuarterTurn, 0, p.X, p.Y, z)
	case KindDecoration:
		return b.decorationPos(n)
	case KindFace:
		return b.faceCentre(n)
	}
	return 0, 0, 0
}

// toPos is where a direction ends at object pid: the party at prefZ (its eye level
// when 0) above its feet; the rest as fromPos (members at the party).
func (b *Brain) toPos(pid uint32, prefZ int32) (x, y, z int32) {
	if pid&7 == KindParty {
		p := &b.Party
		if prefZ == 0 {
			prefZ = p.EyeLevel
		}
		return p.X, p.Y, prefZ + p.Z
	}
	return b.fromPos(pid)
}

// memberSlot is the party slot whose roster id is n (Actor_GetDirectionInfo searches
// g_partyRoster), -1 when none.
func (b *Brain) memberSlot(n int) int {
	if b.Party.Members == nil {
		return -1
	}
	for k := range b.Party.Members.Players {
		if b.Party.Members.Players[k].RosterID == n {
			return k
		}
	}
	return -1
}

func (b *Brain) decorationPos(n int) (int32, int32, int32) {
	var decs []physics.Decoration
	switch {
	case b.Indoor != nil:
		decs = b.Indoor.Decorations
	case b.Outdoor != nil:
		decs = b.Outdoor.Decorations
	}
	if n < len(decs) {
		return decs[n].X, decs[n].Y, decs[n].Z
	}
	return 0, 0, 0
}

func (b *Brain) faceCentre(n int) (int32, int32, int32) {
	if b.Indoor == nil || n >= len(b.Indoor.Map.Faces) {
		return 0, 0, 0
	}
	bb := b.Indoor.Map.Faces[n].BBox
	return (int32(bb[1]) + int32(bb[0])) >> 1, (int32(bb[3]) + int32(bb[2])) >> 1, (int32(bb[5]) + int32(bb[4])) >> 1
}

// Direction is the direction and distance from object from to object to (prefZ: the
// height above the party's feet to aim at, 0 its eye level), in the original's x87
// arithmetic: the deltas and their squares as floats, the length as a double.
//
// mm8: 0x4043dc (Actor_GetDirectionInfo)
func (b *Brain) Direction(from, to uint32, prefZ int32) Dir {
	x1, y1, z1 := b.fromPos(from)
	x2, y2, z2 := b.toPos(to, prefZ)
	dx, dy, dz := float32(x2-x1), float32(y2-y1), float32(z2-z1)
	dx2 := float32(float64(dx) * float64(dx))
	dy2 := float32(float64(dy) * float64(dy))
	dist := math.Sqrt(float64(dz)*float64(dz) + float64(dy2) + float64(dx2))
	d := Dir{From: from, To: to}
	if dist <= 1.0 {
		d.V = [3]int32{0x10000, 0, 0}
		d.Dist, d.DistXZ = 1, 1
		return d
	}
	d.V = [3]int32{ftol(float64(dx) / dist * 65536), ftol(float64(dy) / dist * 65536), ftol(float64(dz) / dist * 65536)}
	d.Dist = ftol(dist)
	d.DistXZ = ftol(math.Sqrt(float64(dy2) + float64(dx2)))
	d.Yaw = physics.Atan2(ftol(float64(dx)), ftol(float64(dy)))
	d.Pitch = physics.Atan2(d.DistXZ, ftol(float64(dz)))
	return d
}

// facing is a direction that keeps the actor's heading, for the original's calls that
// pass an uninitialised direction (RandomMove; see re/notes/monsters.md).
func facing(a *Actor) *Dir { return &Dir{Yaw: int32(a.Yaw), Pitch: int32(a.Pitch)} }

// dirOr returns dir, or the direction from actor i to target when dir is nil.
func (b *Brain) dirOr(i int, target uint32, dir *Dir, prefZ int32) *Dir {
	if dir != nil {
		return dir
	}
	d := b.Direction(PID(i), target, prefZ)
	return &d
}

// still stops an actor and starts a new action of length n in state s.
func (b *Brain) still(a *Actor, s int16, n uint16) {
	a.ActionTime, a.AIState = 0, s
	a.ActionLength = n
	a.Vel = [3]int16{}
}

// Stand turns actor i to its target and stands for length ticks (0: 256..511).
//
// mm8: 0x4041a9 (Actor_Stand)
func (b *Brain) Stand(i int, target uint32, length int32, dir *Dir) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, 0)
	a.Yaw = uint16(dir.Yaw)
	if length == 0 {
		length = int32(b.rand()%0x100 + 0x100)
	}
	b.still(a, Stand, uint16(length))
	a.UpdateAnimation(b.Sprites)
}

// StandShort stands an actor for 128..255 ticks where it faces.
//
// mm8: 0x404153 (Actor_StandShort)
func (b *Brain) StandShort(i int) {
	a := &b.Actors[i]
	r := b.rand()
	b.still(a, Stand, uint16(r%0x80+0x80))
	a.UpdateAnimation(b.Sprites)
}

// StandOrBored: half the time the actor faces its target and stands, else Bored.
//
// mm8: 0x40424b (Actor_StandOrBored)
func (b *Brain) StandOrBored(i int, target uint32, length int32, dir *Dir) {
	a := &b.Actors[i]
	if b.rand()%2 != 0 {
		b.Bored(i, target, dir)
		return
	}
	dir = b.dirOr(i, target, dir, 0)
	a.Yaw, a.Pitch = uint16(dir.Yaw), uint16(dir.Pitch)
	if length == 0 {
		length = int32(b.rand()%0x100 + 0x100)
	}
	b.still(a, Stand, uint16(length))
	a.UpdateAnimation(b.Sprites)
}

// Bored: the actor faces its target; when the view then shows its front, it fidgets
// for its fidget animation's length, otherwise it stands as long.
//
// mm8: 0x403207 (Actor_Bored)
func (b *Brain) Bored(i int, target uint32, dir *Dir) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, 0)
	a.Yaw = uint16(dir.Yaw)
	a.ActionLength = b.frameTicks(a.Sprites[tables.AnimFidget])
	ang := physics.Atan2(int32(a.Pos[0])-b.Party.X, int32(a.Pos[1])-b.Party.Y)
	if ((physics.HalfTurn>>3)+int32(a.Yaw)-ang+physics.HalfTurn)&0x700 != 0 {
		b.Stand(i, target, int32(int16(a.ActionLength)), dir)
		return
	}
	a.ActionTime, a.AIState = 0, Fidget
	a.Vel = [3]int16{}
	if b.rand()%100 < 5 {
		b.sound(i, 3)
	}
	a.UpdateAnimation(b.Sprites)
}

// FaceObject turns the actor to what it bumped into (an actor or the friendly party)
// for 256 ticks; 5% of the time it is bored instead.
//
// mm8: 0x404323 (Actor_FaceObject)
func (b *Brain) FaceObject(i int, target uint32, dir *Dir) {
	a := &b.Actors[i]
	if b.rand()%100 < 5 {
		b.Bored(i, target, dir)
		return
	}
	dir = b.dirOr(i, target, dir, 0)
	a.Yaw, a.Pitch = uint16(dir.Yaw), uint16(dir.Pitch)
	b.still(a, Interacting, 0x100)
	a.UpdateAnimation(b.Sprites)
}

// pursuitZ is the height a flier aims at a walking party: its height, or for a flier
// with a missile its radius + 0x200 (outdoors only, unless any).
func (b *Brain) pursuitZ(a *Actor, anyMap bool) int32 {
	if a.Info.Fly == 0 || b.Party.Flying {
		return 0
	}
	if a.Info.Attack1.Missile != 0 && (anyMap || b.Outdoor != nil) {
		return int32(int16(a.Radius)) + 0x200
	}
	return b.Party.Height
}

// walkTicks is how long a walk of dist takes at the actor's speed, up to cap.
func walkTicks(a *Actor, dist int32, cap int16) uint16 {
	var n uint16
	if a.Speed != 0 {
		n = uint16((dist << 7) / int32(int16(a.Speed)))
	}
	if int16(n) > cap {
		n = uint16(cap)
	}
	return n
}

// Pursue walks the actor toward its target 45 degrees off to either side, for the
// time the distance takes (at most 128 ticks), or rand() % length + length; near
// (307.2) or for a monster that never moves it stands instead.
//
// mm8: 0x4027d6 (Actor_Pursue)
func (b *Brain) Pursue(i int, target uint32, length int32, dir *Dir) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, b.pursuitZ(a, false))
	if IsKind(int(a.Info.ID), 4) {
		b.StandOrBored(i, partyPID, or(length, 0x100), dir)
		return
	}
	if float64(dir.Dist) < meleeRange {
		b.StandOrBored(i, target, or(length, 0x100), dir)
		return
	}
	if length == 0 {
		a.ActionLength = walkTicks(a, dir.DistXZ, 0x80)
	} else {
		a.ActionLength = uint16(int32(b.rand())%length + length)
	}
	if b.rand()%2 == 0 {
		a.Yaw = uint16(dir.Yaw - 0x100)
	} else {
		a.Yaw = uint16(dir.Yaw + 0x100)
	}
	a.ActionTime, a.Pitch, a.AIState = 0, uint16(dir.Pitch), Pursue
	if b.rand()%100 < 2 {
		b.sound(i, 2)
	}
	a.UpdateAnimation(b.Sprites)
}

func or(v, d int32) int32 {
	if v == 0 {
		return d
	}
	return v
}

// PursueClose walks straight at the target (at most 32 ticks, or length) while it is
// at least minDist away; nearer it stands.
//
// mm8: 0x40296a (Actor_PursueClose)
func (b *Brain) PursueClose(i int, target uint32, length int32, dir *Dir, minDist int32) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, b.pursuitZ(a, false))
	if IsKind(int(a.Info.ID), 4) {
		b.StandOrBored(i, partyPID, or(length, 0x100), dir)
		return
	}
	if dir.Dist < minDist {
		b.StandOrBored(i, target, or(length, 0x100), dir)
		return
	}
	if length == 0 {
		a.ActionLength = walkTicks(a, dir.DistXZ, 0x20)
	} else {
		a.ActionLength = uint16(length)
	}
	a.Yaw = uint16(dir.Yaw)
	a.ActionTime, a.Pitch, a.AIState = 0, uint16(dir.Pitch), Pursue
	a.UpdateAnimation(b.Sprites)
}

// Flee runs from the target: away from it, a quarter turn plus up to a half turn
// off its direction, for the distance's time (at most 256 ticks). A monster that never
// moves, or one fleeing an actor while the party is within 307.2, stands instead.
//
// mm8: 0x402ab6 (Actor_Flee)
func (b *Brain) Flee(i int, target uint32, length int32, dir *Dir) {
	a := &b.Actors[i]
	if !a.CanAct() {
		return
	}
	pid := PID(i)
	dir = b.dirOr(i, target, dir, int32(a.Info.Fly))
	toParty := b.Direction(pid, partyPID, 0)
	if IsKind(int(a.Info.ID), 4) || target&7 == KindActor && float64(toParty.Dist) < meleeRange {
		b.StandOrBored(i, partyPID, or(length, 0x100), &toParty)
		return
	}
	a.ActionLength = walkTicks(a, dir.DistXZ, 0x100)
	a.Yaw = uint16(dir.Yaw + physics.QuarterTurn)
	a.Yaw = uint16(int32(a.Yaw)+int32(b.rand()%physics.HalfTurn)) & physics.AngleMask
	a.ActionTime, a.Pitch, a.AIState = 0, uint16(dir.Pitch), Flee
	a.UpdateAnimation(b.Sprites)
}

// Circle moves around the party: toward the point the actor's distance away from the
// party, 16 units of angle to one side (parity picks it), for length ticks (0: 128),
// so that it gets a clear shot; near it stands.
//
// mm8: 0x402c2f (Actor_PursueCircle)
func (b *Brain) Circle(i int, target uint32, parity int, length int32, dir *Dir) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, b.pursuitZ(a, true))
	if IsKind(int(a.Info.ID), 4) {
		b.StandOrBored(i, partyPID, or(length, 0x100), dir)
		return
	}
	if float64(dir.Dist) >= meleeRange {
		if a.Speed != 0 {
			off := int32(0x10)
			if parity%2 != 0 {
				off = -0x10
			}
			ang := dir.Yaw + physics.HalfTurn + off
			px := b.Party.X + physics.Mul16(dir.DistXZ, physics.Cos(ang))
			py := physics.Mul16(dir.DistXZ, physics.Cos(ang-physics.QuarterTurn)) + b.Party.Y
			a.Yaw = uint16(physics.Atan2(px-int32(a.Pos[0]), py-int32(a.Pos[1])))
			a.ActionLength = uint16(or(length, 0x80))
			a.Pitch, a.AIState = uint16(dir.Pitch), Pursue
			a.UpdateAnimation(b.Sprites)
			return
		}
	} else if length == 0 {
		length = 0x100
	}
	b.Stand(i, target, length, dir)
}

// RandomMove walks the actor about its start point: a heading toward it give or take
// 64 units of angle, for the distance home plus up to radius * 15/16; a quarter of the
// time it stands a moment instead, and a turn of more than 256 units from a walk stops
// it. A Global monster within 128 of its start stands.
//
// mm8: 0x403537 (Actor_RandomMove)
func (b *Brain) RandomMove(i int, target uint32, radius int32, length int32) {
	a := &b.Actors[i]
	dx, dy := int32(a.Start[0])-int32(a.Pos[0]), int32(a.Start[1])-int32(a.Pos[1])
	ax, ay := abs32(dx), abs32(dy)
	var dist int32
	if ay < ax {
		dist = ay>>1 + ax
	} else {
		dist = ax>>1 + ay
	}
	// The original passes its uninitialised direction (it tests the local's address
	// for nil); the actor keeps its heading instead.
	dir := facing(a)
	if IsKind(int(a.Info.ID), 4) {
		b.StandOrBored(i, partyPID, or(length, 0x100), dir)
		return
	}
	if a.Info.Move != tables.MoveGlobal || dist > 0x7f {
		r := int32(b.rand()&0xf) << 12
		dist += physics.Mul16(radius, r)
		ang := physics.Atan2(dx, dy) & (physics.FullTurn - 1)
		if b.rand()%100 < 25 {
			b.StandShort(i)
			return
		}
		yaw := ang - 0x80 + int32(b.rand()%0x100)
		if abs32(yaw-int32(int16(a.Yaw))) <= 0x100 || a.Flags&FlagAnimationSet != 0 {
			a.Yaw = uint16(yaw)
			a.ActionLength = 0
			if a.Speed != 0 {
				a.ActionLength = uint16((dist << 5) / int32(int16(a.Speed)))
			}
			a.ActionTime, a.AIState = 0, Tethered
			if b.rand()%100 < 2 {
				b.sound(i, 3)
			}
			a.UpdateAnimation(b.Sprites)
			return
		}
	}
	b.Stand(i, target, 0x100, dir)
}

// Stun: a hit actor turns hostile (losing charm and fear) and, unless it is attacking
// or stunned already (or force), turns to the attacker and plays its got-hit
// animation. One hit while fleeing gives up on fleeing.
//
// mm8: 0x40332f (Actor_Stun)
func (b *Brain) Stun(i int, attacker uint32, force bool) {
	a := &b.Actors[i]
	if a.AIState == Flee {
		a.Flags |= FlagFledHit
	}
	if a.Info.Hostility != 4 {
		a.Flags &^= flagBit4
		a.Info.Hostility = 4
	}
	if a.Active(BuffCharm) {
		a.Buffs[BuffCharm].Clear()
	}
	if a.Active(BuffAfraid) {
		a.Buffs[BuffAfraid].Clear()
	}
	switch a.AIState {
	case Stunned, Ranged1, Ranged2, Ranged3, Ranged4, Melee:
		if !force {
			return
		}
	}
	dir := b.Direction(PID(i), attacker, 0)
	a.Yaw = uint16(dir.Yaw)
	a.ActionTime = 0
	a.ActionLength = b.frameTicks(a.Sprites[tables.AnimGotHit])
	a.AIState = Stunned
	b.sound(i, 2)
	a.UpdateAnimation(b.Sprites)
}

// Die starts an actor's death: the dying animation, no HP, no buffs. The kill flags,
// the reagents and the loot are M8c's.
//
// mm8: 0x402ec9 (Actor_Die, its first part)
func (b *Brain) Die(i int) {
	a := &b.Actors[i]
	a.ActionTime, a.AIState, a.Animation = 0, Dying, tables.AnimDying
	a.HP = 0
	a.ActionLength = b.frameTicks(a.Sprites[tables.AnimDying])
	a.Buffs[BuffParalysed].Clear()
	a.Buffs[BuffStoned].Clear()
	b.sound(i, 1)
	a.UpdateAnimation(b.Sprites)
	for k := range buffsExpired {
		a.Buffs[k].Clear()
	}
	b.stub("die", "Actor_Die 0x402ec9: kill flags, reagents and loot (M8c)")
}

// attackAim is where an attack state aims at its target: an actor at 3/4 of its
// height, the party at its eyes.
func (b *Brain) attackAim(target uint32) (int32, int32, int32) {
	switch target & 7 {
	case KindActor:
		t := &b.Actors[target>>3]
		return int32(t.Pos[0]), int32(t.Pos[1]), t.aimZ()
	case KindParty:
		p := &b.Party
		return p.X, p.Y, p.EyeLevel + p.Z
	}
	return 0, 0, 0
}

// startAttack turns the actor to the target and starts attack state s with
// animation sprite anim; the recovery is the monster's Rec (doubled when slowed),
// in ticks: added to the animation's length when afterAnim, as is outside
// turn-based mode.
func (b *Brain) startAttack(i int, target uint32, dir *Dir, s int16, anim int, afterAnim bool) {
	a := &b.Actors[i]
	dir = b.dirOr(i, target, dir, 0)
	a.Yaw = uint16(dir.Yaw)
	a.ActionLength = b.frameTicks(a.Sprites[anim])
	a.ActionTime, a.AIState = 0, s
	b.sound(i, 0)
	rec := int32(0)
	if in := b.Tables.Info(int(a.Info.ID)); in != nil {
		rec = in.Recovery
	}
	if a.Active(BuffSlowed) {
		rec *= 2
	}
	switch {
	case b.Party.TurnBased:
		a.Info.Recovery = rec
	case afterAnim:
		a.Info.Recovery = int32(int16(a.ActionLength)) + recoveryTicks(rec)
	default:
		a.Info.Recovery = recoveryTicks(rec)
	}
	a.Vel = [3]int16{}
}

// MeleeAttack starts a melee attack when the actor sees its target from 3/4 of its
// height (the blow lands when the animation ends); otherwise it circles for a clear
// view. A stationary wimp only stands.
//
// mm8: 0x403f4e (Actor_MeleeAttack)
func (b *Brain) MeleeAttack(i int, target uint32, dir *Dir) {
	a := &b.Actors[i]
	if a.Info.Move == tables.MoveStationary && a.Info.AI == tables.AIWimp {
		b.Stand(i, target, 0, dir)
		return
	}
	tx, ty, tz := b.attackAim(target)
	if !b.CanSeePoints(tx, ty, tz, int32(a.Pos[0]), int32(a.Pos[1]), a.aimZ()) {
		r := b.rand()
		b.Circle(i, target, r%2, 0x40, dir)
		return
	}
	b.startAttack(i, target, dir, Melee, tables.AnimAttack, false)
	a.UpdateAnimation(b.Sprites)
}

// RangedAttack1 starts the first attack's missile when either end sees the other.
//
// mm8: 0x40373b (Actor_RangedAttack1)
func (b *Brain) RangedAttack1(i int, target uint32, dir *Dir) {
	a := &b.Actors[i]
	tx, ty, tz := b.attackAim(target)
	x, y, z := int32(a.Pos[0]), int32(a.Pos[1]), a.centreZ()
	if !b.CanSeePoints(tx, ty, tz, x, y, z) && !b.CanSeePoints(x, y, z, tx, ty, tz) {
		b.Circle(i, target, i, 0x40, dir)
		return
	}
	b.startAttack(i, target, dir, Ranged1, tables.AnimShoot, true)
	a.UpdateAnimation(b.Sprites)
}

// RangedAttack2 starts the second attack's missile when the target sees the actor.
//
// mm8: 0x403953 (Actor_RangedAttack2)
func (b *Brain) RangedAttack2(i int, target uint32, dir *Dir) {
	a := &b.Actors[i]
	tx, ty, tz := b.attackAim(target)
	if !b.CanSeePoints(tx, ty, tz, int32(a.Pos[0]), int32(a.Pos[1]), a.centreZ()) {
		b.Circle(i, target, i, 0x40, dir)
		return
	}
	b.startAttack(i, target, dir, Ranged2, tables.AnimShoot, false)
	a.UpdateAnimation(b.Sprites)
}

// SpellAttack1 and SpellAttack2 start casting spell 1 or 2; a spell that is cast at a
// target (SpellIsCastAt) shows the fidget animation for 64 ticks.
//
// mm8: 0x403b2c (Actor_SpellAttack1), 0x403d3d (Actor_SpellAttack2)
func (b *Brain) SpellAttack1(i int, target uint32, dir *Dir) {
	b.spellAttack(i, target, dir, Ranged3, b.Actors[i].Info.Spell1)
}

func (b *Brain) SpellAttack2(i int, target uint32, dir *Dir) {
	b.spellAttack(i, target, dir, Ranged4, b.Actors[i].Info.Spell2)
}

func (b *Brain) spellAttack(i int, target uint32, dir *Dir, s int16, spell uint8) {
	a := &b.Actors[i]
	tx, ty, tz := b.attackAim(target)
	if !b.CanSeePoints(tx, ty, tz, int32(a.Pos[0]), int32(a.Pos[1]), a.centreZ()) {
		b.Circle(i, target, i, 0x40, dir)
		return
	}
	b.startAttack(i, target, dir, s, tables.AnimShoot, true)
	if !SpellIsCastAt(int(spell)) {
		a.UpdateAnimation(b.Sprites)
		return
	}
	a.ActionLength, a.ActionTime, a.AIState = 0x40, 0, Fidget
	a.UpdateAnimation(b.Sprites)
	a.AIState = s
}

// SpellIsCastAt reports the spells a monster casts at a target (not on itself).
//
// mm8: 0x42ef35
func SpellIsCastAt(spell int) bool {
	switch spell {
	case 5, 0x11, 0x26, 0x2e, 0x2f, 0x33, 0x49, 0x4d, 0x50, 0x55, 0x56, 0x5f:
		return false
	}
	return true
}

// ChooseAttack picks actor i's attack at distance dist: spell 1 (2) or 2 (3) when
// usable and its Use% roll succeeds, else the second attack (1) on its Att% roll, else
// the first (0). Buff 22 allows only the first.
//
// mm8: 0x425cd8 (Actor_ChooseAttack)
func (b *Brain) ChooseAttack(i int, dist int32) int {
	a := &b.Actors[i]
	if a.Active(BuffNoSpells) {
		return 0
	}
	in := &a.Info
	if in.Special == tables.SpecialSummon && in.SpecialC < 3 && b.rand()%100 < 5 {
		b.stub("summon", "Spawn_Summoned 0x44e7cb: a summoner's call (no shipped monster has one)")
	}
	s1 := b.SpellUsable(a, int(in.Spell1), dist)
	s2 := b.SpellUsable(a, int(in.Spell2), dist)
	if s1 && in.Spell1Chance != 0 && b.rand()%100 < int(in.Spell1Chance) {
		return 2
	}
	if s2 && in.Spell2Chance != 0 && b.rand()%100 < int(in.Spell2Chance) {
		return 3
	}
	if in.Attack2Chance != 0 && b.rand()%100 < int(in.Attack2Chance) {
		return 1
	}
	return 0
}

// SpellUsable: healing only when hurt, a buff only when it is not on, Spirit Lash
// only within 0x180, Dispel Magic only when the party or a member has a buff.
//
// mm8: 0x425db4 (Actor_SpellUsable)
func (b *Brain) SpellUsable(a *Actor, spell int, dist int32) bool {
	buff := -1
	switch spell {
	case 0x44, 0x4d: // Heal, Power Cure
		return int32(a.HP) < a.Info.HP
	case 5:
		buff = BuffHaste
	case 0x11:
		buff = BuffShield
	case 0x26:
		buff = BuffStoneskin
	case 0x2e:
		buff = BuffBless
	case 0x2f:
		buff = BuffFate
	case 0x33:
		buff = BuffHeroism
	case 0x34:
		return dist <= 0x180
	case 0x49:
		buff = BuffHammerhands
	case 0x50:
		return b.partyBuffed()
	case 0x55:
		buff = BuffDayOfProtection
	case 0x56:
		buff = BuffHourOfPower
	case 0x5f:
		buff = BuffPainReflection
	default:
		return true
	}
	return !a.Active(buff)
}

// partyBuffed reports a party buff or a member's buff on.
func (b *Brain) partyBuffed() bool {
	m := b.Party.Members
	if m == nil {
		return false
	}
	for k := range m.Buffs {
		if m.Buffs[k].Active() {
			return true
		}
	}
	for p := range m.Players {
		for k := range m.Players[p].Buffs {
			if m.Players[p].Buffs[k].Active() {
				return true
			}
		}
	}
	return false
}

// HostileNear reports an actor alive within 0x1400 of (x, y, z) (0xa00 indoors) that is
// hostile to the party.
//
// mm8: 0x42e9d7 (Actors_HostileNear)
func HostileNear(t *tables.Monsters, actors []Actor, x, y, z int32, indoor bool) bool {
	r := int32(0x1400)
	if indoor {
		r = 0xa00
	}
	for i := range actors {
		a := &actors[i]
		if approxDist(int32(a.Pos[0])-x, int32(a.Pos[1])-y, int32(a.Pos[2])-z) >= r {
			continue
		}
		switch a.AIState {
		case Dead, Dying, Removed, Disabled, Summoned:
			continue
		}
		if a.Flags&FlagHostile != 0 || Relation(t, nil, a) != 0 {
			return true
		}
	}
	return false
}
