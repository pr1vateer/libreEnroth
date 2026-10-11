package monsters

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/blv"
)

// Ground objects (re/notes/monsters.md, "Objects"): items dropped, thrown or spawned
// are SpriteObjects that fall, bounce and slide to rest, moved each frame before the
// party (Objects_Update). Projectiles and their impacts are M9's.

// More object attributes (Object.Attrib).
const (
	ObjOwnLifetime = 0x2  // the object's Lifetime replaces its kind's
	objTurnBased   = 0x4  // a turn-based mode projectile (M9)
	ObjSkipFrame   = 0x40 // not moved this frame
	objFollows     = 0x80 // stays on its caster actor (spell effects, M9)
)

// Object mover constants.
const (
	objectMaxX     = 0x7fff // indoors an object further out is removed
	objectMaxZ     = 20000
	outdoorEdge    = 0x8000
	outdoorMaxZ    = 13000
	steepWall      = 0x7d01 // a face normal z below this bounces an object twice as hard
	splashOnModel  = 0x1e
	itemBounceStop = 10
)

// CreateObject is Launch at rest.
//
// mm8: 0x42eaea (Object_Create with speed 0 and no member)
func CreateObject(objects []Object, o Object) ([]Object, int) {
	return Launch(objects, o, 0, 0, 0)
}

// Launch puts o into the first free slot (ObjList 0) of objects, growing the list
// when the slot is past its end, starting where it is and moving at speed along yaw
// and pitch. An object without a dobjlist entry is not made (-1), nor one past the
// 1000th slot. (A member's spell starting beside the party is M9's.)
//
// mm8: 0x42eaea (Object_Create)
func Launch(objects []Object, o Object, yaw, pitch, speed int32) ([]Object, int) {
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
	if speed != 0 {
		cp := physics.Cos(pitch)
		o.Vel[0] = int16(speed * physics.Mul16(cp, physics.Cos(yaw)) >> 16)
		o.Vel[1] = int16(speed * physics.Mul16(cp, physics.Cos(yaw-physics.QuarterTurn)) >> 16)
		o.Vel[2] = int16(uint32(speed*physics.Cos(pitch-physics.QuarterTurn)) >> 16)
	}
	if slot == len(objects) {
		objects = append(objects, o)
	} else {
		objects[slot] = o
	}
	return objects, slot
}

// DropItem makes count objects of kind sprite (an item's sprite) carrying it at a point,
// thrown at speed: straight up, or with randomYaw each in a random direction 45..90°
// up. Without an item (and attrib 0x10 clear) it carries the first items.txt item of
// that sprite.
//
// mm8: 0x42ecee (Object_DropItem)
func (e *Env) DropItem(objects []Object, it *items.Item, sprite int16, x, y, z, speed int32, count int,
	randomYaw bool, attrib uint16) []Object {
	o := newObject()
	if it != nil {
		o.Item = *it
	}
	o.Type = sprite
	o.ObjList = int16(e.Tables.ObjListOf(sprite))
	o.Pos = [3]int32{x, y, z}
	o.Attrib = attrib
	if e.Indoor != nil {
		o.Sector = int16(e.Indoor.SectorAt(x, y, z))
	}
	if attrib&0x10 == 0 && it == nil && e.Items != nil {
		for n := range e.Items.Items {
			if e.Items.Items[n].Sprite == sprite {
				o.Item.Number = int32(n)
				break
			}
		}
	}
	for range count {
		yaw, pitch := int32(0), int32(physics.QuarterTurn)
		if randomYaw {
			o.Yaw = uint16(e.Rand.Int() % physics.FullTurn)
			yaw = int32(int16(o.Yaw))
			pitch = int32(e.Rand.Int()%physics.QuarterTurn>>1) + physics.QuarterTurn>>1
		}
		objects, _ = Launch(objects, o, yaw, pitch, speed)
	}
	return objects
}

