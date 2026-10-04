package world

import (
	"strconv"
	"strings"

	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// Rest refusals (global.txt).
const (
	txtRestTurnBased = 0x1de // "You can't rest in turn-based mode!"
	txtRestHere      = 0x1df // "You can't rest here!"
	txtRestHostile   = 0x1e0 // "There are hostile enemies near!"
)

// RestRefusal is the global.txt message that keeps the party from resting where it
// stands (0: it may rest): on water outdoors or while airborne or water walking (the
// selected member also reacts, speech 0xd), with monsters near (M8: none yet), or in
// turn-based mode.
//
// mm8: 0x42f877 (msg 0x68: Terrain_HeightAt, 0x42e9d7 monsters near, Party +0x898,
// g_partyFlags & 0x88)
func (w *World) RestRefusal() int {
	m, p := w.S.Party, w.group
	speak := func() {
		if m.Selected != 0 {
			m.Speak(m.Selected-1, 0xd, w.S.Ctx)
		}
	}
	if w.outdoor != nil {
		if _, water, _ := w.outdoor.geo.TerrainZ(p.X, p.Y, true, false, false); water {
			speak()
			return txtRestHere
		}
	}
	airborne := p.Flags&(party.FlagAirborne|party.FlagWaterWalk) != 0
	monsters := w.MonstersNear()
	switch {
	case m.TurnBased:
		return txtRestTurnBased
	case airborne:
		speak()
		return txtRestHere
	case monsters:
		speak()
		return txtRestHostile
	}
	return 0
}

// MonstersNear reports hostile monsters close to the party (M8).
//
// mm8: 0x42e9d7
func (w *World) MonstersNear() bool { return false }

// RestFood is the food a rest costs: 2, outdoors on the terrain by its tileset
// (grass 1, snow and tileset 7 3, desert 5, volcanic and swamp 4), at least 1.
//
// mm8: 0x41f370 (Rest_Build), 0x48a864 (Outdoor_RestFoodCost)
func (w *World) RestFood() int {
	if w.outdoor == nil {
		return 2
	}
	p := w.group
	if p.Flags&party.FlagAirborne != 0 {
		return 2
	}
	_, water, _, face := w.outdoor.geo.FloorZ(p.X, p.Y, p.Z, p.Levitate, p.WaterWalk)
	if face != 0 || water {
		return 2
	}
	ts := -1
	if i := w.outdoor.Map.TileIndex(physics.GridX(p.X), physics.GridY(p.Y)-1); i >= 0 && i < len(w.outdoor.tiles) {
		ts = w.outdoor.tiles[i].Tileset
	}
	switch ts {
	case 0:
		return 1
	case 1, 7:
		return 3
	case 2:
		return 5
	case 3, 6:
		return 4
	}
	return 2
}

// MapStats.txt columns of the random encounter chances (Txt_LoadMapStats 0x452a82:
// row +0x30..+0x32).
const (
	statEncounter = 12
	statMonster1  = 13
	statMonster2  = 14
)

// RestEncounter rolls the map's random encounter for a night's rest: the chance in
// MapStats (a random map's when this one is not listed), then which of its three
// monster types comes. It reports monsters that arrived; spawning them is M8, so it
// is false for now, after drawing the same random numbers.
//
// mm8: 0x42f877 (msg 0x61), 0x44f0da (Rest_SpawnEncounter)
func (w *World) RestEncounter() bool {
	rng := w.S.Ctx.Rand
	row := w.statsRow(w.tables.MapIndex(w.name))
	if row == nil {
		row = w.statsRow(rng.Int()%w.tables.numMaps() + 1)
	}
	cell := func(c int) int {
		if row == nil || c >= len(row) {
			return 0
		}
		v, _ := strconv.Atoi(strings.TrimSpace(row[c]))
		return v
	}
	if rng.Int()%100+1 > cell(statEncounter) {
		return false
	}
	roll := rng.Int()%100 + 1
	kind := 1
	if cell(statMonster1) < roll {
		kind = 2
		if cell(statMonster1)+cell(statMonster2) < roll {
			kind = 3
		}
	}
	return w.spawnEncounter(kind)
}

// spawnEncounter puts a group of monster type kind (1..3) of the map near the party
// (M8).
func (w *World) spawnEncounter(kind int) bool { return false }

// statsRow is the MapStats.txt row with number n, nil when there is none.
func (w *World) statsRow(n int) []string {
	if n <= 0 {
		return nil
	}
	for _, row := range w.tables.MapStats.Rows {
		if len(row) > 2 && strings.TrimSpace(row[0]) == strconv.Itoa(n) {
			return row
		}
	}
	return nil
}

// numMaps is the number of MapStats.txt rows (0x5e818c).
func (t *Tables) numMaps() int {
	n := 0
	for _, row := range t.MapStats.Rows {
		if len(row) > 2 {
			if _, err := strconv.Atoi(strings.TrimSpace(row[0])); err == nil {
				n++
			}
		}
	}
	return max(n, 1)
}

// RestDone is called when a full rest ends: the outdoor sun follows the clock (the
// view's lighting is synced from the calendar).
//
// mm8: 0x41f297 (Outdoor_UpdateSun, Outdoor_UpdateNight)
func (w *World) RestDone() { w.syncClock() }
