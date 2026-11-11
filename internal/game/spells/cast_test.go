package spells

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
)

// fake is the cast queue's world for the seeded tests.
type fake struct {
	rand     []int
	ri       int
	hour     int
	minute   int
	tb       bool
	sp       int
	cursed   bool
	can      bool
	raw      [39]uint16
	level    uint16
	rec      int
	paid     int
	spent    int
	recovery int
	spoken   int
	nomana   int
	failed   int
	notes    int
	shots    []Shot
	spell    tables.Spell
}

func (f *fake) Rand() int {
	if f.ri >= len(f.rand) {
		return 0
	}
	v := f.rand[f.ri]
	f.ri++
	return v
}
func (f *fake) Note(string, string) { f.notes++ }
func (f *fake) TurnBased() bool     { return f.tb }
func (f *fake) Hour() (int, int)    { return f.hour, f.minute }
func (f *fake) Player(int) (Player, bool) {
	return Player{CanAct: f.can, Cursed: f.cursed, SP: f.sp, Raw: f.raw, Level: func(int) uint16 { return f.level }}, true
}
func (f *fake) Pay(_, cost int) bool {
	if f.sp < cost {
		return false
	}
	f.sp -= cost
	f.paid += cost
	return true
}
func (f *fake) Spend(_, cost int)                   { f.sp -= cost; f.spent += cost }
func (f *fake) Recover(_, ticks int)                { f.recovery = ticks }
func (f *fake) Speak(int, int)                      { f.spoken++ }
func (f *fake) NoMana()                             { f.nomana++ }
func (f *fake) Failed()                             { f.failed++ }
func (f *fake) ClearInvisibility()                  {}
func (f *fake) Spell(int) tables.Spell              { return f.spell }
func (f *fake) Recovery(int, bool) int              { return f.rec }
func (f *fake) Target(int, int) int32               { return 0 }
func (f *fake) Aim(int, int32) (int32, int32)       { return 10, 20 }
func (f *fake) Place() (int32, int32, int32, int32) { return 100, 200, 300, 192 }
func (f *fake) Sector(int32, int32, int32) int16    { return 0 }
func (f *fake) Obj(int16) (int16, int16)            { return 3, 400 }
func (f *fake) Hand(int, int) items.Item            { return items.Item{Number: 1} }
func (f *fake) Fire(s Shot) int                     { f.shots = append(f.shots, s); return len(f.shots) }

func fireBolt() tables.Spell {
	return tables.Spell{School: 0, Cost: [4]uint16{2, 2, 2, 2}, Recovery: [4]uint16{110, 110, 100, 90}, Object: 1020}
}

// Enqueue clears the queue down to the new cast. CancelPending drops a slot whose
// flags meet 0x3ca and leaves the others.
//
// mm8: 0x4263ca, 0x426787
func TestEnqueueAndCancel(t *testing.T) {
	var q Queue
	roster := []int{4, 5}
	if q.Enqueue(2, 0, 0, 0, 0, roster) != 0 || q.At(0).Spell != 2 || q.At(0).Roster != 4 {
		t.Fatalf("first slot %+v", q.At(0))
	}
	if q.Enqueue(6, 1, 0, Pending, 7, roster) != 0 || q.At(0).Spell != 6 || q.At(0).Flags != Pending || q.At(1).Spell != 0 {
		t.Fatalf("replaced %+v %+v", q.At(0), q.At(1))
	}
	q.Enqueue(2, 0, 0, 0, 0, roster)
	q.Enqueue(6, 0, 0, Pending, 0, roster)
	// The second enqueue emptied the first. Put a plain cast beside a pending one
	// by writing the slot after cancel's target exists.
	q.s[1] = Slot{Spell: 2, Roster: 4}
	q.CancelPending()
	if q.At(0).Spell != 0 || q.At(1).Spell != 2 {
		t.Fatalf("cancel %+v %+v", q.At(0), q.At(1))
	}
}

