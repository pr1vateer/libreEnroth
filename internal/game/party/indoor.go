package party

import (
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/blv"
)

// MoveIndoor runs one frame of the party through an indoor map: ticks is the frame's
// length in 128ths of a second. It finds the floor (falling back to the last good
// position when there is none), applies the queued actions and gravity, then sweeps
// the party along its velocity, sliding off what it hits, up to 100 times.
//
// mm8: 0x472e86 (Party_MoveIndoor)
func (p *Party) MoveIndoor(g *physics.IndoorGeo, ticks int32) {
	dt, dtFixed := timeStep(ticks)
	p.ticks += int64(dt)
	m := g.Map
	x, y, z := p.X, p.Y, p.Z
	z0 := z
	vz := p.VZ
	var vx, vy int32
	event := 0
	ran, walked := false, false

	sector := g.SectorAt(x, y, z)
	floorZ, floorFace := g.FloorZSector(x, y, z+0x28, &sector)
	p.Flying = false
	if floorZ == physics.NoFloor {
		floorZ, floorFace = g.FloorZNudge(x, y, z+0x28, &sector)
		if floorZ == physics.NoFloor {
			p.X, p.Y, p.Z = p.lastGood[0], p.lastGood[1], p.lastGood[2]
			p.FallStartZ = p.lastGood[2]
			p.unbind()
			return
		}
	}
	p.lastGood = [3]int32{p.X, p.Y, p.Z}

	airborne := false
	if p.FeatherFall {
		airborne = true
		p.FallStartZ = floorZ
	}
	if p.FallStartZ-z0 > fallHurts && !airborne && z0 <= floorZ+1 {
		p.fallDamage(p.FallStartZ - z0)
	}
	airborne = floorZ+1 < z0
	near := z0-floorZ < stepUp+1
	if near {
		p.FallStartZ = z0
	}
	if p.StepTimer > 0 {
		p.StepTimer -= dt
	}
	if !airborne {
		p.FallStartZ = floorZ + 1
		p.unbind()
		z = floorZ + 1
		if ff := &m.Faces[floorFace]; p.FloorFace != int32(floorFace) && p.pressurePlate(ff.Attr, ff.Attr) {
			event = faceEvent(m, ff)
		}
		p.FloorFace = int32(floorFace)
	}
	fluid := m.Faces[floorFace].Attr&physics.AttrFluid != 0

	dir, look := p.Dir, p.Look
	step := p.turnStep(dtFixed)
	w := p.WalkSpeed
	for p.Actions.Len() > 0 {
		switch a := p.Actions.Pop(); a {
		case TurnLeft:
			p.turn(&dir, 1, 1, step)
		case TurnRight:
			p.turn(&dir, -1, 1, step)
		case RunTurnLeft:
			p.turn(&dir, 1, 2, step)
		case RunTurnRight:
			p.turn(&dir, -1, 2, step)
		case StrafeLeft:
			vx -= physics.Mul16(w>>1, physics.Sin(dir))
			vy += physics.Mul16(w>>1, physics.Cos(dir))
			walked = true
		case StrafeRight:
			vx += physics.Mul16(w>>1, physics.Sin(dir))
			vy -= physics.Mul16(w>>1, physics.Cos(dir))
			walked = true
		case Forward:
			vx += physics.Mul16(w, physics.Cos(dir))
			vy += physics.Mul16(w, physics.Sin(dir))
			walked = true
		case Back:
			vx -= physics.Mul16(w, physics.Cos(dir))
			vy -= physics.Mul16(w, physics.Sin(dir))
			walked = true
		case RunForward:
			vx += physics.Mul16(w<<1, physics.Cos(dir))
			vy += physics.Mul16(w<<1, physics.Sin(dir))
			ran = true
		case RunBack:
			vx -= physics.Mul16(w, physics.Cos(dir))
			vy -= physics.Mul16(w, physics.Sin(dir))
			ran = true
		case LookUp, LookDown, LookCenter:
			p.look(&look, a)
		case Jump:
			if (!airborne || z <= floorZ+6 && vz < 1) && p.JumpStrength != 0 {
				airborne = true
				vz = p.jumpVZ(vz)
			}
		}
	}
	p.Dir, p.Look = dir, look

	grav := physics.Gravity(false)
	switch {
	case airborne:
		vz -= 2 * grav * dt
	case m.Faces[floorFace].Normal[2] < 0x8000: // steeper than 60 degrees: slide down
		vz -= grav * dt
	case p.Flags&FlagNoFallDmg == 0:
		vz = 0
	}
	switch {
	case !airborne:
		p.FallStartZ = z
	case vz > 0:
		p.FallStartZ = z // still rising
	}
	if vx*vx+vy*vy < minSpeedSq {
		vx, vy = 0, 0
	}

	s := physics.State{
		CheckHi: true, RadiusLo: p.Radius, RadiusHi: p.Radius >> 1, Height: p.Height - 0x20, IgnorePID: -1,
	}
	for range maxIter {
		s.PosLo = [3]int32{x, y, s.RadiusLo + 1 + z}
		s.PosHi = [3]int32{x, y, s.Height + 1 + z}
		s.Velocity = [3]int32{vx, vy, vz}
		s.Sector = int32(sector)
		if !s.Begin(dtFixed) {
			break
		}
		for range maxIter {
			g.CollideFaces(&s, true, true)
			g.CollideDecorations(&s)
			if p.Obstacles != nil {
				p.Obstacles.Collide(&s)
			}
			if !g.CollidePortals(&s) {
				break
			}
		}
		adj := s.AdjustedDist
		hit := adj < s.MoveDist
		nx, ny, nz := s.NewPosLo[0], s.NewPosLo[1], s.NewPosLo[2]-s.RadiusLo-1
		if hit {
			nx = x + physics.Mul16(s.Dir[0], adj)
			ny = y + physics.Mul16(s.Dir[1], adj)
			nz = z + physics.Mul16(s.Dir[2], adj)
		}
		sec := int(s.Sector)
		fz, ff := g.FloorZSector(nx, ny, nz+0x28, &sec)
		s.Sector = int32(sec)
		if fz == physics.NoFloor || fz-z > maxStepUp {
			p.unbind() // stuck: the frame's move is dropped
			return
		}
		floorFace = ff
		if !hit {
			x, y, z = nx, ny, nz
			sector = sec
			break
		}
		x += physics.Mul16(s.Dir[0], adj)
		y += physics.Mul16(s.Dir[1], adj)
		z += physics.Mul16(s.Dir[2], adj)
		sector = sec
		s.TotalMoved += adj
		switch s.PID & 7 {
		case physics.KindActor:
			if p.Obstacles != nil {
				p.Obstacles.Bumped()
			}
		case physics.KindDecoration:
			d := g.Decorations[s.PID>>3]
			speed := int32(physics.Isqrt(uint32(vx*vx + vy*vy)))
			a := physics.Atan2(x-d.X, y-d.Y)
			vx, vy = physics.Mul16(speed, physics.Cos(a)), physics.Mul16(speed, physics.Sin(a))
		case physics.KindFace:
			fi := int(s.PID >> 3)
			f := &m.Faces[fi]
			if slide(f, &x, &y, &z, &vx, &vy, &vz, s.MoveDist, s.RadiusLo, &p.FallStartZ, m) {
				break
			}
			if p.FloorFace != int32(fi) && p.pressurePlate(m.Faces[floorFace].Attr, f.Attr) {
				event = faceEvent(m, f)
			}
		}
		vx, vy, vz = physics.Mul16(vx, friction), physics.Mul16(vy, friction), physics.Mul16(vz, friction)
	}

	if !p.Levitate && p.StepTimer < 1 {
		falling := airborne && !near
		switch {
		case ran && !falling:
			p.Hooks.Steps(true, StepSurface{Fluid: fluid})
		case walked && !falling:
			p.Hooks.Steps(false, StepSurface{Fluid: fluid})
		default:
			p.Hooks.StopSteps()
			p.StepTimer = walkSoundGap
		}
	}
	if airborne && !near {
		p.Flags |= FlagAirborne
	} else {
		p.Flags &^= FlagAirborne
	}
	p.Flags &^= FlagBurning
	p.X, p.Y, p.VZ = x, y, vz
	if !airborne && m.Faces[floorFace].Attr&physics.AttrLava != 0 {
		p.Flags |= FlagBurning
	}
	p.Z = z
	p.Sector = sector
	if event != 0 {
		p.Hooks.FaceEvent(event)
	}
}

