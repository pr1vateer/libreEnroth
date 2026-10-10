package party

import (
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/maps/blv"
	"libre-enroth/internal/maps/odm"
)

// mapEdge is how far from the centre the party may go outdoors.
const mapEdge = 0x5800

// MoveOutdoor runs one frame of the party over an outdoor map (ticks as in MoveIndoor):
// floor and ceiling, actions including flying, gravity and the slide down steep
// terrain, the sweep against BModels and decorations, steep-terrain and water gating,
// then landing, the 0x1fe0 ceiling and the terrain's water and lava flags.
//
// mm8: 0x473fee (Party_MoveOutdoor)
func (p *Party) MoveOutdoor(g *physics.OutdoorGeo, ticks int32) {
	dt, dtFixed := timeStep(ticks)
	p.ticks += int64(dt)
	m := g.Map
	z0 := p.Z
	vz := p.VZ
	flyBase := p.FlyBaseZ
	event := 0
	var vx, vy int32
	x, y, z := p.X, p.Y, p.Z
	ceil := g.FlyCeiling
	if ceil < 1 {
		ceil = defaultCeil
	}
	steep := g.TooSteep(p.X, p.Y)
	jumping, ran, walked, flyPressed := false, false, false, false
	var ceilFace int32
	p.Flags &^= FlagWaterWalk
	ww := p.WaterWalk
	floorZ, water, burn, face := g.FloorZ(x, y, z0, p.Levitate, ww)
	if p.FeatherFall {
		p.FallStartZ = floorZ
	}
	if p.FallStartZ-z0 > fallHurts && !p.FeatherFall && z0 <= floorZ+1 {
		p.fallDamage(p.FallStartZ - z0)
	}
	ceilZ := int32(-1)
	if p.Flying {
		ceilZ, ceilFace = g.CeilingZ(x, y, p.Height+z0)
	}
	noFaces := face == 0
	floor1 := floorZ + 1
	if floor1 < z0 {
		jumping = true
	} else {
		ceilZ = -1
		p.Flying = false
	}
	near := z0-floorZ < stepUp+1
	if p.StepTimer > 0 {
		p.StepTimer -= dt
	}
	if !g.PlaneOfWater && !p.Fly {
		p.Flying = false
	}
	if !jumping {
		pid := face<<3 | physics.KindFace
		if p.FloorFace != pid && face != 0 && int(face>>6) < len(m.Models) {
			if f := modelFace(m, face); p.pressurePlate(f.Attr, f.Attr) {
				event = int(f.Event)
			}
		}
		p.FloorFace = pid
	}

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
			p.Flags |= FlagMoved
		case RunTurnRight:
			p.turn(&dir, -1, 2, step)
			p.Flags |= FlagMoved
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
		case LookUp, LookDown, LookCenter:
			p.look(&look, a)
		case Jump:
			if (!steep || face != 0) && !jumping && p.JumpStrength != 0 && p.Flags&(FlagWater|FlagBurning) == 0 {
				jumping = true
				vz = p.jumpVZ(vz)
			}
		case FlyUp:
			if !p.Fly && !g.PlaneOfWater {
				break
			}
			p.unbind()
			p.Flying = false
			if p.Z < ceil || jumping {
				flyBase += flyStep
				z += flyStep
				p.Flying = true
				if z > ceil {
					flyBase, z = ceil, ceil
				}
				vx, vy, vz = 0, 0, 0
				if ceilFace != 0 && z < ceilZ && ceilZ <= p.Height+z {
					// Bumped into a BModel ceiling: stop just under it.
					p.Z = ceilZ - p.Height - 0x1f
					p.Flags |= FlagNoFallDmg
					p.Actions.Clear()
					p.Flying = false
					p.FlyBaseZ = z
					flyBase = z
					z = p.Z
				}
				p.VZ = 0
				flyPressed = true
			}
		case FlyDown:
			if !p.Fly && !g.PlaneOfWater {
				break
			}
			p.unbind()
			z -= flyStep
			flyBase -= flyStep
			p.Flying = true
			flyPressed = true
			p.VZ = 0
			vz = 0
			if z <= floorZ {
				p.Flying = false
				p.Actions.Clear()
			}
		case Land:
			if p.Flying {
				p.VZ = 0
				p.Flags |= FlagNoFallDmg
			}
			p.Flying = false
			p.Actions.Clear()
			p.unbind()
		case RunForward:
			switch {
			case p.Flying:
				vx += physics.Mul16(w<<2, physics.Cos(dir))
				vy += physics.Mul16(w<<2, physics.Sin(dir))
			case steep && face == 0: // no running up or down steep terrain
				vx += physics.Mul16(w, physics.Cos(dir))
				vy += physics.Mul16(w, physics.Sin(dir))
				walked = true
			default:
				vx += physics.Mul16(w<<1, physics.Cos(dir))
				vy += physics.Mul16(w<<1, physics.Sin(dir))
				ran = true
			}
			p.Flags |= FlagMoved
		case RunBack:
			if p.Flying {
				vx -= physics.Mul16(w<<2, physics.Cos(dir))
				vy -= physics.Mul16(w<<2, physics.Sin(dir))
			} else {
				vx -= physics.Mul16(w, physics.Cos(dir))
				vy -= physics.Mul16(w, physics.Sin(dir))
				walked = true
			}
			p.Flags |= FlagMoved
		}
	}
	p.Dir, p.Look = dir, look

	if z < floorZ && !p.Flying {
		if water && vz != 0 {
			p.Hooks.Splash(x, y, floorZ)
		}
		vz = 0
		z = floorZ
		p.FallStartZ = floorZ
		p.unbind()
	}
	if p.Flying {
		// Hover at the flying height, bobbing 4 units over GetTickCount's 2.048 s.
		ms := int32(p.ticks * 1000 / 128)
		p.FallStartZ = flyBase + physics.Mul16(physics.Cos(ms), 4)
		if flyPressed {
			p.FallStartZ = flyBase
		}
		z = p.FallStartZ
	} else {
		flyBase = z
	}

	grav := physics.Gravity(g.PlaneOfWater)
	falling := false // gravity applies: reached LAB_00474d9b
	switch {
	case !jumping || p.Flying:
		if jumping { // flying
			falling = true
		} else if steep && face == 0 {
			// Slide down steep terrain.
			z = floorZ
			n := g.TerrainNormal(x, y)
			vz2 := vz - grav*dt*8
			k := abs32(n[2]*vz2+n[1]*vy+n[0]*vx) >> 16
			vx += physics.Mul16(n[0], k)
			vy += physics.Mul16(n[1], k)
			vz = vz2 + physics.Mul16(n[2], k)
		}
	default:
		vz -= 2 * grav * dt
		falling = true
	}
	if !falling || g.PlaneOfWater || vz > 0 {
		p.FallStartZ = z
	}
	if vx*vx+vy*vy < minSpeedSq && !steep {
		vx, vy = 0, 0
	}

	s := physics.State{
		CheckHi: true, RadiusLo: p.Radius, RadiusHi: p.Radius >> 1, Height: p.Height - 0x20, IgnorePID: -1,
	}
	cellX, cellY := physics.GridX(p.X), physics.GridY(p.Y)
	for range maxIter {
		s.PosLo = [3]int32{x, y, s.RadiusLo + 1 + z}
		s.PosHi = [3]int32{x, y, s.Height + 1 + z}
		s.Velocity = [3]int32{vx, vy, vz}
		s.Sector = 0
		if !s.Begin(dtFixed) {
			break
		}
		g.CollideModels(&s, true, true)
		g.CollideCellDecorations(&s, cellX, cellY)
		// Collide_Objects (projectiles hitting the party) is M9's.
		if p.Obstacles != nil {
			p.Obstacles.Collide(&s)
		}
		adj := s.AdjustedDist
		hit := adj < s.MoveDist
		nx, ny, nz := s.NewPosLo[0], s.NewPosLo[1], s.NewPosLo[2]-s.RadiusLo-1
		if hit {
			nx = x + physics.Mul16(s.Dir[0], adj)
			ny = y + physics.Mul16(s.Dir[1], adj)
			nz = z + physics.Mul16(s.Dir[2], adj)
		}
		// The flags keep what the last floor query found.
		var zX, zY, faceX, faceY int32
		_, water, burn, face = g.FloorZ(nx, ny, nz, p.Levitate, false)
		zX, water, burn, faceX = g.FloorZ(nx, y, nz, p.Levitate, false)
		zY, water, burn, faceY = g.FloorZ(x, ny, nz, p.Levitate, false)
		steepX, steepY := g.TooSteep(nx, y), g.TooSteep(x, ny)
		noFaces = faceX == 0 && faceY == 0 && face == 0
		okX, okY := true, true
		if !g.PlaneOfWater && noFaces {
			// Steep terrain above the party blocks each axis on its own.
			if steepX && z < zX {
				okX = false
			}
			if steepY && z < zY {
				okY = false
			}
			if !okX && !okY {
				var zXY int32
				zXY, water, burn, face = g.FloorZ(nx, ny, nz, p.Levitate, false)
				if g.TooSteep(nx, ny) && zXY <= z {
					okX, okY = true, true
				}
			}
		}
		if okX {
			x = nx
		}
		if okY {
			y = ny
		}
		if !hit {
			if !noFaces {
				x, y = s.NewPosLo[0], s.NewPosLo[1]
			}
			z = s.NewPosLo[2] - s.RadiusLo - 1
			break
		}
		s.TotalMoved += adj
		x, y, z = nx, ny, nz
		switch s.PID & 7 {
		case physics.KindActor:
			if p.Obstacles != nil {
				p.Obstacles.Bumped()
			}
			p.unbind()
		case physics.KindDecoration:
			d := g.Decorations[s.PID>>3]
			speed := int32(physics.Isqrt(uint32(vy*vy + vx*vx)))
			a := physics.Atan2(nx-d.X, ny-d.Y)
			vx, vy = physics.Mul16(speed, physics.Cos(a)), physics.Mul16(speed, physics.Sin(a))
			p.unbind()
		case physics.KindFace:
			p.Flying = false
			p.unbind()
			id := s.PID >> 3
			f := modelFace(m, id)
			if p.slideModel(f, m.Models[id>>6].Vertices, g.PlaneOfWater, nx, ny, nz, &x, &y, &z, &vx, &vy, &vz, s.MoveDist, s.RadiusLo) {
				break
			}
			if !p.Levitate && p.FloorFace != s.PID && p.pressurePlate(f.Attr, f.Attr) {
				p.FloorFace = s.PID
				event = int(f.Event)
			}
		}
		vx, vy, vz = physics.Mul16(vx, friction), physics.Mul16(vy, friction), physics.Mul16(vz, friction)
	}

	if !p.Levitate && p.StepTimer < 1 {
		fallingFar := jumping && !near
		surface := StepSurface{GX: physics.GridX(p.X), GY: physics.GridY(p.Y) - 1}
		if !noFaces {
			surface.Face = p.FloorFace
		}
		switch {
		case ran && !fallingFar:
			p.Hooks.Steps(true, surface)
		case walked && !fallingFar:
			p.Hooks.Steps(false, surface)
		default:
			p.Hooks.StopSteps()
			p.StepTimer = walkSoundGap
		}
	}
	if !jumping || near {
		p.Flags &^= FlagAirborne
	} else {
		p.Flags |= FlagAirborne
	}

	oGX, oGY := physics.GridX(p.X), physics.GridY(p.Y)-1
	nGX, nGY := physics.GridX(x), physics.GridY(y)-1
	fromLand := g.Attr(oGX, oGY)&physics.TileWater == 0
	toLandX := g.Attr(nGX, oGY)&physics.TileWater == 0
	toLandY := g.Attr(oGX, nGY)&physics.TileWater == 0
	if !noFaces || nGX == oGX && nGY == oGY && toLandX && toLandY {
		// On a BModel, or still on the same dry cell: no water gating.
		if water && floorZ < p.Z && z < floorZ {
			p.Hooks.Splash(x, y, z)
			p.unbind()
		}
		p.VZ, p.FlyBaseZ = vz, flyBase
		p.X, p.Y, p.Z = x, y, z
		if z > maxZ {
			p.Z, p.FallStartZ = maxZ, maxZ
		}
		if event != 0 {
			p.Hooks.FaceEvent(event)
			if p.X != x || p.Y != y || p.Z != z {
				p.unbind()
				return
			}
		}
		if p.Z < floorZ {
			p.unbind()
			p.VZ = 0
			p.Z = floor1
			if p.FallStartZ-z > fallHurts && !p.FeatherFall && z <= floor1 && !g.PlaneOfWater {
				p.fallDamage(p.FallStartZ - z)
			}
			p.FallStartZ = z
		}
		if ceilFace != 0 && p.Z < ceilZ && ceilZ <= p.Height+p.Z {
			p.unbind()
			p.Z = ceilZ - p.Height - 1
			p.FlyBaseZ = p.Z
		}
		p.Flags &^= FlagWater | FlagBurning
		return
	}

	okX, _ := g.WaterGate(fromLand, toLandX, p.Flying, ww || p.Levitate, !near, p.Levitate, nGX, oGY)
	okY, inW := g.WaterGate(fromLand, toLandY, p.Flying, ww || p.Levitate, !near, p.Levitate, oGX, nGY)
	inWater := inW // each call clears the flag first
	moved := true
	switch {
	case okX:
		p.X = x
		if okY {
			p.Y = y
		}
	case okY:
		p.Y = y
	default:
		moved = false
		if p.StepTimer < 1 {
			p.Hooks.StopSteps()
			p.StepTimer = walkSoundGap
		}
	}
	if moved && ww {
		p.Flags &^= FlagWaterWalk
		if !burn && (!toLandX || !toLandY) && !p.Flying {
			p.Flags |= FlagWaterWalk
		}
	}
	p.Z = z
	if z > maxZ {
		p.Z, p.FallStartZ = maxZ, maxZ
		p.unbind()
	}
	p.Flags &^= FlagWater | FlagBurning
	p.VZ, p.FlyBaseZ = vz, flyBase
	if inWater {
		if tz, w, b := g.TerrainZ(p.X, p.Y, p.Levitate, true, true); p.Z <= tz {
			switch {
			case b:
				p.Flags |= FlagBurning
			case w:
				p.Flags |= FlagWater
			}
		}
	}
	if event != 0 {
		p.Hooks.FaceEvent(event)
		if p.X != x || p.Y != y || p.Z != z {
			p.unbind()
			return
		}
	}
	fs := p.FallStartZ
	if p.Z < floorZ {
		p.VZ = 0
		p.unbind()
		p.Z = floor1
		fs = z
		if p.FallStartZ-z > fallHurts && !p.FeatherFall && z <= floor1 && !g.PlaneOfWater {
			p.fallDamage(p.FallStartZ - z)
		}
	}
	p.FallStartZ = fs
	if ceilFace != 0 && p.Z < ceilZ && ceilZ <= p.Height+p.Z {
		// As the original has it: this branch moves the party up by the overlap.
		p.Z += 1 + p.Height - ceilZ
		p.FlyBaseZ = p.Z
		p.unbind()
	}
}

