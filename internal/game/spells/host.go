package spells

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// Host is the world as the cast queue uses it. The monsters package does not import
// spells; the world implements Host.
type Host interface {
	Rand() int
	Note(key, what string)
	TurnBased() bool
	// Hour is the party's hour and minute (0 and 12 with minute 0 make dark and light free).
	Hour() (hour, minute int)
	// Player is the roster id's member. ok is false when that id is not in the party.
	Player(roster int) (Player, bool)
	// Pay spends SP. False leaves it unchanged.
	Pay(roster, cost int) bool
	// Spend subtracts cost with no check (the cursed failure, which already had the SP).
	Spend(roster, cost int)
	Recover(roster, ticks int)
	Speak(roster, id int)
	// NoMana is the "not enough spell points" line.
	NoMana()
	// Failed is the line a cursed caster gets when the cast fizzles.
	Failed()
	// ClearInvisibility drops party buff 11 when a cast starts.
	ClearInvisibility()
	Spell(id int) tables.Spell
	// Recovery is Player_GetRecovery: ranged for a bow, not for a breath or a blaster.
	Recovery(roster int, ranged bool) int
	// Target is the pid to shoot at when the slot names none.
	Target(roster, spell int) int32
	// Aim is yaw and pitch toward target, or the party's facing when target is not an actor.
	Aim(roster int, target int32) (yaw, pitch int32)
	// Place is the party's feet and its height.
	Place() (x, y, z, height int32)
	Sector(x, y, z int32) int16
	// Obj is the dobjlist index and speed of an object type. index 0 launches nothing.
	Obj(typeID int16) (index, speed int16)
	// Hand is the item in equipment slot (1 main, 2 bow), or a zero item.
	Hand(roster, slot int) items.Item
	// Fire puts one projectile into the world. The member argument of Object_Create
	// is roster+1. It reports the slot, or -1.
	Fire(s Shot) int
}

// Player is the caster the preamble reads.
type Player struct {
	CanAct bool
	Cursed bool
	SP     int
	// Raw is the skill word with no bonuses (mastery lives in its high bits).
	Raw [39]uint16
	// Level is Player_GetSkill: the same word plus its bonuses.
	Level func(skill int) uint16
}

// Shot is one projectile Object_Create should launch.
type Shot struct {
	Type              int16
	Obj               int16
	X, Y, Z           int32
	Yaw, Pitch, Speed int32
	Attrib            uint16
	Sector            int16
	Caster            int32
	Target            int32
	Spell             int32
	Level             int32
	Skill             int32
	Sound             uint16
	Item              items.Item
	Member            int32 // roster id + 1
}
