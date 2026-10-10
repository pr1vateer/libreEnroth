package monsters

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/blv"
)

// The movers (re/notes/monsters.md, "Movement"): every frame each actor that moves gets
// its velocity (along its yaw while walking, else slowing down), gravity, and is swept
// through the map like the party (physics), bumping into the other actors of the AI
// list and the party. A bump changes its mind: it flees what it met, or faces a
// friend.

// Mover constants.
const (
	maxWalkSpeed  = 1000
	slowDown      = 55000  // 16.16 velocity scale when not walking
	friction      = 0xe484 // after each hit
	minSpeedSq    = 400    // Physics_MinSpeedSq
	maxMoveIters  = 100
	actorBump     = 0x28  // the other actors' radius for the movers' sweeps
	steepFloor    = 45000 // floor normals below this slide the actor down
	ledgeDrop     = 100   // a walker turns away from a drop deeper than this
	attrNoWalk    = 0x400000
	outdoorRadius = 0x28 // a walking actor's sweep radius outdoors
	splashHeight  = 0x3c
)

// timeStep is the movers' g_timer step: 1..32 ticks and the same in 16.16 seconds.
func timeStep(ticks int32) (dt, dtFixed int32) {
	dt = max(1, min(ticks, 0x20))
	return dt, (dt << 16) / 128
}

// walkVelocity sets a walking actor's velocity along its yaw (and pitch when it flies)
// at its speed: halved or divided by the slow power, doubled when pursuing or fleeing,
// at most 1000. Others slow down.
func walkVelocity(a *Actor, fly bool) {
	if a.Animation != tables.AnimWalk {
		a.Vel[0] = int16(int32(a.Vel[0]) * slowDown >> 16)
		a.Vel[1] = int16(int32(a.Vel[1]) * slowDown >> 16)
		if fly {
			a.Vel[2] = int16(int32(a.Vel[2]) * slowDown >> 16)
		}
		return
	}
	speed := int32(int16(a.Speed))
	if a.Active(BuffSlowed) {
		if p := uint16(a.Buffs[BuffSlowed].Power); p == 0 {
			speed = ftol(float64(speed) * 0.5)
		} else {
			speed /= int32(p)
		}
	}
	if a.AIState == Pursue || a.AIState == Flee {
		speed *= 2
	}
	speed = min(speed, maxWalkSpeed)
	yaw := int32(int16(a.Yaw))
	a.Vel[0] = int16(physics.Mul16(speed, physics.Cos(yaw)))
	a.Vel[1] = int16(physics.Mul16(speed, physics.Cos(yaw-physics.QuarterTurn)))
	if fly {
		a.Vel[2] = int16(physics.Mul16(speed, physics.Cos(int32(int16(a.Pitch))-physics.QuarterTurn)))
	}
}

// sweep prepares the collision state for an actor at its position.
func sweepState(a *Actor, radius int32) physics.State {
	return physics.State{CheckHi: true, RadiusLo: radius, RadiusHi: radius, Height: int32(int16(a.Height)), IgnorePID: -1}
}

func (b *Brain) startSweep(st *physics.State, a *Actor, sector int32, dtFixed int32) bool {
	x, y, z := int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2])
	st.PosLo = [3]int32{x, y, z + 1 + st.RadiusLo}
	st.PosHi = [3]int32{x, y, max(z-st.RadiusLo-1+st.Height, st.PosLo[2])}
	st.Velocity = [3]int32{int32(a.Vel[0]), int32(a.Vel[1]), int32(a.Vel[2])}
	st.Sector = sector
	return st.Begin(dtFixed)
}

// collideOthers sweeps against the AI list's other actors (as 0x28 wide) and counts
// the touches; indoors only those not already overlapping.
func (b *Brain) collideOthers(st *physics.State, i int, indoor bool) int {
	a := &b.Actors[i]
	n := 0
	for _, j := range b.list {
		if j == i {
			continue
		}
		t := &b.Actors[j]
		if indoor {
			d := approxDist(abs32(int32(t.Pos[0])-int32(a.Pos[0])), abs32(int32(t.Pos[1])-int32(a.Pos[1])),
				abs32(int32(t.Pos[2])-int32(a.Pos[2])))
			if int32(int16(t.Radius))+int32(int16(a.Radius)) > d {
				continue
			}
		}
		if b.collideActor(st, j, actorBump) {
			n++
		}
	}
	return n
}

