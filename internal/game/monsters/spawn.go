package monsters

import (
	"fmt"

	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/physics"
)

// SpawnPoint is a map's 0x18-byte spawn point (odm.Spawn, blv.Spawn).
type SpawnPoint struct {
	X, Y, Z int32
	Radius  uint16 // +0x0c
	Kind    uint16 // +0x0e: SpawnMonsters, anything else items
	Index   uint16 // +0x10: monsters 1..12, items the level 1..7
	Attrib  uint16 // +0x12: SpawnAlertOnly
	Group   uint32 // +0x14
}

// Spawn point kinds and attributes.
const (
	SpawnMonsters  = 3
	SpawnAlertOnly = 0x1
)

// Geo is the indoor geometry spawning tests positions against.
type Geo interface {
	SectorAt(x, y, z int32) int
	FloorZ(x, y, z int32, sector int) (int32, int)
}

// Env is what spawning reads: the tables, the sprite frames, the indoor geometry (nil
// outdoors), the random generator and the map's alert value (Map_IsAlert).
type Env struct {
	Tables *tables.Monsters
	Items  *tables.Items
	SFT    *desc.SFT
	Indoor Geo
	Rand   items.Rand
	Alert  uint32
	// Found marks the artifacts found so far (Party +0x7d4).
	Found *[items.NumArtifacts]bool
}

// Variant chances (A%, B%; C takes the rest) by difficulty 0..4.
//
// mm8: 0x4f9c66 (g_spawnVariantChances)
var variantChances = [5][3]int{{0, 0, 0}, {90, 8, 2}, {70, 20, 10}, {50, 30, 20}, {30, 40, 30}}

// LoadSprites fills an actor's animations, size and speed from its monster's dmonlist
// record (by the monsters.txt id, not MonList): the 8 sprite groups, height and radius,
// monsters.txt's speed, and the sounds unless noSounds.
//
// mm8: 0x45830d (Actor_LoadSprites)
func (a *Actor) LoadSprites(t *tables.Monsters, sft *desc.SFT, noSounds bool) {
	i := int(a.Info.ID) - 1
	if i < 0 || i >= len(t.MonList) {
		return
	}
	ml := &t.MonList[i]
	for k := range a.Sprites {
		a.Sprites[k] = int16(sft.FindGroup(ml.Sprites[k]))
	}
	a.Height, a.Radius = ml.Height, ml.Radius
	if in := t.Info(int(a.Info.ID)); in != nil {
		a.Speed = uint16(in.Speed)
	}
	if !noSounds {
		a.Sounds = ml.Sounds
	}
}

