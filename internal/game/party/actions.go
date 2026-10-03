package party

// Action is one movement request queued by the keyboard handler and consumed by the
// move functions.
type Action int32

// Party actions (re/notes/physics.md).
const (
	TurnLeft     Action = 1
	TurnRight    Action = 2
	StrafeLeft   Action = 3 // half walking speed
	StrafeRight  Action = 4
	Forward      Action = 5
	Back         Action = 6
	LookUp       Action = 7 // 25 per press, up to 0x80
	LookDown     Action = 8
	LookCenter   Action = 9
	Jump         Action = 0xc
	FlyUp        Action = 0xd // 0x1e per frame
	FlyDown      Action = 0xe
	Land         Action = 0xf
	RunForward   Action = 0x10 // x2 (x4 flying)
	RunBack      Action = 0x11 // x1 (x4 flying)
	RunTurnLeft  Action = 0x12 // x2
	RunTurnRight Action = 0x13
)

// maxActions is the queue's capacity; Push drops beyond it.
const maxActions = 30

// Actions is the party's action queue.
//
// mm8: 0x75e2c0 (g_partyActions)
type Actions struct {
	n int
	a [maxActions]Action
}

// Push queues an action.
//
// mm8: 0x476be0 (Actions_Push)
func (q *Actions) Push(a Action) {
	if q.n < maxActions {
		q.a[q.n] = a
		q.n++
	}
}

// Pop takes the oldest action, 0 when empty.
//
// mm8: 0x476bf4 (Actions_Pop)
func (q *Actions) Pop() Action {
	if q.n == 0 {
		return 0
	}
	a := q.a[0]
	copy(q.a[:], q.a[1:q.n])
	q.n--
	return a
}

// Len is the number of queued actions.
func (q *Actions) Len() int { return q.n }

// Clear empties the queue (fly-up into a ceiling and landing do).
func (q *Actions) Clear() { q.n = 0 }

// Binding is one movement key binding of the original's key table.
type Binding int

// The bindings Keys reads, by their key table index.
const (
	BindForward     Binding = 0x00
	BindBackward    Binding = 0x01
	BindLeft        Binding = 0x02
	BindRight       Binding = 0x03
	BindJump        Binding = 0x05
	BindAlwaysRun   Binding = 0x13
	BindLookUp      Binding = 0x14
	BindLookDown    Binding = 0x15
	BindCenterView  Binding = 0x16
	BindFlyUp       Binding = 0x19
	BindFlyDown     Binding = 0x1a
	BindLand        Binding = 0x1b
	BindStrafeLeft  Binding = 0x1c
	BindStrafeRight Binding = 0x1d
)

// KeyState is what Keys asks about the keyboard: whether a binding is active this frame
// (held for the movement keys, pressed for the rest, as the key table's type says) and
// whether Shift and Ctrl are down.
type KeyState interface {
	Active(b Binding) bool
	Shift() bool
	Ctrl() bool
}

// Keys turns the movement bindings into actions, as the in-game keyboard handler does
// each frame. alwaysRun is the AlwaysRun toggle: running is Shift xor AlwaysRun; Ctrl
// turns the turn keys into strafes. The non-movement bindings (yell, cast, ...) belong
// to later milestones.
//
// mm8: 0x42efd9 (Input_GameKeys)
func (p *Party) Keys(k KeyState, alwaysRun bool) {
	run := k.Shift() != alwaysRun
	pick := func(walk, running Action) Action {
		if run {
			return running
		}
		return walk
	}
	q := &p.Actions
	if k.Active(BindForward) {
		q.Push(pick(Forward, RunForward))
	}
	if k.Active(BindBackward) {
		q.Push(pick(Back, RunBack))
	}
	if k.Active(BindLeft) {
		if k.Ctrl() {
			q.Push(StrafeLeft)
		} else {
			q.Push(pick(TurnLeft, RunTurnLeft))
		}
	}
	if k.Active(BindRight) {
		if k.Ctrl() {
			q.Push(StrafeRight)
		} else {
			q.Push(pick(TurnRight, RunTurnRight))
		}
	}
	if k.Active(BindJump) {
		q.Push(Jump)
	}
	for _, b := range [...]struct {
		b Binding
		a Action
	}{
		{BindLookUp, LookUp}, {BindLookDown, LookDown}, {BindCenterView, LookCenter},
		{BindFlyUp, FlyUp}, {BindFlyDown, FlyDown}, {BindLand, Land},
		{BindStrafeLeft, StrafeLeft}, {BindStrafeRight, StrafeRight},
	} {
		if k.Active(b.b) {
			q.Push(b.a)
		}
	}
}
