package monsters

import "libre-enroth/internal/assets/tables"

// Sprites reports whether a sprite frame's first view has a sprite (the original's
// SpriteFrame.hwSprites[0] >= 1).
type Sprites interface {
	Loaded(frame int) bool
}

// UpdateAnimation sets the animation for the AI state: stand, walk, attack, shoot, got
// hit, dying, dead or fidget. A dead actor without a dead sprite is removed. Every case
// but walking, dead and unknown states marks FlagAnimationSet.
//
// mm8: 0x4584dd (Actor_UpdateAnimation)
func (a *Actor) UpdateAnimation(s Sprites) {
	a.Flags &^= FlagAnimationSet
	switch a.AIState {
	case Stand, Interacting, Summoned:
		a.Animation = tables.AnimStand
	case Tethered:
		a.Animation = tables.AnimWalk
		return
	case Melee:
		a.Animation = tables.AnimAttack
	case Ranged1, Ranged2, Ranged3, Ranged4:
		a.Animation = tables.AnimShoot
	case Dying, Resurrected:
		a.Animation = tables.AnimDying
	case Dead:
		if s == nil || !s.Loaded(int(a.Sprites[tables.AnimDead])) {
			a.AIState = Removed
			return
		}
		a.Animation = tables.AnimDead
		return
	case Pursue, Flee:
		a.Animation = tables.AnimWalk
	case Stunned:
		a.Animation = tables.AnimGotHit
	case Fidget:
		a.Animation = tables.AnimFidget
	default:
		return
	}
	a.Flags |= FlagAnimationSet
}

// Relation is how actor a regards actor b (nil = the party): 0 friendly, else the
// hostile.txt level (4 attacks on sight).
//
// mm8: 0x401051 (Actor_GetRelation)
func Relation(t *tables.Monsters, a, b *Actor) int {
	ca, cb := 0, 0
	if a != nil {
		if a.Active(BuffBerserk) {
			return 4
		}
		ca = int(a.Info.ID)
	}
	if b != nil {
		if b.Active(BuffBerserk) {
			return 4
		}
		cb = int(b.Info.ID)
	}
	if a != nil && b != nil && b.Group != 0 && a.Group != 0 && a.Group == b.Group {
		return 0
	}
	ca, cb = tables.Class(ca), tables.Class(cb)
	ally := func(x *Actor, c int) int {
		if x.Ally > 0 {
			c = int(x.Ally)
		}
		if x.Ally == 9999 {
			c = 0
		}
		if x.Active(BuffEnslaved) {
			c = 0
		}
		return c
	}
	if a != nil {
		ca = ally(a, ca)
	}
	if b != nil {
		cb = ally(b, cb)
	}
	if a != nil && a.Active(BuffCharm) && cb == 0 {
		return 0
	}
	if b != nil && b.Active(BuffCharm) && ca == 0 {
		return 0
	}
	if a != nil && !a.Active(BuffEnslaved) && a.Flags&FlagHostile != 0 && cb == 0 {
		return 4
	}
	if b == nil || a == nil || a.Active(BuffEnslaved) || b.Flags&FlagHostile == 0 {
		if ca == 0 {
			if b != nil && !b.Active(BuffEnslaved) && b.Flags&FlagHostile != 0 {
				return 4
			}
			if t.HostileAt(cb, 0) != 0 {
				return 4
			}
			if cb > 0x42 {
				return 0
			}
			return int(t.HostileAt(0, cb))
		}
	} else if ca == 0 {
		return 4
	}
	if ca > 0x42 || cb > 0x42 {
		return 0
	}
	return int(t.HostileAt(ca, cb))
}