// env is what the brain's drops need of Env.
func (b *Brain) env() *Env {
	e := &Env{Tables: b.Tables, Rand: b.Rand}
	if b.Party.Ctx != nil {
		e.Items = b.Party.Ctx.Items
	}
	if b.Indoor != nil {
		e.Indoor = b.Indoor
	}
	return e
}

// lifetime is how long object o lasts: its kind's lifetime or, with ObjOwnLifetime,
// its own.
func lifetime(o *Object, k *tables.ObjListEntry) int16 {
	if o.Attrib&ObjOwnLifetime != 0 {
		return o.Lifetime
	}
	return k.Lifetime
}

// MoveObjects runs the objects for a frame of ticks: an object marked to skip a frame
// does; the others age, those with a lifetime go when it is over (a projectile's
// impact then is M9's), and the rest move. Spell effects riding on an actor are M9's.
//
// mm8: 0x472b60 (Objects_Update)
func (b *Brain) MoveObjects(ticks int32) {
	objs := b.objects()
	dt, dtFixed := timeStep(ticks)
	for i := range objs {
		o := &objs[i]
		if o.Attrib&ObjSkipFrame != 0 {
			o.Attrib &^= ObjSkipFrame
			continue
		}
		if o.Attrib&objFollows != 0 {
			b.stub("follow", "Objects_Update 0x472b60: spell effects riding on their caster (M9)")
			continue
		}
		if o.ObjList == 0 || int(o.ObjList) >= len(b.Tables.ObjList) {
			continue
		}
		k := &b.Tables.ObjList[o.ObjList]
		o.Time += uint16(dt)
		if k.Flags&tables.ObjLifetime != 0 && (int16(o.Time) < 0 || int16(o.Time) >= lifetime(o, k)) {
			if int16(o.Time) >= 0 && k.Flags&tables.ObjFlagOnIntercept != 0 {
				b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
				continue
			}
			removeObject(o)
			continue
		}
		if b.Indoor != nil {
			b.moveObjectIndoor(o, k, dt, dtFixed)
		} else if b.Outdoor != nil {
			b.moveObjectOutdoor(o, k, dt, dtFixed)
		}
		// In turn-based mode a projectile further than 0x1400 from the party goes (M9).
	}
}

// removeObject frees an object's slot (the record stays, marked removed; Launch reuses
// the slot).
//
// mm8: 0x42ee3b (Object_Remove; its turn-based bookkeeping is M9's)
func removeObject(o *Object) {
	o.ObjList = 0
	o.Attrib |= ObjRemoved
}

// RemoveObject frees object slot i.
func RemoveObject(objects []Object, i int) { removeObject(&objects[i]) }

// objectSweep is the collision state of an object of kind k: one sphere at its feet.
func objectSweep(k *tables.ObjListEntry) physics.State {
	return physics.State{RadiusLo: int32(k.Radius), Height: int32(k.Height), IgnorePID: -1}
}

func (b *Brain) startObjectSweep(st *physics.State, o *Object, dtFixed int32) bool {
	st.PosLo = [3]int32{o.Pos[0], o.Pos[1], o.Pos[2] + st.RadiusLo + 1}
	st.PosHi = st.PosLo
	st.Velocity = [3]int32{int32(o.Vel[0]), int32(o.Vel[1]), int32(o.Vel[2])}
	st.Sector = int32(o.Sector)
	return st.Begin(dtFixed)
}

// dampObject is an object's velocity after touching something: x and y in 32 bits, z
// in 64.
func dampObject(o *Object) {
	o.Vel[0] = int16(int32(o.Vel[0]) * friction >> 16)
	o.Vel[1] = int16(int32(o.Vel[1]) * friction >> 16)
	o.Vel[2] = int16(int64(o.Vel[2]) * friction >> 16)
}

func slow(o *Object) bool {
	return int32(o.Vel[1])*int32(o.Vel[1])+int32(o.Vel[0])*int32(o.Vel[0]) < minSpeedSq
}

