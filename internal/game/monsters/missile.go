package monsters

// A monster's missiles (re/notes/spells.md). Missile 1..11 and 13 are the object
// types below; 12 launches nothing.

// missileTypes maps a monsters.txt missile number to its object type.
var missileTypes = [...]int16{
	1: 545, 2: 550, 3: 510, 4: 500, 5: 515, 6: 505,
	7: 530, 8: 525, 9: 520, 10: 535, 11: 540, 13: 555,
}

// LaunchMissile fires actor i's missile along dir. The caster is the actor.
// Special 1 with count 2 adds a shot 0x28 higher; count 3 adds a pair at yaw ±0x1e.
// second is the attack: 0 the first, 1 the second (the blow's type).
//
// mm8: 0x404bc6
func (b *Brain) LaunchMissile(i int, dir Dir, missile int, second int) {
	if missile < 0 || missile >= len(missileTypes) || missileTypes[missile] == 0 || i < 0 || i >= len(b.Actors) {
		return
	}
	a := &b.Actors[i]
	o := newObject()
	o.Type = missileTypes[missile]
	if b.Tables != nil {
		o.ObjList = int16(b.Tables.ObjListOf(o.Type))
	}
	o.Pos[0], o.Pos[1] = int32(a.Pos[0]), int32(a.Pos[1])
	o.Pos[2] = int32(a.Pos[2]) - int32(float32(int16(a.Height))*-0.75)
	o.Yaw = uint16(dir.Yaw)
	o.Caster = int32(i<<3 | KindActor)
	o.CasterAbility = uint8(second)
	o.LOD = missileLOD(dir.Dist)
	if b.Indoor != nil {
		o.Sector = int16(b.Indoor.SectorAt(o.Pos[0], o.Pos[1], o.Pos[2]))
	}
	speed := int32(0)
	if b.Tables != nil && int(o.ObjList) < len(b.Tables.ObjList) {
		speed = int32(b.Tables.ObjList[o.ObjList].Speed)
	}
	objs := b.objects()
	objs, _ = Launch(objs, o, dir.Yaw, dir.Pitch, speed)
	if a.Info.Special == 1 {
		switch a.Info.SpecialC {
		case 2:
			o.Pos[2] += 0x28
			objs, _ = Launch(objs, o, dir.Yaw, dir.Pitch, speed)
		case 3:
			objs, _ = Launch(objs, o, dir.Yaw+0x1e, dir.Pitch, speed)
			objs, _ = Launch(objs, o, dir.Yaw-0x1e, dir.Pitch, speed)
		}
	}
	if b.Objects != nil {
		*b.Objects = objs
	}
}

// missileLOD is the sprite detail from the distance: 0 under 307.2, then 1, 2, 3.
func missileLOD(dist int32) uint8 {
	if float64(dist) < 307.2 {
		return 0
	}
	if dist < 0x400 {
		return 1
	}
	if dist > 0x9ff {
		return 3
	}
	return 2
}

// Explode drops object 600 at three quarters of the actor's height. The blast is
// that object's impact, not a separate damage call.
//
// mm8: 0x404a87
func (b *Brain) Explode(i int) {
	if i < 0 || i >= len(b.Actors) || b.Objects == nil {
		return
	}
	a := &b.Actors[i]
	o := newObject()
	o.Type = 600
	if b.Tables != nil {
		o.ObjList = int16(b.Tables.ObjListOf(600))
	}
	o.Pos[0], o.Pos[1] = int32(a.Pos[0]), int32(a.Pos[1])
	o.Pos[2] = int32(a.Pos[2]) - int32(float32(int16(a.Height))*-0.75)
	o.Caster = int32(i<<3 | KindActor)
	o.LOD = 3
	o.CasterAbility = 4
	if b.Indoor != nil {
		o.Sector = int16(b.Indoor.SectorAt(o.Pos[0], o.Pos[1], o.Pos[2]))
	}
	objs, _ := Launch(b.objects(), o, 0, 0, 0)
	*b.Objects = objs
}

// Splash drops object 800 where something hits the water. The sound is 0x99 (M11).
//
// mm8: 0x42ee72
func (b *Brain) Splash(x, y, z int32) {
	if b.Objects == nil {
		return
	}
	o := newObject()
	o.Type = 800
	if b.Tables != nil {
		o.ObjList = int16(b.Tables.ObjListOf(800))
	}
	o.Pos = [3]int32{x, y, z}
	if b.Indoor != nil {
		o.Sector = int16(b.Indoor.SectorAt(x, y, z))
	}
	objs, _ := Launch(b.objects(), o, 0, 0, 0)
	*b.Objects = objs
}
