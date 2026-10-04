package dialog

import (
	"fmt"

	"libre-enroth/internal/assets/tables"
)

// Transition is the "enter?" dialogue of a MoveToMap with a house or an exit picture
// (window type 0x1a, g_screenMode 0x12), and the map-edge one (g_screenMode 0x11):
// the picture in the portrait frame, the destination's name on top and a text under
// it, OK and Close.
type Transition struct {
	// Topbar is the top bar picture ("topbar", or "topbar2" for a house entrance).
	Topbar string
	// Video is the house clip that plays on the left ("" none: the view stays).
	Video string
	// Icon is the picture: ticon%02d, "outside" at a map edge ("" none: PENDING).
	Icon string
	// Title is the destination's name, Text the question.
	Title, Text string
	// TitleY is the title's line (5 here, 4 at a map edge).
	TitleY int
	// Bar8 is false at a map edge (the left bar is not drawn there).
	Bar8 bool
}

// videoSpecial is the 2DEvents video that counts as a dungeon (no clip, the topbar).
const videoSpecial = 0x9f

// NewTransition builds the dialogue of MoveToMap to mapName with house (0: none) and
// exit picture pic: a house entrance plays the house's clip and reads the house's
// trans.txt row; the 14 dungeons of the list read rows 1..14; another exit asks "Do
// you wish to leave <this map>?".
//
// mm8: 0x4425f5 (Evt_TransitionDialog), 0x4428bc (its draw)
func NewTransition(h Host, house, pic int, mapName string) *Transition {
	t := h.Tables()
	var video int16
	if def := t.House(house); def != nil {
		video = def.Video
	}
	special := video == videoSpecial
	dungeon := -1
	if !special {
		if dungeon = t.Travel.Dungeon(mapName); dungeon != -1 {
			special = true
		}
	}
	tr := &Transition{Topbar: "topbar", TitleY: 5, Bar8: true}
	if !special && house != 0 {
		tr.Topbar = "topbar2"
	}
	if pic != 0 {
		tr.Icon = fmt.Sprintf("ticon%02d", pic)
	}
	if house != 0 && !special {
		tr.Video = t.Anim(int(video)).Video
	}
	textRow := house // g_houseVideo
	if dungeon != -1 {
		textRow = dungeon + 1
	}
	// The draw: the title is the destination while a clip plays or for a dungeon (the
	// map itself for a name "0..."), else this map.
	here := h.MapStatsIndex(h.MapName())
	title := here
	if v := t.House(textRow); (tr.Video != "" || v != nil && v.Video == videoSpecial || dungeon != -1) && (mapName == "" || mapName[0] != '0') {
		title = h.MapStatsIndex(mapName)
	}
	tr.Title = h.MapStatsName(title)
	switch {
	case textRow == 0:
		if title != 0 {
			tr.Text = cfmt(h.Global(0x199), h.MapStatsName(title))
		}
	case textRow < len(t.Trans):
		tr.Text = t.Trans[textRow] // "No transition text found!" when empty
	}
	return tr
}

// EdgeNeighbour is the map across an outdoor map's edge: x or y beyond ±0x5800 on
// out01..out15 ("out%02d.odm" of 9 characters) leads north (y > 0x5800), south, east
// (x > 0x5800) or west to the map of the edge table, with its days on the way and the
// arrival marker (1 North Start .. 4 West Start).
//
// mm8: 0x48a53c (with the tables at 0x4fe95b, +0x34 days, +0x68 arrival)
func EdgeNeighbour(t *tables.Travel, mapName string, x, y int32) (name string, days, arrival int, ok bool) {
	if len(mapName) != 9 {
		return "", 0, 0, false
	}
	// strtok(lower(name), "out") leaves "NN.", whose first two characters atoi reads.
	n := 0
	i := 0
	for i < len(mapName) && (mapName[i]|0x20 == 'o' || mapName[i]|0x20 == 'u' || mapName[i]|0x20 == 't') {
		i++
	}
	for j := i; j < i+2 && j < len(mapName) && mapName[j] >= '0' && mapName[j] <= '9'; j++ {
		n = n*10 + int(mapName[j]-'0')
	}
	if n <= 0 || n >= 0x10 {
		return "", 0, 0, false
	}
	var side int
	switch {
	case x < -0x5800:
		side = 4
	case x > 0x5800:
		side = 3
	case y < -0x5800:
		side = 2
	case y > 0x5800:
		side = 1
	default:
		return "", 0, 0, false
	}
	k := n*4 + side
	nb := int(t.Edge[k])
	if nb == 0 || nb >= 0xe {
		return "", 0, 0, false
	}
	return fmt.Sprintf("out%02d.odm", nb), int(t.Edge[0x34+k]), int(t.Edge[0x68+k]), true
}

// TravelDays is the days a map-edge journey takes (at least 1).
//
// mm8: 0x442be0
func TravelDays(days int) int { return max(days, 1) }

// NewEdgeTransition is the map-edge dialogue: "outside", the neighbour's name, "It
// will take %d days to travel to %s." and "Do you wish to leave %s?".
//
// mm8: 0x442b10 (it opens), 0x442bee (its draw)
func NewEdgeTransition(h Host, neighbour string, days int) *Transition {
	tr := &Transition{Topbar: "topbar", Icon: "outside", TitleY: 4}
	n := h.MapStatsIndex(neighbour)
	if n == 0 {
		return tr
	}
	tr.Title = h.MapStatsName(n)
	d := TravelDays(days)
	f := h.Global(0x80)
	if d == 1 {
		f = h.Global(0x297)
	}
	tr.Text = cfmt(f, d, tr.Title) + "\n \n" + cfmt(h.Global(0x7e), h.MapStatsName(h.MapStatsIndex(h.MapName())))
	return tr
}
