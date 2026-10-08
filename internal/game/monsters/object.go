package monsters

import (
	"encoding/binary"

	"libre-enroth/internal/game/items"
)

// Object attributes (Object.Attrib).
const (
	ObjDrawn       = 0x1   // drawn this frame
	ObjTemporary   = 0x8   // removed when the map loads (Objects_RemoveTemporary)
	ObjNotPickable = 0x20  // the software renderer leaves it out of the pick buffer
	ObjRemoved     = 0x200 // Object_Remove
)

// Object is a 0x70-byte SpriteObject: an item on the ground, a projectile or an effect.
type Object struct {
	Type          int16    // +0x00: the dobjlist id
	ObjList       int16    // +0x02: dobjlist.bin index, 0 = a free slot
	Pos           [3]int32 // +0x04
	Vel           [3]int16 // +0x10
	Yaw           uint16   // +0x16
	Sound         uint16   // +0x18
	Attrib        uint16   // +0x1a: Obj*
	Sector        int16    // +0x1c
	Time          uint16   // +0x1e: the sft frame time
	Lifetime      int16    // +0x20
	Glow          int16    // +0x22: multiplies the frame's glow
	Item          items.Item
	Spell         int32    // +0x48
	SpellLevel    int32    // +0x4c
	SpellSkill    int32    // +0x50
	Unk54         int32    // +0x54
	Caster        int32    // +0x58: pid
	Target        int32    // +0x5c: pid
	LOD           uint8    // +0x60
	CasterAbility uint8    // +0x61
	Unk62         [2]byte  // +0x62
	Start         [3]int32 // +0x64
}

// DecodeObject reads a 0x70-byte record.
func DecodeObject(b []byte) Object {
	le := binary.LittleEndian
	i16 := func(o int) int16 { return int16(le.Uint16(b[o:])) }
	i32 := func(o int) int32 { return int32(le.Uint32(b[o:])) }
	o := Object{
		Type: i16(0), ObjList: i16(2),
		Yaw: le.Uint16(b[0x16:]), Sound: le.Uint16(b[0x18:]), Attrib: le.Uint16(b[0x1a:]), Sector: i16(0x1c),
		Time: le.Uint16(b[0x1e:]), Lifetime: i16(0x20), Glow: i16(0x22),
		Item:  items.DecodeItem(b[0x24:]),
		Spell: i32(0x48), SpellLevel: i32(0x4c), SpellSkill: i32(0x50), Unk54: i32(0x54), Caster: i32(0x58), Target: i32(0x5c),
		LOD: b[0x60], CasterAbility: b[0x61], Unk62: [2]byte{b[0x62], b[0x63]},
	}
	for k := range 3 {
		o.Pos[k] = i32(4 + 4*k)
		o.Vel[k] = i16(0x10 + 2*k)
		o.Start[k] = i32(0x64 + 4*k)
	}
	return o
}

// Encode writes the 0x70-byte record.
func (o *Object) Encode(b []byte) {
	le := binary.LittleEndian
	p16 := func(at int, v int16) { le.PutUint16(b[at:], uint16(v)) }
	p32 := func(at int, v int32) { le.PutUint32(b[at:], uint32(v)) }
	p16(0, o.Type)
	p16(2, o.ObjList)
	for k := range 3 {
		p32(4+4*k, o.Pos[k])
		p16(0x10+2*k, o.Vel[k])
		p32(0x64+4*k, o.Start[k])
	}
	le.PutUint16(b[0x16:], o.Yaw)
	le.PutUint16(b[0x18:], o.Sound)
	le.PutUint16(b[0x1a:], o.Attrib)
	p16(0x1c, o.Sector)
	le.PutUint16(b[0x1e:], o.Time)
	p16(0x20, o.Lifetime)
	p16(0x22, o.Glow)
	o.Item.Encode(b[0x24:])
	p32(0x48, o.Spell)
	p32(0x4c, o.SpellLevel)
	p32(0x50, o.SpellSkill)
	p32(0x54, o.Unk54)
	p32(0x58, o.Caster)
	p32(0x5c, o.Target)
	b[0x60], b[0x61], b[0x62], b[0x63] = o.LOD, o.CasterAbility, o.Unk62[0], o.Unk62[1]
}
