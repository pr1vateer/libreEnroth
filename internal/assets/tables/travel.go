package tables

import (
	"encoding/binary"

	"libre-enroth/internal/assets/exe"
)

// The travel tables compiled into MM8-Rel.exe: the stables' and boats' routes, the
// arrivals of the house exits with a negative quest bit, the map-edge neighbours and
// the dungeons the transition dialogue numbers (re/notes/houses.md#travel).

// Route is a stable or boat route (0x20 bytes at 0x502d98).
type Route struct {
	Map        int     // +0x00 MapStats row of the destination
	Days       [7]bool // +0x01 the days of the week (day % 7) it leaves
	TravelDays int     // +0x08 days on the way (0 counts as 1)
	X, Y, Z    int32   // +0x0c arrival
	Dir        int32   // +0x18
	QBit       int32   // +0x1c the route needs this quest bit (0: none)
}

// NoRoute is a house's empty route slot.
const NoRoute = 0xff

// Exe addresses and sizes of the travel tables.
const (
	vaRoutes       = 0x502d98
	NumRoutes      = 25
	vaHouseRoutes  = 0x502f77 + 0x69 // [house*4 + code - 0x69]: codes 0x69..0x6c
	numRouteHouses = 0x46            // houses 54..69 are the stables and boats
	vaExitArrivals = 0x4f73dc        // x[6], y[6] at +0x18, z[6] at +0x30, dir[6] at +0x48
	NumExitArrival = 6
	vaEdge         = 0x4fe934 + 0x27 // neighbour [n*4 + side], days at +0x34, arrival at +0x68
	edgeSize       = 0x68 + 15*4 + 5 // up to out15's west side, which reads past the tables
	vaDungeons     = 0x4f90dc
	NumDungeons    = 14
)

// Arrival is a fixed arrival point.
type Arrival struct {
	X, Y, Z, Dir int32
}

// Travel holds the travel tables.
type Travel struct {
	Routes []Route
	// HouseRoutes are the stables' and boats' route numbers, 4 per house
	// ([house*4 + code - 0x69]), NoRoute for an empty slot.
	HouseRoutes []byte
	// ExitArrivals are where a house exit with quest bit -k puts the party (k = 1..6).
	ExitArrivals []Arrival
	// Edge is the raw neighbour, days and arrival tables of the outdoor maps.
	Edge []byte
	// Dungeons are the maps whose transition texts are trans.txt rows 1..14.
	Dungeons []string
}

// ReadTravel reads the travel tables from the executable.
//
// mm8: 0x4b761b (routes), 0x42f877 (msg 0xbf: the exit arrivals), 0x48a53c (edges),
// 0x4425c4 (the dungeon list)
func ReadTravel(im *exe.Image) (*Travel, error) {
	t := &Travel{}
	raw, err := im.Read(vaRoutes, NumRoutes*0x20)
	if err != nil {
		return nil, err
	}
	for i := range NumRoutes {
		r := raw[i*0x20:]
		rt := Route{Map: int(r[0]), TravelDays: int(r[8])}
		for d := range rt.Days {
			rt.Days[d] = r[1+d] != 0
		}
		v := func(o int) int32 { return int32(binary.LittleEndian.Uint32(r[o:])) }
		rt.X, rt.Y, rt.Z, rt.Dir, rt.QBit = v(0xc), v(0x10), v(0x14), v(0x18), v(0x1c)
		t.Routes = append(t.Routes, rt)
	}
	if t.HouseRoutes, err = im.Read(vaHouseRoutes, numRouteHouses*4); err != nil {
		return nil, err
	}
	a, err := im.Int32s(vaExitArrivals, 4*NumExitArrival)
	if err != nil {
		return nil, err
	}
	for i := range NumExitArrival {
		t.ExitArrivals = append(t.ExitArrivals, Arrival{a[i], a[6+i], a[12+i], a[18+i]})
	}
	if t.Edge, err = im.Read(vaEdge, edgeSize); err != nil {
		return nil, err
	}
	if t.Dungeons, err = im.CStringTable(vaDungeons, NumDungeons); err != nil {
		return nil, err
	}
	return t, nil
}

// HouseRoute is the route of a stable's or boat's button code (0x69..0x6c), NoRoute
// for none.
//
// mm8: 0x502f77[house*4 + code]
func (t *Travel) HouseRoute(house, code int) int {
	i := house*4 + code - 0x69
	if code < 0x69 || code > 0x6c || i < 0 || i >= len(t.HouseRoutes) {
		return NoRoute
	}
	return int(t.HouseRoutes[i])
}

// Route returns route i, nil for NoRoute.
func (t *Travel) Route(i int) *Route {
	if i < 0 || i >= len(t.Routes) {
		return nil
	}
	return &t.Routes[i]
}

// Dungeon is name's index in the dungeon list, -1 when not there.
//
// mm8: 0x4425c4 (_stricmp over 0x4f90dc)
func (t *Travel) Dungeon(name string) int {
	for i, d := range t.Dungeons {
		if equalFold(d, name) {
			return i
		}
	}
	return -1
}

// equalFold is _stricmp == 0 for ASCII.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