// collideObjectActors sweeps an object against the actors: a party's or nobody's
// object against all of them (radius: indoors their dmonlist to-hit radius, outdoors
// their own); an actor's misses its own kind (M9).
func (b *Brain) collideObjectActors(st *physics.State, o *Object, indoor bool) {
	if o.Caster&7 == KindActor {
		b.stub("objcaster", "Objects_Move: an actor's projectile against the other actors (M9)")
		return
	}
	for j := range b.Actors {
		r := int32(0)
		if indoor {
			if ml := int(b.Actors[j].MonList) - 1; ml >= 0 && ml < len(b.Tables.MonList) {
				r = int32(b.Tables.MonList[ml].ToHitRadius)
			}
		}
		b.collideActor(st, j, r)
	}
}

// deflectObject turns an object round a decoration it met, keeping its speed.
func (b *Brain) deflectObject(o *Object, n int) {
	x, y, _ := b.decorationPos(n)
	speed := int32(physics.Isqrt(uint32(int32(o.Vel[1])*int32(o.Vel[1]) + int32(o.Vel[0])*int32(o.Vel[0]))))
	ang := physics.Atan2(o.Pos[0]-x, o.Pos[1]-y)
	o.Vel[0] = int16(physics.Mul16(speed, physics.Cos(ang)))
	o.Vel[1] = int16(speed * physics.Cos(ang-physics.QuarterTurn) >> 16)
}

// bounceObject pushes an object off a wall or ceiling along its normal (at least an
// eighth of the speed), twice as hard unless the face is nearly flat.
func bounceObject(o *Object, st *physics.State, n [3]int32) {
	k := abs32(int32(o.Vel[2])*n[2]+int32(o.Vel[0])*n[0]+int32(o.Vel[1])*n[1]) >> 16
	k = max(k, st.Speed>>3)
	nz := physics.Mul16(n[2], k)
	o.Vel[0] += int16(n[0]*k>>16) * 2
	o.Vel[1] += int16(n[1]*k>>16) * 2
	add := int16(nz) * 2
	if n[2] >= steepWall {
		o.Vel[2] += int16(nz)
		add = int16(uint32(nz*32000) >> 16)
	}
	o.Vel[2] += add
}

