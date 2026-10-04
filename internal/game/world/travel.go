package world

import (
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// The transition dialogues: a MoveToMap with a house or exit picture asks first
// (Evt_TransitionDialog), and so does walking off the edge of an outdoor map.

// transition is the open transition dialogue.
type transition struct {
	view *dialog.Transition
	// ev is the MoveToMap that asked (edge false).
	ev evt.Transition
	// edge: the map edge, to neighbour in days, arriving at marker arrival.
	edge          bool
	neighbour     string
	days, arrival int
}

// edgeLimit is how far from the centre an outdoor map goes.
const edgeLimit = 0x5800

// edgeStarts are the arrival markers of the map edges (Level_PlaceParty's 1..4).
var edgeStarts = [...]string{"Party Start", "North Start", "South Start", "East Start", "West Start"}

// Transition implements evt.Host: the dialogue opens; the game timer stops meanwhile.
//
// mm8: 0x4425f5 (Evt_TransitionDialog: Timer_Pause)
func (w *World) Transition(t evt.Transition) {
	w.transition = &transition{view: dialog.NewTransition(w, t.House, t.Pic, t.Map), ev: t}
}

// TransitionDialog is the open transition dialogue, nil for none.
func (w *World) TransitionDialog() *dialog.Transition {
	if w.transition == nil {
		return nil
	}
	return w.transition.view
}

// AnswerTransition closes the transition dialogue: OK (msg 0x19b) goes, Close or Esc
// (0x19c) stays — at a map edge, held back within it.
//
// mm8: 0x42f877 (msgs 0x19b, 0x19c; 0x5a, 0x5b at a map edge)
func (w *World) AnswerTransition(ok bool) {
	t := w.transition
	if t == nil {
		return
	}
	w.transition = nil
	switch {
	case t.edge && ok:
		w.crossEdge(t)
	case t.edge:
		w.group.ClampToMap()
	case ok:
		w.enterTransition(t.ev)
	}
}

// enterTransition is the OK of a MoveToMap's dialogue: without a position the event
// goes on after the MoveToMap; else the party takes the non-zero values (the pitch
// brings the vertical speed along), and for another map leaves for it.
//
// mm8: 0x42f877 (msg 0x19b; the 4 days a party saved in d42.blv loses are not done)
func (w *World) enterTransition(t evt.Transition) {
	if !t.Moves() {
		w.vm.Resume(t.Resume, "")
		return
	}
	p := w.group
	if t.X != 0 {
		p.X = t.X
	}
	if t.Y != 0 {
		p.Y = t.Y
	}
	if t.Z != 0 {
		p.Z, p.FallStartZ = t.Z, t.Z
	}
	if t.Dir != 0 {
		p.Dir = t.Dir & physics.AngleMask
	}
	if t.Look != 0 {
		p.Look, p.VZ = t.Look, t.VZ
	}
	if len(t.Map) > 0 && t.Map[0] == '0' {
		return
	}
	dir := t.Dir
	if dir != -1 {
		dir &= physics.AngleMask
	}
	w.vm.MapLeave()
	w.MoveToMap(t.Map, t.X, t.Y, t.Z, dir, t.Look, t.VZ)
}

// tickEdge is the end of an outdoor move: past ±0x5800 on a map with a neighbour that
// way, on foot on dry land (anything goes on the Plane of Water), the map-edge
// dialogue opens and the party waits beyond the edge; otherwise it is held back.
//
// mm8: 0x46bb13 (World_TickOutdoor), 0x442b10 (the dialogue)
func (w *World) tickEdge() {
	p := w.group
	if p.X >= -edgeLimit && p.X <= edgeLimit && p.Y >= -edgeLimit && p.Y <= edgeLimit {
		return
	}
	name, days, arrival, ok := dialog.EdgeNeighbour(w.tables.Game.Travel, w.name, p.X, p.Y)
	geo := w.outdoor.geo
	if !geo.PlaneOfWater {
		_, water, _ := geo.TerrainZ(p.X, p.Y, false, false, false)
		if p.Flags&(party.FlagWater|party.FlagAirborne|party.FlagWaterWalk|party.FlagBurning) != 0 || p.Flying || water {
			ok = false
		}
	}
	if !ok {
		p.ClampToMap()
		return
	}
	w.transition = &transition{view: dialog.NewEdgeTransition(w, name, days), edge: true,
		neighbour: name, days: days, arrival: arrival}
}

// crossEdge is the OK of the map-edge dialogue: the days pass (a rest each night),
// the food is eaten — without enough, everyone is weak — and the party arrives at the
// neighbour's marker for that side, on the ground. Flying (off the Plane of Water)
// or no longer past the edge, it is held back instead.
//
// mm8: 0x42f877 (msg 0x5a)
func (w *World) crossEdge(t *transition) {
	p := w.group
	name, _, _, ok := dialog.EdgeNeighbour(w.tables.Game.Travel, w.name, p.X, p.Y)
	if !w.outdoor.geo.PlaneOfWater && p.Flying || !ok {
		p.ClampToMap()
		return
	}
	m, ctx := w.S.Party, w.S.Ctx
	days := dialog.TravelDays(t.days)
	w.vm.MapLeave()
	m.AdvanceTime(days*24*60, 0, ctx)
	weak := func() {
		for i := range m.Players {
			m.SetCondition(i, party.CondWeak, false, ctx)
		}
		m.DaysWithoutRest++
	}
	if m.Food == 0 {
		weak()
	} else {
		m.RestHeal(0, ctx)
		if m.Food-int32(days) < 0 {
			weak()
		}
		m.EatFood(int32(days))
	}
	w.travel = &Arrival{Map: name, Start: t.arrival, Ground: true}
}
