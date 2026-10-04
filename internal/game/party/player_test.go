package party

import (
	"testing"

	"libre-enroth/internal/assets/desc"
)

// The MSVC runtime's rand() after srand(1) (the runtime default).
func TestRandMSVC(t *testing.T) {
	r := NewRand(1)
	for i, want := range []int{41, 18467, 6334, 26500, 19169, 15724, 11478, 29358, 26962, 24464} {
		if got := r.Int(); got != want {
			t.Fatalf("rand #%d = %d, want %d", i, got, want)
		}
	}
}

// testPFT has a one-record sequence per expression 0..0x40 (texture = id, 8 << 3
// ticks long), except 0xe: two records (textures 30, 31) of time 2 each.
func testPFT() desc.PFT {
	var p desc.PFT
	for id := 0; id <= 0x40; id++ {
		if id == 0xe {
			p = append(p,
				desc.PlayerFrame{Expr: id, Texture: 30, Time: 2, Total: 4, Flags: desc.PlayerFrameNew | desc.PlayerFrameMore},
				desc.PlayerFrame{Texture: 31, Time: 2})
			continue
		}
		p = append(p, desc.PlayerFrame{Expr: id, Texture: id, Time: 8, Total: 8, Flags: desc.PlayerFrameNew})
	}
	return p
}

// seedFor finds a generator state whose next outputs satisfy ok; the tests use it to
// steer Party_TickExpressions' rand() calls.
func seedFor(t *testing.T, ok func(r *Rand) bool) uint32 {
	t.Helper()
	for s := uint32(0); s < 1<<20; s++ {
		if ok(NewRand(s)) {
			return s
		}
	}
	t.Fatal("no seed")
	return 0
}

func TestIdleExpr(t *testing.T) {
	for _, c := range []struct{ r, want int }{
		{0, 0xd}, {24, 0xd}, {25, 0xe}, {30, 0xe}, {31, 0xf}, {43, 0x11}, {45, 0x11}, {46, 0x12},
		{63, 0x14}, {64, 0x36}, {87, 0x39}, {88, 0x1d}, {93, 0x1d}, {94, 0x1e}, {99, 0x1e},
	} {
		if got := idleExpr(c.r); got != c.want {
			t.Errorf("idleExpr(%d) = %#x, want %#x", c.r, got, c.want)
		}
	}
}

// Party_TickExpressions 0x4917dd for a healthy member.
func TestTickExpressionIdle(t *testing.T) {
	pft := testPFT()
	// rand % 5 == 0, then rand % 100 = 25..30 (expression 0xe), then any.
	seed := seedFor(t, func(r *Rand) bool {
		a, b := r.Int(), r.Int()
		return a%5 == 0 && b%100 >= 25 && b%100 < 31
	})
	rng := NewRand(seed)
	m := Members{Players: []Player{{Expr: ExprNormal, ExprLen: 0x20, ExprTime: 0x1d}}}
	p := &m.Players[0]
	m.TickExpressions(pft, 2, rng) // 0x1f < 0x20: nothing
	if p.Expr != ExprNormal || p.ExprTime != 0x1f || rng.state != seed {
		t.Fatalf("before the end: %+v", *p)
	}
	m.TickExpressions(pft, 2, rng) // 0x21 >= 0x20: idle expression 0xe, length 4 << 3
	if p.Expr != 0xe || p.ExprTime != 0 || p.ExprLen != 32 {
		t.Fatalf("idle: %+v", *p)
	}
	// The face walks the two records: (time >> 3) % 4 against times 2, 2.
	for _, c := range []struct{ ticks, tex int }{{0, 30}, {16, 30}, {8, 31}, {7, 31}} {
		p.ExprTime += uint16(c.ticks)
		if got := p.Portrait(pft, 0, rng); got != c.tex {
			t.Errorf("time %d: texture %d, want %d", p.ExprTime, got, c.tex)
		}
	}
	// At the end it goes back to normal without the 1-in-5 roll: one rand() for the
	// length, rand % 256 + 0x20.
	p.ExprTime = 30
	want := *rng
	n := want.Int()%0x100 + 0x20
	m.TickExpressions(pft, 2, rng)
	if p.Expr != ExprNormal || p.ExprTime != 0 || int(p.ExprLen) != n || *rng != want {
		t.Fatalf("back to normal: %+v, want length %#x", *p, n)
	}
	// Normal ends and the roll fails: normal again with a new length.
	seed = seedFor(t, func(r *Rand) bool { return r.Int()%5 != 0 })
	rng = NewRand(seed)
	p.ExprTime = p.ExprLen - 1
	m.TickExpressions(pft, 3, rng)
	want = *NewRand(seed)
	want.Int()
	if n := want.Int()%0x100 + 0x20; p.Expr != ExprNormal || int(p.ExprLen) != n || p.ExprTime != 0 {
		t.Fatalf("normal again: %+v, want length %#x", *p, n)
	}
}