// collideActor is Collide_Actor for actor j (radius 0: its own).
//
// mm8: 0x46e143 (Collide_Actor)
func (b *Brain) collideActor(st *physics.State, j int, radius int32) bool {
	t := &b.Actors[j]
	switch t.AIState {
	case Removed, Dying, Disabled, Dead, Summoned:
		return false
	}
	if radius == 0 {
		radius = int32(int16(t.Radius))
	}
	return st.SweepActor(int32(t.Pos[0]), int32(t.Pos[1]), int32(t.Pos[2]), radius, int32(int16(t.Height)), int32(PID(j)))
}

// CollideActors sweeps the party's path against the actors it cannot walk through: all
// but the peasants that are not hostile.
//
// mm8: 0x472e86, 0x473fee (Party_MoveIndoor/Outdoor: the actor loop)
func CollideActors(st *physics.State, actors []Actor) {
	for j := range actors {
		t := &actors[j]
		if t.IsPeasant() && t.Flags&FlagHostile == 0 {
			continue
		}
		switch t.AIState {
		case Removed, Dying, Disabled, Dead, Summoned:
			continue
		}
		st.SweepActor(int32(t.Pos[0]), int32(t.Pos[1]), int32(t.Pos[2]), int32(int16(t.Radius)),
			int32(int16(t.Height)), int32(PID(j)))
	}
}

// advance moves an actor the hit's distance along the sweep: x and y in the original's
// 32-bit products, z in 64 bits.
func advance(a *Actor, st *physics.State) {
	adj := st.AdjustedDist
	a.Pos[0] += int16(uint32(st.Dir[0]*adj) >> 16)
	a.Pos[1] += int16(uint32(st.Dir[1]*adj) >> 16)
	a.Pos[2] += int16(int64(st.Dir[2]) * int64(adj) >> 16)
}

// bump is what an actor does on meeting pid: another actor (crowded: two or more
// touched) or the party, or a decoration it slides around. It reports false for the
// other kinds.
func (b *Brain) bump(i int, pid uint32, crowded bool) bool {
	a := &b.Actors[i]
	switch pid & 7 {
	case KindActor:
		t := &b.Actors[pid>>3]
		switch {
		case crowded:
			b.StandOrBored(i, partyPID, 0, facing(a))
		case a.Info.Hostility == 0 && t.Info.Hostility == 0:
			b.FaceObject(i, pid, nil)
		default:
			b.Flee(i, pid, 0, nil)
		}
	case KindParty:
		if Relation(b.Tables, a, nil) == 0 {
			b.FaceObject(i, pid, nil)
			break
		}
		a.Vel[0], a.Vel[1] = 0, 0
		if m := b.Party.Members; m != nil && m.Buffs[party.PartyBuffInvisibility].Active() {
			m.Buffs[party.PartyBuffInvisibility] = party.Buff{}
		}
	case KindDecoration:
		x, y, _ := b.decorationPos(int(pid >> 3))
		speed := int32(physics.Isqrt(uint32(int32(a.Vel[1])*int32(a.Vel[1]) + int32(a.Vel[0])*int32(a.Vel[0]))))
		ang := physics.Atan2(int32(a.Pos[0])-x, int32(a.Pos[1])-y)
		a.Vel[0] = int16(physics.Mul16(speed, physics.Cos(ang)))
		a.Vel[1] = int16(speed * physics.Cos(ang-physics.QuarterTurn) >> 16)
	default:
		return false
	}
	return true
}

