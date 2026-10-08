// Package monsters is the original's Actor.cpp: the actors of a map (their 0x3cc-byte
// records in the .ddm/.dlv), spawning them from the map's spawn points, their animation,
// and the ground objects' records (re/notes/monsters.md).
package monsters

import (
	"bytes"
	"encoding/binary"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// Record sizes and limits.
const (
	ActorSize  = 0x3cc
	ObjectSize = 0x70
	MaxActors  = 500
	MaxObjects = 1000
	NumBuffs   = 30
)

// AI states (Actor.AIState).
const (
	Stand       = 0
	Tethered    = 1 // walking about the start point
	Melee       = 2
	Ranged1     = 3
	Dying       = 4
	Dead        = 5
	Pursue      = 6
	Flee        = 7
	Stunned     = 8
	Fidget      = 9
	Interacting = 10
	Removed     = 0xb
	Ranged2     = 0xc
	Ranged3     = 0xd
	Stoned      = 0xe
	Paralysed   = 0xf
	Resurrected = 0x10
	Summoned    = 0x11
	Ranged4     = 0x12
	Disabled    = 0x13
)

// Actor flags (Actor.Flags).
const (
	FlagDrawn         = 0x8       // drawn this frame
	FlagDisabled      = 0x10000   // with AI state Disabled
	FlagHostile       = 0x80000   // hostile to the party
	FlagAlertOnly     = 0x100000  // only on an alert map
	FlagAnimationSet  = 0x200000  // Actor_UpdateAnimation chose an animation
	FlagLooted        = 0x800000  // the corpse's loot is made
	FlagDatedAppearer = 0x2000000 // out07's placed monster 7
)

// Actor buffs (Actor.Buffs).
const (
	BuffCharm          = 1
	BuffShrink         = 3
	BuffStoned         = 5
	BuffParalysed      = 6
	BuffBerserk        = 8
	BuffMassDistortion = 9
	BuffEnslaved       = 11
)

// Actor_Init's defaults.
const (
	defaultTether = 0x100
	defaultRadius = 0x20
	defaultHeight = 0x80
	defaultSpeed  = 200
)

// Buff is a SpellBuff (0x10 bytes).
type Buff struct {
	Expires int64 // +0x00: active while > 0
	Power   int16 // +0x08
	Skill   int16 // +0x0a
	Overlay int16 // +0x0c
	Caster  uint8 // +0x0e
	Flags   uint8 // +0x0f
}

// Active reports a buff whose expiry time is positive (the original's int64 test).
func (b *Buff) Active() bool { return b.Expires > 0 }

// Job is an NPC schedule entry (0xc bytes).
type Job struct {
	X, Y, Z                  int16  // +0x00
	Attrib                   uint16 // +0x06
	Action, Hour, Day, Month uint8  // +0x08
}

// Actor is a 0x3cc-byte actor record (types.h Actor). Unknown and padding fields are kept
// so that a record encodes back byte for byte.
type Actor struct {
	NameBytes    [32]byte           // +0x000
	NPC          int16              // +0x020
	Unk22        int16              // +0x022
	Flags        uint32             // +0x024
	HP           int16              // +0x028
	Unk2a        int16              // +0x02a
	Info         tables.MonsterInfo // +0x02c: the monsters.txt row (id +0x6a, hostility +0x3d)
	RangeAttack  int16              // +0x08c
	MonList      int16              // +0x08e: dmonlist.bin index + 1
	Radius       uint16             // +0x090
	Height       uint16             // +0x092
	Speed        uint16             // +0x094
	Pos          [3]int16           // +0x096
	Vel          [3]int16           // +0x09c
	Yaw, Pitch   uint16             // +0x0a2, +0x0a4
	Sector       int16              // +0x0a6
	ActionLength uint16             // +0x0a8
	Start        [3]int16           // +0x0aa
	Guard        [3]int16           // +0x0b0
	Tether       uint16             // +0x0b6
	AIState      int16              // +0x0b8
	Animation    uint16             // +0x0ba: tables.Anim*
	CarriedItem  uint16             // +0x0bc
	UnkBE        uint16             // +0x0be
	ActionTime   int32              // +0x0c0
	Sprites      [8]int16           // +0x0c4: sft frames per animation
	Sounds       [4]uint16          // +0x0d4
	Buffs        [NumBuffs]Buff     // +0x0dc
	Items        [4]items.Item      // +0x2bc: two event items, the loot item and gold
	Group        int32              // +0x34c
	Ally         int32              // +0x350
	Jobs         [8]Job             // +0x354
	Summoner     int32              // +0x3b4
	LastHitBy    int32              // +0x3b8
	UniqueName   int32              // +0x3bc: placemon.txt id
	Unk3C0       [12]byte           // +0x3c0
}

// Name is the record's name field (unused by the shipped maps' actors).
func (a *Actor) Name() string {
	n := a.NameBytes[:]
	if i := bytes.IndexByte(n, 0); i >= 0 {
		n = n[:i]
	}
	return string(n)
}

// ID is the monsters.txt id.
func (a *Actor) ID() int { return int(a.Info.ID) }

// DecodeActor reads a 0x3cc-byte record.
func DecodeActor(b []byte) Actor {
	le := binary.LittleEndian
	i16 := func(o int) int16 { return int16(le.Uint16(b[o:])) }
	i32 := func(o int) int32 { return int32(le.Uint32(b[o:])) }
	var a Actor
	copy(a.NameBytes[:], b[:32])
	a.NPC, a.Unk22 = i16(0x20), i16(0x22)
	a.Flags = le.Uint32(b[0x24:])
	a.HP, a.Unk2a = i16(0x28), i16(0x2a)
	a.Info.Decode(b[0x2c:0x8c])
	a.RangeAttack, a.MonList = i16(0x8c), i16(0x8e)
	a.Radius, a.Height, a.Speed = le.Uint16(b[0x90:]), le.Uint16(b[0x92:]), le.Uint16(b[0x94:])
	for k := range 3 {
		a.Pos[k] = i16(0x96 + 2*k)
		a.Vel[k] = i16(0x9c + 2*k)
		a.Start[k] = i16(0xaa + 2*k)
		a.Guard[k] = i16(0xb0 + 2*k)
	}
	a.Yaw, a.Pitch, a.Sector = le.Uint16(b[0xa2:]), le.Uint16(b[0xa4:]), i16(0xa6)
	a.ActionLength = le.Uint16(b[0xa8:])
	a.Tether, a.AIState, a.Animation = le.Uint16(b[0xb6:]), i16(0xb8), le.Uint16(b[0xba:])
	a.CarriedItem, a.UnkBE = le.Uint16(b[0xbc:]), le.Uint16(b[0xbe:])
	a.ActionTime = i32(0xc0)
	for k := range a.Sprites {
		a.Sprites[k] = i16(0xc4 + 2*k)
	}
	for k := range a.Sounds {
		a.Sounds[k] = le.Uint16(b[0xd4+2*k:])
	}
	for k := range a.Buffs {
		r := b[0xdc+0x10*k:]
		a.Buffs[k] = Buff{
			Expires: int64(le.Uint64(r)), Power: int16(le.Uint16(r[8:])), Skill: int16(le.Uint16(r[0xa:])),
			Overlay: int16(le.Uint16(r[0xc:])), Caster: r[0xe], Flags: r[0xf],
		}
	}
	for k := range a.Items {
		a.Items[k] = items.DecodeItem(b[0x2bc+items.ItemSize*k:])
	}
	a.Group, a.Ally = i32(0x34c), i32(0x350)
	for k := range a.Jobs {
		r := b[0x354+0xc*k:]
		a.Jobs[k] = Job{
			X: int16(le.Uint16(r)), Y: int16(le.Uint16(r[2:])), Z: int16(le.Uint16(r[4:])), Attrib: le.Uint16(r[6:]),
			Action: r[8], Hour: r[9], Day: r[0xa], Month: r[0xb],
		}
	}
	a.Summoner, a.LastHitBy, a.UniqueName = i32(0x3b4), i32(0x3b8), i32(0x3bc)
	copy(a.Unk3C0[:], b[0x3c0:0x3cc])
	return a
}

// Encode writes the 0x3cc-byte record.
func (a *Actor) Encode(b []byte) {
	le := binary.LittleEndian
	p16 := func(o int, v int16) { le.PutUint16(b[o:], uint16(v)) }
	p32 := func(o int, v int32) { le.PutUint32(b[o:], uint32(v)) }
	copy(b[:32], a.NameBytes[:])
	p16(0x20, a.NPC)
	p16(0x22, a.Unk22)
	le.PutUint32(b[0x24:], a.Flags)
	p16(0x28, a.HP)
	p16(0x2a, a.Unk2a)
	a.Info.Encode(b[0x2c:0x8c])
	p16(0x8c, a.RangeAttack)
	p16(0x8e, a.MonList)
	le.PutUint16(b[0x90:], a.Radius)
	le.PutUint16(b[0x92:], a.Height)
	le.PutUint16(b[0x94:], a.Speed)
	for k := range 3 {
		p16(0x96+2*k, a.Pos[k])
		p16(0x9c+2*k, a.Vel[k])
		p16(0xaa+2*k, a.Start[k])
		p16(0xb0+2*k, a.Guard[k])
	}
	le.PutUint16(b[0xa2:], a.Yaw)
	le.PutUint16(b[0xa4:], a.Pitch)
	p16(0xa6, a.Sector)
	le.PutUint16(b[0xa8:], a.ActionLength)
	le.PutUint16(b[0xb6:], a.Tether)
	p16(0xb8, a.AIState)
	le.PutUint16(b[0xba:], a.Animation)
	le.PutUint16(b[0xbc:], a.CarriedItem)
	le.PutUint16(b[0xbe:], a.UnkBE)
	p32(0xc0, a.ActionTime)
	for k, v := range a.Sprites {
		p16(0xc4+2*k, v)
	}
	for k, v := range a.Sounds {
		le.PutUint16(b[0xd4+2*k:], v)
	}
	for k, bf := range a.Buffs {
		r := b[0xdc+0x10*k:]
		le.PutUint64(r, uint64(bf.Expires))
		le.PutUint16(r[8:], uint16(bf.Power))
		le.PutUint16(r[0xa:], uint16(bf.Skill))
		le.PutUint16(r[0xc:], uint16(bf.Overlay))
		r[0xe], r[0xf] = bf.Caster, bf.Flags
	}
	for k := range a.Items {
		a.Items[k].Encode(b[0x2bc+items.ItemSize*k:])
	}
	p32(0x34c, a.Group)
	p32(0x350, a.Ally)
	for k, j := range a.Jobs {
		r := b[0x354+0xc*k:]
		le.PutUint16(r, uint16(j.X))
		le.PutUint16(r[2:], uint16(j.Y))
		le.PutUint16(r[4:], uint16(j.Z))
		le.PutUint16(r[6:], j.Attrib)
		r[8], r[9], r[0xa], r[0xb] = j.Action, j.Hour, j.Day, j.Month
	}
	p32(0x3b4, a.Summoner)
	p32(0x3b8, a.LastHitBy)
	p32(0x3bc, a.UniqueName)
	copy(b[0x3c0:0x3cc], a.Unk3C0[:])
}

// Init clears an actor's runtime fields for a new monster: tether 0x100, radius 0x20,
// height 0x80, speed 200, no buffs.
//
// mm8: 0x4583e3 (Actor_Init)
func (a *Actor) Init() {
	a.MonList, a.NPC = 0, 0
	a.Pos, a.Vel = [3]int16{}, [3]int16{}
	a.Yaw, a.Pitch = 0, 0
	a.Flags = 0
	a.Sector = 0
	a.ActionTime = 0
	a.Start, a.Guard = [3]int16{}, [3]int16{}
	a.Tether, a.Radius, a.Height = defaultTether, defaultRadius, defaultHeight
	a.AIState, a.Animation = Stand, 0
	a.Speed = defaultSpeed
	a.CarriedItem = 0
	a.Group, a.Ally = 0, 0
	a.Summoner, a.LastHitBy, a.UniqueName = 0, 0, 0
	a.Sprites = [8]int16{}
	a.Buffs = [NumBuffs]Buff{}
}

// Active reports buff i.
func (a *Actor) Active(i int) bool { return a.Buffs[i].Active() }

// CanAct reports an actor that is not stoned or paralysed and is not disabled,
// summoned, removed, dead or dying.
//
// mm8: 0x4093f1 (Actor_CanAct)
func (a *Actor) CanAct() bool {
	switch a.AIState {
	case Disabled, Summoned, Removed, Dead, Dying:
		return false
	}
	return !a.Active(BuffStoned) && !a.Active(BuffParalysed)
}

// Gone reports an actor that is dying, dead, removed, summoned or stoned, and with
// disabledToo a disabled one.
//
// mm8: 0x40946e (Actor_IsGone)
func (a *Actor) Gone(disabledToo bool) bool {
	switch a.AIState {
	case Summoned, Removed, Dead, Dying:
		return true
	case Disabled:
		if disabledToo {
			return true
		}
	}
	return a.Active(BuffStoned)
}