// Spawn puts the monsters of spawn point sp into actors: the MapStats slot it names, a
// random number of them (count overrides it), each a random A/B/C variant by the slot's
// difficulty (+ difBonus, at most 4), one at the point and each next one at a random
// point radius away from it (0x40 indoors, 0x80 outdoors). Indoors a monster is kept only
// when that next point is in the point's sector and on a floor within 0x400 of it; a
// rejected point still places the next monster. hostile sets FlagHostile. A point marked
// SpawnAlertOnly spawns only on an alert map, the others only on a calm one.
//
// mm8: 0x44dfbe (Spawn_Monsters)
func Spawn(e *Env, actors []Actor, row *tables.MapStats, sp *SpawnPoint, difBonus, count int, hostile bool) ([]Actor, error) {
	if (e.Alert == 0) == (sp.Attrib&SpawnAlertOnly != 0) {
		return actors, nil
	}
	if row == nil {
		return actors, nil
	}
	n, dif := 1, 0
	var name string
	switch idx := int(sp.Index); {
	case idx >= 1 && idx <= 3:
		k := idx - 1
		r := e.Rand.Int()
		dif = int(row.Dif[k])
		name = row.Monster[k]
		if span := int(row.Max[k]) - int(row.Min[k]) + 1; span != 0 {
			n = r%span + int(row.Min[k])
		} else {
			n = int(row.Min[k]) // the original divides by zero
		}
	case idx >= 4 && idx <= 12:
		name = fmt.Sprintf("%s %c", row.Monster[(idx-4)%3], 'A'+(idx-4)/3)
	default:
		return actors, nil
	}
	if name == "" || name[0] == '0' {
		return actors, nil
	}
	dif = min(dif+difBonus, 4)
	if count != 0 {
		n = count
	}
	if len(actors)+n >= MaxActors {
		return actors, nil
	}
	x, y, z := sp.X, sp.Y, sp.Z
	sector, radius := 0, int64(0x80)
	if e.Indoor != nil {
		sector, radius = e.Indoor.SectorAt(x, y, z), 0x40
	}
	for range n {
		var a Actor
		a.Init()
		pick := name
		if dif != 0 {
			r := e.Rand.Int() % 100
			c := variantChances[dif]
			v := 'C'
			switch {
			case r < c[0]:
				v = 'A'
			case r < c[0]+c[1]:
				v = 'B'
			}
			pick = fmt.Sprintf("%s %c", name, v)
		}
		ml := e.Tables.FindMonList(pick)
		if ml < 0 {
			return actors, fmt.Errorf("can't create random monster: '%s'! See MonList.txt", pick)
		}
		id := e.Tables.FindByPicture(pick)
		if id == 0 {
			id = 1
		}
		if id < 0 {
			return actors, fmt.Errorf("can't create random monster: '%s'! See Monster.txt", pick)
		}
		in := e.Tables.Info(id)
		a.HP = int16(in.HP)
		a.Info = *in
		m := &e.Tables.MonList[ml]
		a.MonList = int16(ml + 1)
		a.Radius, a.Height, a.Speed = m.Radius, m.Height, m.Speed
		a.Pos = [3]int16{int16(x), int16(y), int16(z)}
		a.Start = a.Pos
		a.Tether = defaultTether
		a.Sector = int16(sector)
		a.Group = int32(sp.Group)
		a.LoadSprites(e.Tables, e.SFT, false)
		a.Info.Hostility = 0
		ang := int32(e.Rand.Int() % 0x800)
		x = sp.X + int32(radius*int64(physics.Cos(ang))>>16)
		y = sp.Y + int32(radius*int64(physics.Cos(ang-physics.QuarterTurn))>>16)
		z = sp.Z
		keep := true
		if e.Indoor != nil {
			keep = false
			if s := e.Indoor.SectorAt(x, y, z); s == sector {
				if fz, _ := e.Indoor.FloorZ(x, y, z, s); fz != physics.NoFloor && abs32(fz-z) < 0x401 {
					z, keep = fz, true
				}
			}
		}
		if keep {
			if hostile {
				a.Flags |= FlagHostile
			}
			actors = append(actors, a)
		}
	}
	return actors, nil
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// CreateObject puts o into the first free slot (ObjList 0) of objects, growing the list
// when the slot is past its end; its start position is where it is. An object without a
// dobjlist entry is not made (-1). Throwing (a speed or a member) is M8c's.
//
// mm8: 0x42eaea (Object_Create, speed and member 0)
func CreateObject(objects []Object, o Object) ([]Object, int) {
	if o.ObjList == 0 {
		return objects, -1
	}
	slot := len(objects)
	for i := range objects {
		if objects[i].ObjList == 0 {
			slot = i
			break
		}
	}
	if slot >= MaxObjects {
		return objects, -1
	}
	o.Start = o.Pos
	o.Vel = [3]int16{}
	if slot == len(objects) {
		objects = append(objects, o)
	} else {
		objects[slot] = o
	}
	return objects, slot
}

// newObject is SpriteObject_Init: no item, glow 1.
//
// mm8: 0x404b7a (SpriteObject_Init)
func newObject() Object { return Object{Glow: 1} }

// itemObject makes the ground object of an item at a point: its kind is the dobjlist
// entry of the item's sprite.
func (e *Env) itemObject(it items.Item, x, y, z int32) Object {
	o := newObject()
	o.Item = it
	if d := e.Items.Item(it.Number); d != nil {
		o.Type = d.Sprite
	}
	o.ObjList = int16(e.Tables.ObjListOf(o.Type))
	o.Pos = [3]int32{x, y, z}
	if e.Indoor != nil {
		o.Sector = int16(e.Indoor.SectorAt(x, y, z))
	}
	return o
}

// SpawnItems makes the item of an item spawn point: a level from g_treasureLevels by the
// point's index and the map's treasure level; level 7 and up an artifact (none if all are
// found); else 20% nothing, 40% gold by the point's index and 40% an item of kind 20..46.
//
// mm8: 0x44ea5f (Spawn_Items), 0x44efc6 (Spawn_RandomItem)
func SpawnItems(e *Env, objects []Object, row *tables.MapStats, sp *SpawnPoint) []Object {
	r := e.Rand.Int() % 100
	ti := int(sp.Index)*7 + int(row.Treasure)
	var mn, mx int
	if ti >= 0 && ti < len(e.Tables.Treasure) {
		mn, mx = int(e.Tables.Treasure[ti][0]), int(e.Tables.Treasure[ti][1])
	}
	span := mx - mn + 1
	rl := e.Rand.Int()
	level := mn
	if span != 0 {
		level = rl%span + mn
	}
	var it items.Item
	switch {
	case level > 6:
		if !items.GenerateArtifact(&it, e.Items, e.Rand, e.found()) {
			return objects
		}
	case r < 20:
		return objects
	case r > 59:
		kind := e.Rand.Int()%0x1b + 0x14
		it = items.Generate(e.Items, level, kind, false, e.Rand, e.found())
	default:
		items.GoldPile(&it, int(sp.Index), e.Rand)
		it.Flags |= items.FlagIdentified
	}
	objects, _ = CreateObject(objects, e.itemObject(it, sp.X, sp.Y, sp.Z))
	return objects
}

func (e *Env) found() *[items.NumArtifacts]bool {
	if e.Found == nil {
		e.Found = new([items.NumArtifacts]bool)
	}
	return e.Found
}