// moveObjectIndoor moves an item indoors: outside the map it goes; on the floor it
// stops falling (a sloped floor keeps pulling), slows down and rests when slow; in the
// air it falls. Then it is swept through the sector's faces, the decorations, the party
// and the actors, bouncing off what it meets.
//
// mm8: 0x4716af (Objects_MoveIndoor, the item path)
func (b *Brain) moveObjectIndoor(o *Object, k *tables.ObjListEntry, dt, dtFixed int32) {
	g := b.Indoor
	if abs32(o.Pos[0]) > objectMaxX || abs32(o.Pos[1]) > objectMaxX || abs32(o.Pos[2]) > objectMaxZ {
		removeObject(o)
		return
	}
	floorZ, face := g.FloorZ(o.Pos[0], o.Pos[1], o.Pos[2], int(o.Sector))
	if floorZ < -29999 {
		s := g.SectorAt(o.Pos[0], o.Pos[1], o.Pos[2])
		o.Sector = int16(s)
		if s == 0 {
			removeObject(o)
			return
		}
		if floorZ, face = g.FloorZ(o.Pos[0], o.Pos[1], o.Pos[2], s); floorZ == physics.NoFloor {
			removeObject(o)
			return
		}
	}
	grav := int16(physics.Gravity(false)) * int16(dt)
	if k.Flags&tables.ObjNoGravity == 0 {
		if o.Pos[2]-3 < floorZ {
			if k.Flags&tables.ObjFlagOnIntercept != 0 {
				b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
				return
			}
			o.Pos[2] = floorZ + 1
			f := &g.Map.Faces[face]
			if f.PolyType == blv.PolyFloor {
				o.Vel[2] = 0
			} else if f.Normal[2] < steepFloor {
				o.Vel[2] -= grav
			}
			dampObject(o)
			if slow(o) {
				o.Vel = [3]int16{}
				return
			}
		} else {
			o.Vel[2] -= grav
		}
	}
	st := objectSweep(k)
	p := &b.Party
	for hits := 0; ; {
		if !b.startObjectSweep(&st, o, dtFixed) {
			return
		}
		for range maxMoveIters {
			g.CollideFaces(&st, false, true)
			g.CollideDecorations(&st)
			if o.Caster&7 != KindParty {
				st.SweepParty(p.X, p.Y, p.Z, p.Radius, p.Height, true)
			}
			b.collideObjectActors(&st, o, true)
			if !g.CollidePortals(&st) {
				break
			}
		}
		if st.MoveDist <= st.AdjustedDist {
			break
		}
		adj := st.AdjustedDist
		o.Pos[0] += physics.Mul16(st.Dir[0], adj)
		o.Pos[1] += physics.Mul16(st.Dir[1], adj)
		o.Pos[2] += physics.Mul16(st.Dir[2], adj)
		o.Sector = int16(st.Sector)
		st.TotalMoved += adj
		if k.Flags&tables.ObjFlagOnIntercept != 0 {
			b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
			return
		}
		pid := uint32(st.PID)
		switch pid & 7 {
		case KindDecoration:
			b.deflectObject(o, int(pid>>3))
		case KindFace:
			fi := int(pid >> 3)
			st.IgnorePID = int32(fi)
			f := &g.Map.Faces[fi]
			switch {
			case f.PolyType != blv.PolyFloor:
				bounceObject(o, &st, f.Normal)
			case k.Flags&tables.ObjBounce != 0:
				if o.Vel[2] = int16(-int32(o.Vel[2]) >> 1); o.Vel[2] < itemBounceStop {
					o.Vel[2] = 0
				}
			default:
				o.Vel[2] = 0
				if slow(o) {
					o.Vel = [3]int16{}
					o.Pos[2] = int32(g.Map.Vertices[f.Verts[0]].Z) + 1
				}
			}
		}
		dampObject(o)
		if hits++; hits > 99 {
			return
		}
	}
	o.Pos = [3]int32{st.NewPosLo[0], st.NewPosLo[1], st.NewPosLo[2] - st.RadiusLo - 1}
	o.Sector = int16(st.Sector)
}