// slideFace is a face hit's effect: a floor stops the fall and puts the actor on it
// (stopping it when slow); other faces push the velocity out along the normal (at
// least an eighth of the speed) and, but for sloped floors, push the actor out to its
// radius and turn it along the new velocity (pushOut false: turn-based outdoors).
func slideFace(a *Actor, st *physics.State, n [3]int32, dist int32, polyType uint8, floorZ int32, pushOut bool) {
	if polyType == blv.PolyFloor {
		a.Vel[2] = 0
		a.Pos[2] = int16(floorZ + 1)
		if int32(a.Vel[1])*int32(a.Vel[1])+int32(a.Vel[0])*int32(a.Vel[0]) < minSpeedSq {
			a.Vel = [3]int16{}
		}
		return
	}
	k := abs32(int32(a.Vel[2])*n[2]+int32(a.Vel[1])*n[1]+int32(a.Vel[0])*n[0]) >> 16
	k = max(k, st.Speed>>3)
	a.Vel[0] += int16(physics.Mul16(n[0], k))
	a.Vel[1] += int16(physics.Mul16(n[1], k))
	a.Vel[2] += int16(physics.Mul16(n[2], k))
	if polyType == blv.PolySlopedFloor {
		return
	}
	d := st.RadiusLo - (int32(a.Pos[0])*n[0]+int32(a.Pos[1])*n[1]+int32(a.Pos[2])*n[2]+dist)>>16
	if !pushOut {
		return
	}
	if d > 0 {
		a.Pos[0] += int16(uint32(d*n[0]) >> 16)
		a.Pos[1] += int16(uint32(n[1]*d) >> 16)
		a.Pos[2] += int16(uint32(n[2]*d) >> 16)
	}
	a.Yaw = uint16(physics.Atan2(int32(a.Vel[0]), int32(a.Vel[1])))
}

// damp is the velocity scale after each hit.
func damp(a *Actor) {
	for k := range a.Vel {
		a.Vel[k] = int16(int64(a.Vel[k]) * friction >> 16)
	}
}

// moves reports an actor the movers move: not removed, disabled or summoned, with a
// speed.
func moves(a *Actor) bool {
	switch a.AIState {
	case Removed, Disabled, Summoned:
		return false
	}
	return a.Speed != 0
}