// slide handles a face hit: floors stop the fall and put the party on them, sloped
// floors and the rest push the velocity out along the normal (by at least an eighth of
// the move), and walls and ceilings also push the party out to its radius. It reports
// true when a sloped floor stopped the party, which skips the face's event.
//
// mm8: 0x472e86 (the pid & 7 == 6 branch)
func slide(f *blv.Face, x, y, z, vx, vy, vz *int32, moveDist, radius int32, fallStart *int32, m *blv.Map) bool {
	n := f.Normal
	switch f.PolyType {
	case blv.PolyFloor:
		if *vz < 0 {
			*vz = 0
		}
		*z = int32(m.Vertices[f.Verts[0]].Z) + 1
		if *fallStart-*z < fallHurts {
			*fallStart = *z
		}
		if *vx**vx+*vy**vy < minSpeedSq {
			*vx, *vy = 0, 0
		}
		return false
	case blv.PolySlopedFloor:
		k := push(n, *vx, *vy, *vz, moveDist)
		*vx += physics.Mul16(n[0], k)
		*vy += physics.Mul16(n[1], k)
		*vz += physics.Mul16(n[2], k)
		if *vx**vx+*vy**vy < minSpeedSq {
			*vx, *vy, *vz = 0, 0, 0
			return true
		}
		return false
	}
	k := push(n, *vx, *vy, *vz, moveDist)
	*vz += physics.Mul16(n[2], k)
	*vx += physics.Mul16(n[0], k)
	*vy += physics.Mul16(n[1], k)
	if d := radius - (n[0]**x+n[1]**y+n[2]**z+f.Dist)>>16; d > 0 {
		*x += n[0] * d >> 16
		*y += n[1] * d >> 16
		*z += n[2] * d >> 16
	}
	return false
}

// push is how hard a hit face pushes back: the velocity into it, at least an eighth of
// the frame's move.
func push(n [3]int32, vx, vy, vz, moveDist int32) int32 {
	k := abs32(n[0]*vx+n[2]*vz+n[1]*vy) >> 16
	if k < moveDist>>3 {
		k = moveDist >> 3
	}
	return k
}

// pressurePlate reports whether stepping on a face runs its event: it is a pressure
// plate, and the floor stood on is not marked to ignore levitating parties (or nobody
// levitates).
func (p *Party) pressurePlate(floorAttr, faceAttr uint32) bool {
	return (floorAttr&physics.AttrNotLevitating == 0 || !p.Levitate) && faceAttr&physics.AttrPressurePlate != 0
}

// faceEvent is the event of an indoor face, from its face extra.
func faceEvent(m *blv.Map, f *blv.Face) int {
	if int(f.Extra) < 0 || int(f.Extra) >= len(m.FaceExtras) {
		return 0
	}
	return int(m.FaceExtras[f.Extra].Event)
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
