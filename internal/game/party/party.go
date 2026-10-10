// Package party is the party: its movement (the action queue the keyboard fills and the
// per-frame indoor and outdoor move functions, ported in the original's integer
// arithmetic on top of internal/game/physics, re/notes/physics.md) and its members
// (player.go: conditions and the portrait expressions, re/notes/ui.md).
package party

import "libre-enroth/internal/game/physics"

// Defaults of the party's body.
//
// mm8: 0x492487 (Party_Ctor), 0x465936 (Ini_Load: mm6.ini [party] height/eyelevel,
// [debug] walkspeed)
const (
	DefaultHeight   = 0xc0
	DefaultEyeLevel = 0xa0
	DefaultRadius   = 0x25
	DefaultWalk     = 0x180 // units per second
	DefaultTurn     = 0x5a  // degrees per second
	DefaultJump     = 5
)

// Party flags (g_partyFlags 0xb7c988).
const (
	FlagWater       = 0x4   // standing in water
	FlagAirborne    = 0x8   // more than 0x20 above the floor after a jump or fall
	FlagWaterWalk   = 0x80  // walking on water with the water walk buff
	FlagNoFallDmg   = 0x100 // the next landing does not hurt
	FlagBurning     = 0x200 // standing in lava
	FlagMoved       = 0x400 // ran or turned fast this frame (outdoors)
	FlagBoundObject = 0x800 // carried by an object (M8)
)

// The move code's constants.
const (
	stepUp       = 0x20   // floors up to this far below count as standing on them
	fallHurts    = 0x200  // falls from higher hurt
	maxStepUp    = 0x80   // indoors, a floor this far above the new position aborts the move
	flyStep      = 0x1e   // fly up/down per action
	maxZ         = 0x1fe0 // outdoor ceiling
	friction     = 0xe484 // velocity scale after each collision
	maxIter      = 100    // collision iterations per frame
	lookStep     = 25     // per look action (25.0 * g_lookScale)
	maxLook      = 0x80
	minSpeedSq   = 400  // slower horizontal speeds stop (Physics_MinSpeedSq 0x46baaa)
	jumpScale    = 1.5  // vz += (jump << 6) * 1.5
	defaultCeil  = 4000 // outdoor flying ceiling when the map sets none
	walkSoundGap = 0x40
)

// Hooks are the side effects of the move functions. NoHooks ignores them; the
// milestones that own them plug in.
type Hooks interface {
	FaceEvent(event int)           // pressure plate: run the map event (Evt_Process, M5)
	FallDamage(height int32)       // landed after falling this far (Members.FallDamage)
	Splash(x, y, z int32)          // landed in water (Splash_Spawn 0x42ee72)
	Steps(run bool, s StepSurface) // footsteps (M11)
	StopSteps()
}

// StepSurface says what the party walks on, for the footstep sound.
type StepSurface struct {
	Fluid  bool  // indoors: the floor face is a fluid
	Face   int32 // outdoors: the BModel face pid stood on (0 = the terrain)
	GX, GY int   // outdoors: the terrain cell (row GridY-1)
}

// Obstacles are the map's actors as the party's sweeps meet them (M8b).
type Obstacles interface {
	// Collide sweeps s against the actors the party cannot walk through.
	Collide(s *physics.State)
	// Bumped: the party ran into an actor, which ends its invisibility.
	Bumped()
}

// NoHooks ignores every hook.
type NoHooks struct{}

func (NoHooks) FaceEvent(int)              {}
func (NoHooks) FallDamage(int32)           {}
func (NoHooks) Splash(int32, int32, int32) {}
func (NoHooks) Steps(bool, StepSurface)    {}
func (NoHooks) StopSteps()                 {}