// ClampToMap keeps the party within 0x5800 of the map centre (where no map-edge
// dialogue opens, or it was closed).
//
// mm8: 0x46bb13 (World_TickOutdoor)
func (p *Party) ClampToMap() {
	p.X = max(-mapEdge, min(mapEdge, p.X))
	p.Y = max(-mapEdge, min(mapEdge, p.Y))
}

// modelFace is BModel face id (model << 6 | face).
func modelFace(m *odm.Map, id int32) *odm.Face {
	return &m.Models[id>>6].Faces[id&0x3f]
}

// slideModel handles a BModel face hit: floors put the party on them; walls and steep
// slopes (normal z below cos 45 degrees, but not faces at most 0x20 high) push the
// velocity out along the normal and the party out to its radius, horizontally only
// when steep; gentle slopes and low faces push along the whole normal and stop slow
// parties, which skips the event (true).
//
// mm8: 0x473fee (the pid & 7 == 6 branch)
func (p *Party) slideModel(f *odm.Face, vtx []odm.Vec3, planeOfWater bool, nx, ny, nz int32, x, y, z, vx, vy, vz *int32, moveDist, radius int32) bool {
	n := f.Normal
	low := int32(f.BBox[5])-int32(f.BBox[4]) < 0x21
	steep := n[2] < 0xb52a && !planeOfWater
	switch {
	case f.PolyType == blv.PolyFloor:
		if *vz < 0 {
			*vz = 0
		}
		*z = vtx[f.Verts[0]].Z + 1
		if *vx**vx+*vy**vy < minSpeedSq {
			*vx, *vy = 0, 0
		}
	case !low && (f.PolyType != blv.PolySlopedFloor || steep):
		k := abs32(n[1]**vy+n[2]**vz+*vx*n[0]) >> 16
		if k < moveDist>>3 {
			k = moveDist >> 3
		}
		*vx += physics.Mul16(n[0], k)
		*vy += physics.Mul16(n[1], k)
		if !steep {
			*vz += physics.Mul16(n[2], k)
		}
		if d := radius - (n[0]*nx+n[1]*ny+n[2]*nz+f.Dist)>>16; d > 0 {
			*x = n[0]*d>>16 + nx
			*y = n[1]*d>>16 + ny
			if !steep {
				*z = n[2]*d>>16 + nz
			}
		}
	default:
		k := abs32(n[1]**vy+n[2]**vz+*vx*n[0]) >> 16
		if k < moveDist>>3 {
			k = moveDist >> 3
		}
		*vx += physics.Mul16(n[0], k)
		*vy += physics.Mul16(n[1], k)
		*vz += physics.Mul16(n[2], k)
		if *vx**vx+*vy**vy < minSpeedSq {
			*vx, *vy, *vz = 0, 0, 0
			return true
		}
	}
	return false
}