// MoveIndoor moves the actors of an indoor map for a frame of ticks.
//
// mm8: 0x46fb6d (Actors_MoveIndoor)
func (b *Brain) MoveIndoor(ticks int32) {
	g := b.Indoor
	m := g.Map
	dt, dtFixed := timeStep(ticks)
	grav := physics.Gravity(false)
	for i := range b.Actors {
		a := &b.Actors[i]
		if !moves(a) {
			continue
		}
		sector := int(a.Sector)
		floorZ, floorFace := g.FloorZSector(int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2]), &sector)
		a.Sector = int16(sector)
		fly := a.Info.Fly != 0 && a.CanAct()
		airborne := floorZ+1 < int32(a.Pos[2])
		if floorZ < -29999 {
			s := g.SectorAt(int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2]))
			a.Sector = int16(s)
			if s == 0 {
				continue
			}
			if floorZ, floorFace = g.FloorZ(int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2]), s); floorZ == physics.NoFloor {
				continue
			}
		}
		walkVelocity(a, fly)
		ff := &m.Faces[floorFace]
		switch {
		case int32(a.Pos[2]) < floorZ:
			a.Pos[2] = int16(floorZ + 1)
			if ff.PolyType == blv.PolyFloor {
				if a.Vel[2] < 0 {
					a.Vel[2] = 0
				}
			} else if ff.Normal[2] < steepFloor {
				a.Vel[2] -= int16(grav) * int16(dt)
			}
		case airborne && !fly:
			a.Vel[2] += int16(grav) * int16(dt) * -8
		}
		if int32(a.Vel[2])*int32(a.Vel[2])+int32(a.Vel[1])*int32(a.Vel[1])+int32(a.Vel[0])*int32(a.Vel[0]) < minSpeedSq {
			a.Vel = [3]int16{}
			if ff.Attr&attrNoWalk != 0 && a.AIState == Dead {
				a.AIState = Removed
			}
			continue
		}
		st := sweepState(a, int32(int16(a.Radius)))
		for range maxMoveIters {
			if !b.startSweep(&st, a, int32(a.Sector), dtFixed) {
				break
			}
			touches := 0
			for range maxMoveIters {
				g.CollideFaces(&st, true, false)
				g.CollideDecorations(&st)
				st.SweepParty(b.Party.X, b.Party.Y, b.Party.Z, b.Party.Radius, b.Party.Height, false)
				// Collide_Objects (projectile impacts) is M9's.
				touches += b.collideOthers(&st, i, true)
				if !g.CollidePortals(&st) {
					break
				}
			}
			crowded := touches > 1
			nx, ny, nz := st.NewPosLo[0], st.NewPosLo[1], st.NewPosLo[2]-st.RadiusLo-1
			if st.AdjustedDist < st.MoveDist {
				nx = int32(a.Pos[0]) + physics.Mul16(st.Dir[0], st.AdjustedDist)
				ny = int32(a.Pos[1]) + physics.Mul16(st.Dir[1], st.AdjustedDist)
				nz = int32(a.Pos[2]) + physics.Mul16(st.Dir[2], st.AdjustedDist)
			}
			sec := int(st.Sector)
			fz, nf := g.FloorZSector(nx, ny, nz, &sec)
			st.Sector = int32(sec)
			noWalk := m.Faces[nf].Attr & attrNoWalk
			switch {
			case noWalk != 0 && a.AIState == Dead:
				a.AIState = Removed
				continue
			case !airborne && !fly && noWalk != 0:
				if a.Info.Hostility == 0 || crowded {
					b.StandOrBored(i, partyPID, 0, facing(a))
					break
				}
				continue
			case fz == physics.NoFloor:
				continue
			case a.Animation == tables.AnimWalk && fz < int32(a.Pos[2])-ledgeDrop && !airborne && !fly:
				if a.Pos[0]&1 != 0 {
					a.Yaw += ledgeDrop
				} else {
					a.Yaw -= ledgeDrop
				}
				continue
			case st.MoveDist <= st.AdjustedDist:
				a.Pos = [3]int16{int16(st.NewPosLo[0]), int16(st.NewPosLo[1]), int16(st.NewPosLo[2]) - int16(st.RadiusLo) - 1}
				a.Sector = int16(st.Sector)
			default:
				advance(a, &st)
				a.Sector = int16(st.Sector)
				st.TotalMoved += st.AdjustedDist
				pid := uint32(st.PID)
				if pid&7 == KindFace {
					fi := int(pid >> 3)
					f := &m.Faces[fi]
					st.IgnorePID = int32(fi)
					floor := int32(m.Vertices[f.Verts[0]].Z)
					slideFace(a, &st, f.Normal, f.Dist, f.PolyType, floor, true)
				} else {
					b.bump(i, pid, crowded)
				}
				damp(a)
				continue
			}
			break
		}
	}
}