// Party is the moving party: its body, position and the state the move functions keep
// between frames.
//
// mm8: 0xb20d90 (g_party, Party +0x04..+0x24, +0x6c4..+0x714, buffs +0x8a8)
type Party struct {
	Height, EyeLevel, Radius int32 // +0x04, +0x0c, +0x14
	WalkSpeed                int32 // +0x1c
	TurnSpeed                int32 // +0x20 degrees per second
	JumpStrength             int32 // +0x24

	X, Y, Z    int32 // +0x6c4 feet
	Dir        int32 // +0x6d0 2048ths of a turn
	Look       int32 // +0x6d4 -0x80..0x80, the view pitch
	VZ         int32 // +0x6f8 units per second
	FlyBaseZ   int32 // +0x700 hover height while flying
	FloorFace  int32 // +0x704 face last stood on: index indoors, pid outdoors
	StepTimer  int32 // +0x708 ticks until the footsteps may restart
	FallStartZ int32 // +0x710
	Flying     bool  // +0x714
	Flags      uint32
	Sector     int // indoors: the sector of the feet after the last move

	// Buffs; spells set them in M9 (debug toggles until then). Fly and WaterWalk are
	// assumed to have a caster with spell points left.
	Fly, WaterWalk, FeatherFall bool // party buffs 7, 18, 5
	Levitate                    bool // any member's buff 25 (Party_AnyLevitate 0x43f49f)

	// TurnDelta is the resolved TurnDelta setting: 0 turns smoothly, else each turn
	// action turns this many units (registry 1 -> 0x80, 2 -> 0x40, 3 -> 0).
	TurnDelta int32

	Actions Actions
	Hooks   Hooks
	// Obstacles are the actors the party bumps into (nil: none).
	Obstacles Obstacles

	lastGood [3]int32 // indoors: the last position with a floor (0x75e33c)
	ticks    int64    // timer ticks moved so far: the flying bob's clock
}

// New is a party with the default body standing at (x, y, z) facing dir.
func New(x, y, z, dir int32) *Party {
	p := &Party{
		Height: DefaultHeight, EyeLevel: DefaultEyeLevel, Radius: DefaultRadius,
		WalkSpeed: DefaultWalk, TurnSpeed: DefaultTurn, JumpStrength: DefaultJump,
		X: x, Y: y, Z: z, Dir: dir & physics.AngleMask, FallStartZ: z, FlyBaseZ: z,
		Hooks: NoHooks{},
	}
	p.lastGood = [3]int32{x, y, z}
	return p
}

// Teleport puts the party at a position, at rest.
func (p *Party) Teleport(x, y, z, dir int32) {
	p.X, p.Y, p.Z, p.Dir = x, y, z, dir&physics.AngleMask
	p.VZ, p.FallStartZ, p.FlyBaseZ, p.Flying = 0, z, z, false
	p.lastGood = [3]int32{x, y, z}
	p.Flags &^= FlagAirborne
	p.Actions.Clear()
}

// unbind detaches the party from an object it rides (flag 0x800).
//
// mm8: 0x44ac4a (Party_Unbind)
func (p *Party) unbind() { p.Flags &^= FlagBoundObject }

// timeStep is the per-frame timing the move code reads from g_timer: ticks (1..32 at 128
// per second) and the same in 16.16 seconds.
//
// mm8: 0x425008 (Timer_Update)
func timeStep(ticks int32) (dt, dtFixed int32) {
	dt = max(1, min(ticks, 0x20))
	return dt, (dt << 16) / 128
}

// turnStep is the smooth turn of one frame: turnSpeed degrees per second.
func (p *Party) turnStep(dtFixed int32) int32 {
	return physics.Mul16(physics.HalfTurn*p.TurnSpeed/180, dtFixed)
}

// turn applies one turn action (sign +1 left, -1 right; double for the running ones).
func (p *Party) turn(dir *int32, sign, mul, step int32) {
	d := p.TurnDelta
	if d == 0 {
		d = step * mul
	}
	*dir = (*dir + sign*d) & physics.AngleMask
}

// walk adds speed along angle a to the velocity.
func walk(vx, vy *int32, speed, a int32) {
	*vx += physics.Mul16(speed, physics.Cos(a))
	*vy += physics.Mul16(speed, physics.Sin(a))
}

// look applies a look action.
func (p *Party) look(look *int32, a Action) {
	switch a {
	case LookUp:
		*look = min(*look+lookStep, maxLook)
	case LookDown:
		*look = max(*look-lookStep, -maxLook)
	case LookCenter:
		*look = 0
	}
}

// jumpVZ is the vertical speed after a jump.
func (p *Party) jumpVZ(vz int32) int32 {
	return int32(float64(p.JumpStrength<<6)*jumpScale + float64(vz))
}

// fallDamage reports a hard landing unless the flag forgives it.
func (p *Party) fallDamage(height int32) {
	if p.Flags&FlagNoFallDmg != 0 {
		p.Flags &^= FlagNoFallDmg
		return
	}
	p.Hooks.FallDamage(height)
}