// Party_TickExpressions 0x4917dd for members with a condition.
func TestTickExpressionCondition(t *testing.T) {
	pft := testPFT()
	rng := NewRand(1)
	m := Members{Players: []Player{{Expr: ExprNormal, ExprLen: 99, ExprTime: 5}, {Expr: ExprNormal}, {Expr: 7}, {Expr: 0x23, ExprLen: 40, ExprTime: 30}}}
	m.Players[0].Conditions[CondAfraid] = 1
	m.Players[1].Conditions[CondDead] = 1
	m.Players[1].Conditions[CondWeak] = 1 // Dead comes first
	m.Players[2].Conditions[CondZombie] = 1
	m.Players[3].Conditions[CondCursed] = 1
	m.TickExpressions(pft, 2, rng)
	for i, want := range []Player{{Expr: 5}, {Expr: ExprDead}, {Expr: 7}, {Expr: 0x23, ExprLen: 40, ExprTime: 32}} {
		p := m.Players[i]
		if p.Expr != want.Expr || p.ExprLen != want.ExprLen || p.ExprTime != want.ExprTime {
			t.Errorf("member %d: %d/%d/%d, want %d/%d/%d", i, p.Expr, p.ExprTime, p.ExprLen, want.Expr, want.ExprTime, want.ExprLen)
		}
	}
	if m.Players[1].Portrait(pft, 2, rng) != PortraitDead {
		t.Error("dead face")
	}
	m.Players[1].Conditions[CondEradicated] = 1
	if m.Players[1].MainCondition() != CondEradicated || m.Players[1].Portrait(pft, 2, rng) != PortraitEradicated {
		t.Error("eradicated face")
	}
	// 0x23 runs out: 32 + 8 = 40 is not < 40, so the condition face takes over.
	m.TickExpressions(pft, 8, rng)
	if p := m.Players[3]; p.Expr != 2 || p.ExprTime != 0 || p.ExprLen != 0 {
		t.Errorf("cursed after 0x23: %+v", p)
	}
	if rng.state != 1 {
		t.Error("conditions used rand()")
	}
}

// Player_SetExpression 0x494b61's priority rules.
func TestSetExpression(t *testing.T) {
	pft := testPFT()
	for _, c := range []struct {
		cur, expr, length int
		set               bool
	}{
		{0, 0x30, 0, true}, {1, 0x30, 0, true}, // nothing showing: always
		{5, 0x30, 0, false}, {5, 0x22, 0, true}, {11, 0x24, 7, true}, // conditions: only 0x22..0x24
		{4, 0x3a, 0, true}, {0xc, 0x3a, 0, true}, {0xc, 0x22, 0, false}, // asleep/paralyzed take 0x3a
		{0xd, 0x30, 0, true}, {0x40, 0x31, 5, true}, // idle and event faces give way
		{0x62, 0x22, 0, false}, {0x63, 0x3a, 0, false}, {100, 0x30, 0, true},
	} {
		p := Player{Expr: uint16(c.cur), ExprTime: 9, ExprLen: 99}
		p.SetExpression(pft, c.expr, c.length)
		switch {
		case !c.set && (p.Expr != uint16(c.cur) || p.ExprTime != 9):
			t.Errorf("%#x over %#x: set", c.expr, c.cur)
		case c.set && (p.Expr != uint16(c.expr) || p.ExprTime != 0):
			t.Errorf("%#x over %#x: not set", c.expr, c.cur)
		case c.set && c.length == 0 && p.ExprLen != 64:
			t.Errorf("%#x: length %d, want the sequence's 8 << 3", c.expr, p.ExprLen)
		case c.set && c.length != 0 && int(p.ExprLen) != c.length:
			t.Errorf("%#x: length %d, want %d", c.expr, p.ExprLen, c.length)
		}
	}
}

// Party_InitNewGame 0x49228d: member 1 selected, faces normal, lengths rand % 256 + 0x80.
func TestNewGame(t *testing.T) {
	m := Members{Players: make([]Player, 3)}
	m.Players[2].Conditions[CondDead] = 5
	m.Players[1].Recovery = 3
	m.NewGame(NewRand(1))
	for i, r := range []int{41, 18467, 6334} {
		p := m.Players[i]
		if p.Expr != ExprNormal || int(p.ExprLen) != r%256+0x80 || p.Recovery != 0 || p.MainCondition() != CondGood {
			t.Errorf("member %d: %+v", i, p)
		}
	}
	if m.Selected != 1 {
		t.Errorf("selected %d", m.Selected)
	}
}