// MoveOutdoor moves the actors of an outdoor map for a frame of ticks. A corpse that
// lands in water sinks with a splash (and, as the original returns there, the rest
// stand still this frame); a walker may not step between land and water unless the
// water gate allows it (the water monsters the other way round), else it turns and
// flees.
//
// mm8: 0x470945 (Actors_MoveOutdoor)
func (b *Brain) MoveOutdoor(ticks int32) {
	g := b.Outdoor
	dt, dtFixed := timeStep(ticks)
	grav := physics.Gravity(g.PlaneOfWater)
	for i := range b.Actors {
		a := &b.Actors[i]
		x0, y0 := int32(a.Pos[0]), int32(a.Pos[1])
		if !moves(a) {
			continue
		}
		floater := IsKind(int(a.Info.ID), 3)
		fly := a.Info.Fly != 0 && a.CanAct()
		a.Sector = 0
		steep := g.TooSteep(x0, y0) && !b.Party.TurnBased
		floorZ, water, _, face := g.FloorZ(x0, y0, int32(a.Pos[2]), false, floater)
		onTerrain := face == 0
		airborne := floorZ+1 < int32(a.Pos[2])
		if a.AIState == Dead && water && !airborne {
			a.AIState = Removed
			continue
		}
		walkVelocity(a, fly)
		if int32(a.Pos[2]) < floorZ {
			a.Pos[2] = int16(floorZ)
			a.Vel[2] = 0
			if fly {
				a.Vel[2] = 0x14
			}
		}
		if !airborne || fly {
			if steep && !airborne && onTerrain {
				a.Pos[2] = int16(floorZ)
				n := g.TerrainNormal(int32(a.Pos[0]), int32(a.Pos[1]))
				a.Vel[2] += int16(grav) * int16(dt) * -8
				k := abs32(int32(a.Vel[1])*n[1]+int32(a.Vel[2])*n[2]+int32(a.Vel[0])*n[0]) >> 16
				a.Vel[0] += int16(n[0] * k >> 16)
				a.Vel[1] += int16(n[1] * k >> 16)
				a.Vel[2] += int16(int64(n[2]) * int64(k) >> 16)
			}
		} else {
			a.Vel[2] -= int16(grav) * int16(dt)
		}
		// 0xbb2d04 (Armageddon's shaking) is M9's.
		if int32(a.Vel[1])*int32(a.Vel[1])+int32(a.Vel[0])*int32(a.Vel[0]) < minSpeedSq && !steep {
			a.Vel[0], a.Vel[1] = 0, 0
		}
		r := int32(outdoorRadius)
		if a.Info.Fly != 0 {
			r = int32(int16(a.Radius))
		}
		st := sweepState(a, r)
		for range maxMoveIters {
			if !b.startSweep(&st, a, 0, dtFixed) {
				break
			}
			g.CollideModels(&st, true, false)
			g.CollideCellDecorations(&st, physics.GridX(int32(a.Pos[0])), physics.GridY(int32(a.Pos[1])))
			st.SweepParty(b.Party.X, b.Party.Y, b.Party.Z, b.Party.Radius, b.Party.Height, false)
			crowded := b.collideOthers(&st, i, false) > 1
			nz := st.NewPosLo[2] - st.RadiusLo - 1
			fz, _, _, _ := g.FloorZ(st.NewPosLo[0], st.NewPosLo[1], nz, false, false)
			if water && nz < fz+splashHeight {
				switch a.AIState {
				case Dead, Dying, Removed, Disabled:
					b.stub("splash", "Splash_Spawn 0x42ee72: a sinking corpse's splash (M9)")
					a.AIState = Removed
					return
				}
			}
			if st.MoveDist <= st.AdjustedDist {
				a.Pos = [3]int16{int16(st.NewPosLo[0]), int16(st.NewPosLo[1]), int16(nz)}
				break
			}
			advance(a, &st)
			st.TotalMoved += st.AdjustedDist
			pid := uint32(st.PID)
			if pid&7 == KindFace {
				mi, fi := int(pid>>9), int(pid>>3)&0x3f
				f := &g.Map.Models[mi].Faces[fi]
				if f.Attr&physics.AttrEthereal == 0 {
					floor := g.Map.Models[mi].Vertices[f.Verts[0]].Z
					slideFace(a, &st, f.Normal, f.Dist, f.PolyType, floor, !b.Party.TurnBased)
				}
			} else {
				b.bump(i, pid, crowded)
			}
			damp(a)
		}
		b.waterGate(a, x0, y0, onTerrain, floater, fly)
	}
}

// waterGate keeps a walker on its side of the shore: unless it stayed in one land cell
// or stands on a model, a move the gate refuses is undone and the actor turns away
// and flees for 128 ticks.
func (b *Brain) waterGate(a *Actor, x0, y0 int32, onTerrain, floater, fly bool) {
	g := b.Outdoor
	gx0, gy0 := physics.GridX(x0), physics.GridY(y0)-1
	gx1, gy1 := physics.GridX(int32(a.Pos[0])), physics.GridY(int32(a.Pos[1]))-1
	land0 := g.Attr(gx0, gy0)&physics.TileWater == 0
	land1 := g.Attr(gx1, gy1)&physics.TileWater == 0
	if gx0 == gx1 && gy0 == gy1 && land0 || !onTerrain {
		return
	}
	if floater {
		land0, land1 = !land0, !land1
	}
	if ok, _ := g.WaterGate(land0, land1, fly, false, false, false, gx1, gy1); ok {
		return
	}
	a.Pos[0], a.Pos[1] = int16(x0), int16(y0)
	if a.CanAct() {
		a.Yaw -= 0x20
		a.ActionTime, a.ActionLength, a.AIState = 0, 0x80, Flee
	}
}