// A fire bolt spends its row's SP and recovers from that row. Noon makes a light
// spell free, midnight a dark one. A cursed caster fails half the time and still
// pays. An unknown spell is noted and not paid for.
//
// mm8: 0x42685f
func TestDispatchCosts(t *testing.T) {
	f := &fake{can: true, sp: 20, level: 1, spell: fireBolt()}
	var q Queue
	q.s[0] = Slot{Spell: 2, Roster: 0}
	q.Dispatch(f)
	if f.paid != 2 || f.sp != 18 || len(f.shots) != 1 || f.shots[0].Z != 300+192/3 || f.recovery != party.SpellDelay(110, false) {
		t.Fatalf("bolt paid %d sp %d z %d rec %d shots %d", f.paid, f.sp, shotZ(f), f.recovery, len(f.shots))
	}
	if f.spoken != 1 {
		t.Errorf("speech %d", f.spoken)
	}

	f = &fake{can: true, sp: 1, spell: fireBolt()}
	q = Queue{}
	q.s[0] = Slot{Spell: 2}
	q.Dispatch(f)
	if f.nomana != 1 || f.paid != 0 || q.At(0).Spell != 0 || f.recovery != 0 {
		t.Fatalf("no sp mana %d paid %d slot %d rec %d", f.nomana, f.paid, q.At(0).Spell, f.recovery)
	}

	// Light is skill 0x13: spells 0x4e..0x58. Noon, minute 0.
	f = &fake{can: true, sp: 5, hour: 12, spell: tables.Spell{Cost: [4]uint16{4, 4, 4, 4}, Object: 1}}
	q = Queue{}
	q.s[0] = Slot{Spell: 0x4e}
	q.Dispatch(f)
	if f.paid != 0 || len(f.shots) != 1 {
		t.Fatalf("noon paid %d shots %d", f.paid, len(f.shots))
	}
	f.hour, f.minute = 0, 0
	f.shots = nil
	q.s[0] = Slot{Spell: 0x60} // dark, and a projectile
	q.Dispatch(f)
	if f.paid != 0 || len(f.shots) != 1 {
		t.Fatalf("midnight paid %d shots %d mana %d fail %d notes %d sp %d slot %d", f.paid, len(f.shots), f.nomana, f.failed, f.notes, f.sp, q.At(0).Spell)
	}

	// Cursed: rand()%100 < 50 fails. 10 is a failure, 80 is not.
	f = &fake{can: true, cursed: true, sp: 10, rand: []int{10}, spell: fireBolt()}
	q = Queue{}
	q.s[0] = Slot{Spell: 2}
	q.s[1] = Slot{Spell: 6}
	q.Dispatch(f)
	if f.failed != 1 || f.spent != 2 || f.paid != 0 || len(f.shots) != 0 || q.At(1).Spell != 6 {
		t.Fatalf("curse fail spent %d paid %d shots %d slot1 %d", f.spent, f.paid, len(f.shots), q.At(1).Spell)
	}
	f.rand, f.ri, f.failed, f.spent = []int{80}, 0, 0, 0
	q.s[0] = Slot{Spell: 2}
	q.s[1] = Slot{}
	q.Dispatch(f)
	if f.failed != 0 || f.paid != 2 || len(f.shots) != 1 {
		t.Fatalf("curse pass paid %d shots %d", f.paid, len(f.shots))
	}

	f = &fake{can: true, sp: 10}
	q = Queue{}
	q.s[0] = Slot{Spell: 1}
	q.Dispatch(f)
	if f.notes != 1 || f.paid != 0 || q.At(0).Spell != 0 {
		t.Fatalf("unknown notes %d paid %d", f.notes, f.paid)
	}
	q.s[0] = Slot{Spell: 3}
	q.Dispatch(f)
	if f.notes != 2 {
		t.Fatalf("second note %d", f.notes)
	}
}

func shotZ(f *fake) int32 {
	if len(f.shots) == 0 {
		return -1
	}
	return f.shots[0].Z
}

// A master of the bow fires twice, the second 0x20 higher. A breath sits at half
// height and is not paid for. A pending slot is not dispatched.
//
// mm8: 0x42685f (0x85, 0x89)
func TestBowAndPending(t *testing.T) {
	f := &fake{can: true, sp: 5, raw: [39]uint16{5: party.SkillMaster | 3}, rec: 40}
	var q Queue
	q.s[0] = Slot{Spell: 0x85}
	q.Dispatch(f)
	if f.paid != 0 || len(f.shots) != 2 || f.shots[1].Z != f.shots[0].Z+0x20 || f.shots[0].Attrib&0x100 == 0 {
		t.Fatalf("bow shots %d paid %d", len(f.shots), f.paid)
	}
	if f.recovery != party.SpellDelay(40, false) {
		t.Errorf("bow recovery %d", f.recovery)
	}
	f.shots, f.recovery = nil, 0
	f.rec = 15
	q.s[0] = Slot{Spell: 0x89}
	q.Dispatch(f)
	if len(f.shots) != 1 || f.shots[0].Z != 300+192/2 {
		t.Fatalf("breath z %d shots %d", shotZ(f), len(f.shots))
	}
	q.s[0] = Slot{Spell: 2, Flags: Pending}
	f.shots = nil
	q.Dispatch(f)
	if len(f.shots) != 0 || q.At(0).Spell != 2 {
		t.Fatalf("pending dispatched, shots %d spell %d", len(f.shots), q.At(0).Spell)
	}
}