// moveObjectOutdoor moves an item outdoors: one that comes down on water sinks with a
// splash; on the ground it stops falling (a bouncing kind bounces), slows down and
// rests when slow, on steep ground it slides; in the air it falls. Then it is swept
// through the models, the decorations of its cell, the party and the actors.
//
// mm8: 0x472047 (Objects_MoveOutdoor, the item path)
func (b *Brain) moveObjectOutdoor(o *Object, k *tables.ObjListEntry, dt, dtFixed int32) {
	g := b.Outdoor
	steep := g.TooSteep(o.Pos[0], o.Pos[1])
	floorZ, water, _, _ := g.FloorZ(o.Pos[0], o.Pos[1], o.Pos[2], false, false)
	z1 := floorZ + 1
	// The hit counter starts at 1 in the air and at the x speed after sliding on the
	// ground (the original reuses the variable).
	hits := int32(0)
	switch {
	case z1 < o.Pos[2]:
		hits = 1
	case water:
		b.splash()
		removeObject(o)
		return
	}
	grav := int16(physics.Gravity(g.PlaneOfWater)) * int16(dt)
	if k.Flags&tables.ObjNoGravity == 0 {
		switch {
		case hits == 1:
			o.Vel[2] -= grav
		case steep:
			o.Pos[2] = z1
			n := g.TerrainNormal(o.Pos[0], o.Pos[1])
			o.Vel[2] -= grav
			d := abs32(int32(o.Vel[0])*n[0]+int32(o.Vel[1])*n[1]+int32(o.Vel[2])*n[2]) >> 16
			o.Vel[0] += int16(uint32(n[0]*d) >> 16)
			o.Vel[1] += int16(uint32(n[1]*d) >> 16)
			o.Vel[2] += int16(physics.Mul16(n[2], d))
		default:
			if k.Flags&tables.ObjFlagOnIntercept != 0 {
				b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
				return
			}
			o.Pos[2] = z1
			if k.Flags&tables.ObjBounce == 0 {
				o.Vel[2] = 0
			} else if o.Vel[2] = int16(-int32(o.Vel[2]) >> 1); o.Vel[2] < itemBounceStop {
				o.Vel[2] = 0
			}
			dampObject(o)
			hits = int32(o.Vel[0])
			if slow(o) {
				o.Vel[0], o.Vel[1] = 0, 0
				return
			}
		}
	}
	if k.Flags&tables.ObjFlagOnIntercept != 0 && (o.Pos[2] <= floorZ || o.Pos[0] < -outdoorEdge || o.Pos[0] > outdoorEdge ||
		o.Pos[1] < -outdoorEdge || o.Pos[1] > outdoorEdge || o.Pos[2] > outdoorMaxZ) {
		b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
		return
	}
	st := objectSweep(k)
	p := &b.Party
	for {
		st.Sector = 0
		o.Sector = 0
		if !b.startObjectSweep(&st, o, dtFixed) {
			return
		}
		g.CollideModels(&st, false, true)
		g.CollideCellDecorations(&st, physics.GridX(o.Pos[0]), physics.GridY(o.Pos[1]))
		if o.Caster&7 != KindParty {
			st.SweepParty(p.X, p.Y, p.Z, p.Radius, p.Height, false)
		}
		b.collideObjectActors(&st, o, false)
		nz := st.NewPosLo[2] - st.RadiusLo - 1
		if fz, wet, _, _ := g.FloorZ(st.NewPosLo[0], st.NewPosLo[1], nz, false, false); wet && nz < fz+splashHeight {
			b.splash()
			removeObject(o)
			return
		}
		if st.MoveDist <= st.AdjustedDist {
			break
		}
		adj := st.AdjustedDist
		o.Pos[0] += physics.Mul16(st.Dir[0], adj)
		o.Pos[1] += physics.Mul16(st.Dir[1], adj)
		o.Pos[2] += physics.Mul16(st.Dir[2], adj)
		o.Sector = int16(st.Sector)
		st.TotalMoved += adj
		if k.Flags&tables.ObjFlagOnIntercept != 0 {
			b.stub("impact", "Object_Impact 0x46bd3a: a projectile's end (M9)")
			return
		}
		pid := uint32(st.PID)
		switch pid & 7 {
		case KindDecoration:
			b.deflectObject(o, int(pid>>3))
		case KindFace:
			mi, fi := int(pid>>9), int(pid>>3)&0x3f
			f := &g.Map.Models[mi].Faces[fi]
			if f.PolyType == blv.PolyFloor {
				o.Pos[2] = g.Map.Models[mi].Vertices[f.Verts[0]].Z + 1
				if slow(o) {
					o.Vel = [3]int16{}
				}
			} else {
				bounceObject(o, &st, f.Normal)
			}
		}
		dampObject(o)
		if hits++; hits > 99 {
			return
		}
	}
	o.Pos = [3]int32{st.NewPosLo[0], st.NewPosLo[1], st.NewPosLo[2] - st.RadiusLo - 1}
	o.Sector = int16(st.Sector)
}

// splash is the splash of something sinking (Splash_Spawn 0x42ee72, an effect object:
// M9).
func (b *Brain) splash() {
	b.stub("splash", "Splash_Spawn 0x42ee72: the splash of something sinking (M9)")
}
