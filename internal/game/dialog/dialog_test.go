package dialog

import (
	"fmt"
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/npc"
	"libre-enroth/internal/game/party"
)

type fakeHost struct {
	t      *tables.All
	npcs   *npc.State
	m      *party.Members
	ran    []int
	status []string
	hidden map[int]bool
	notes  []string
	auto   []int
	reply  func(id int) string
	d      **Dialog
	ctx    *party.Ctx
	loc    []byte
	mapNow string
	moves  []string
	spoke  []int
	glob   map[int]string // global.txt overrides
}

func (h *fakeHost) Tables() *tables.All     { return h.t }
func (h *fakeHost) NPCs() *npc.State        { return h.npcs }
func (h *fakeHost) Members() *party.Members { return h.m }
func (h *fakeHost) Global(i int) string {
	if g, ok := h.glob[i]; ok {
		return g
	}
	switch i {
	case 0x19e:
		return "This place is open from %d%s to %d%s"
	case 0x1d8:
		return "am"
	case 0x1d9:
		return "pm"
	case 0x1ad:
		return "%s the %s"
	case 0x19b:
		return "Enter %s"
	case 0x199:
		return "Do you wish to leave %s?"
	}
	return fmt.Sprintf("g%#x", i)
}
func (h *fakeHost) MapStatsName(i int) string { return fmt.Sprintf("map%d", i) }
func (h *fakeHost) RunTopic(id int) {
	h.ran = append(h.ran, id)
	if h.reply != nil && h.d != nil {
		(*h.d).Reply = h.reply(id)
	}
}
func (h *fakeHost) CanShowTopic(id int) int {
	if h.hidden[id] {
		return 0
	}
	return 2
}
func (h *fakeHost) Status(text string, _ int) { h.status = append(h.status, text) }
func (h *fakeHost) Speak(_, id int)           { h.spoke = append(h.spoke, id) }
func (h *fakeHost) Ctx() *party.Ctx           { return h.ctx }
func (h *fakeHost) Location() []byte          { return h.loc }
func (h *fakeHost) MapName() string           { return h.mapNow }
func (h *fakeHost) MapStatsFile(i int) string { return fmt.Sprintf("out%02d.odm", i) }
func (h *fakeHost) MapStatsIndex(name string) int {
	var i int
	fmt.Sscanf(name, "out%02d.odm", &i)
	return i
}
func (h *fakeHost) Teleport(x, y, z, dir, _, _ int32) {
	h.moves = append(h.moves, fmt.Sprintf("teleport %d %d %d %d", x, y, z, dir))
}
func (h *fakeHost) MoveToMap(name string, x, y, z, dir, _, _ int32) {
	h.moves = append(h.moves, fmt.Sprintf("%s %d %d %d %d", name, x, y, z, dir))
}
func (h *fakeHost) SetAutonote(n int) { h.auto = append(h.auto, n) }
func (h *fakeHost) Note(what string)  { h.notes = append(h.notes, what) }

func newHost() *fakeHost {
	nt := &tables.NPCTables{
		NPCs:      make([]tables.NPC, tables.NumNPCs+1),
		Greetings: [][2]string{{}, {"Hello", "Back again?"}},
		GroupNews: make([]int16, tables.NumGroups),
		News:      make([]string, tables.NumGroups),
	}
	nt.NPCs[10] = tables.NPC{Name: "Tenner", Portrait: 900, House: 5, Greeting: 1, Topics: [6]int32{100, 101, 102, 103, 104, 105}}
	nt.NPCs[11] = tables.NPC{Name: "Eleven", Portrait: 901, House: 5, Join: 1, Topics: [6]int32{603, 320}}
	nt.NPCs[12] = tables.NPC{Name: "Gone", Portrait: 902, House: 5, Flags: npc.FlagGone}
	text := make([]string, tables.NumTexts+1)
	for i := range text {
		text[i] = fmt.Sprintf("text%d", i)
	}
	topic := make([]string, tables.NumTopics+1)
	for i := range topic {
		topic[i] = fmt.Sprintf("topic%d", i)
	}
	houses := make([]tables.House, tables.NumHouses+1)
	houses[5] = tables.House{Name: "Hut", Video: 3, Open: 6, Closed: 18, ExitPic: 2, ExitMap: 7, QBit: 30}
	houses[6] = tables.House{Name: "Shop", Video: 6, Owner: "Kervin", Title: "Blacksmith"}
	anims := make([]tables.HouseAnim, tables.NumHouseAnims)
	anims[3] = tables.HouseAnim{Video: "lporhs1", Type: 0x1d}
	anims[6] = tables.HouseAnim{Video: "lwpshp", Portrait: 0x83d, Type: 1}
	m := &party.Members{Players: []party.Player{{RosterID: 0}}, QBits: make(party.Bits, party.QBitBytes)}
	m.Roster = make([]party.Player, 50)
	for i := range m.Roster {
		m.Roster[i].RosterID = i
	}
	m.Calendar.Hour = 9
	return &fakeHost{
		t:    &tables.All{Houses: houses, HouseAnims: anims, NPC: nt, Topics: &tables.Topics{Topic: topic, Text: text}, Trans: make([]string, 10)},
		npcs: npc.New(nt), m: m, hidden: map[int]bool{},
		ctx: &party.Ctx{Rand: party.NewRand(1)}, loc: make([]byte, 0x28), mapNow: "out02.odm",
	}
}

func params(bs []Button) []int {
	var out []int
	for _, b := range bs {
		out = append(out, b.Param)
	}
	return out
}

// TestOpenHours: the house is open from Open to Closed-1; a wrapping range means open
// before Closed or from Open on; a closed house says when it opens.
//
// mm8: 0x443f4b (House_Enter)
func TestOpenHours(t *testing.T) {
	for _, c := range []struct {
		open, closed, hour int
		want               bool
	}{
		{6, 18, 5, false}, {6, 18, 6, true}, {6, 18, 17, true}, {6, 18, 18, false},
		{0, 0, 3, true}, {0, 0, 23, true}, {20, 4, 21, true}, {20, 4, 2, true}, {20, 4, 12, false},
	} {
		if got := Open(c.open, c.closed, c.hour); got != c.want {
			t.Errorf("Open(%d, %d, %d) = %v", c.open, c.closed, c.hour, got)
		}
	}
	h := newHost()
	h.m.Calendar.Hour = 20
	if d := OpenHouse(h, 5); d != nil {
		t.Fatal("opened a closed house")
	}
	if want := []string{"This place is open from 6am to 6pm"}; !slices.Equal(h.status, want) {
		t.Errorf("status %q, want %q", h.status, want)
	}
}

// TestHouseResidents: the portraits are the residents (not the gone ones), plus the exit
// icon once its quest bit is set; a resident shows the Join topic and the visible
// topics, at most 4.
//
// mm8: 0x443da1, 0x4b509f
func TestHouseResidents(t *testing.T) {
	h := newHost()
	d := OpenHouse(h, 5)
	if d == nil || d.Proprietor || len(d.Portraits) != 2 || !slices.Equal(d.Residents, []int{10, 11}) || d.ExitMap != 0 {
		t.Fatalf("house 5: %+v", d)
	}
	h.m.QBits.Set(30, true)
	d = OpenHouse(h, 5)
	if len(d.Portraits) != 3 || !d.Portraits[2].Exit || d.Portraits[2].Icon != "ticon02" || d.ExitMap != 7 {
		t.Fatalf("with the exit: %+v", d.Portraits)
	}
	if d.Portraits[0].Icon != "npc0900" {
		t.Errorf("icon %q", d.Portraits[0].Icon)
	}
	// Two visits counted: Tenner's second greeting shows in state 0.
	if g := h.npcs.Get(10).Flags; g != 2 {
		t.Errorf("visits %d", g)
	}
	h.hidden[102] = true
	d.SelectResident(0)
	if got := params(d.Buttons); !slices.Equal(got, []int{0x13, 0x14, 0x16, 0x17}) {
		t.Errorf("topics %#x", got)
	}
	if d.Greeting() != "Back again?" || d.Name() != "Tenner" || d.PortraitIcon() != "npc0900" {
		t.Errorf("greeting %q name %q", d.Greeting(), d.Name())
	}
	if l := d.Label(d.Buttons[1]); l != "topic101" {
		t.Errorf("label %q", l)
	}
	d.Click(d.Buttons[1])
	if !slices.Equal(h.ran, []int{101}) || d.State != 0x15 || d.Greeting() != "" {
		t.Errorf("ran %v state %#x", h.ran, d.State)
	}
	// Back: to the portraits, then out.
	if !d.Back() || d.Sel != 0 || len(d.Buttons) != 0 {
		t.Errorf("first back: sel %d", d.Sel)
	}
	if d.Back() {
		t.Error("second back should leave")
	}
	// The exit portrait asks to enter the map.
	d.SelectResident(2)
	if !d.OnExit() || d.ExitText() != "Enter map7" || len(d.Buttons) != 2 {
		t.Errorf("exit: %v %q %v", d.OnExit(), d.ExitText(), d.Buttons)
	}
}

// TestJoin: a roster topic (600 + k) offers k; Yes adds k to the party and the NPC is
// gone, the house reloads; quest bit 400 + k is set either way.
//
// mm8: 0x4b4d70, 0x4b2b74 (0x77/0x78)
func TestJoin(t *testing.T) {
	h := newHost()
	d := OpenHouse(h, 5)
	h.d = &d
	d.SelectResident(1)
	if got := params(d.Buttons); !slices.Equal(got, []int{ParamBio, 0x13, 0x14}) {
		t.Fatalf("Eleven's topics %#x", got)
	}
	if l := d.Label(d.Buttons[1]); l != "g0x7a" {
		t.Errorf("roster topic label %q, want Join (g0x7a)", l)
	}
	if l := d.Label(d.Buttons[2]); l != "topic320" || !slices.Equal(h.auto, []int{320 - 0xac}) {
		t.Errorf("teacher topic label %q, autonotes %v", l, h.auto)
	}
	d.Click(d.Buttons[1])
	if d.Reply != "text204" || d.State != StateJoin || d.Menu != -1 || !slices.Equal(params(d.Buttons), []int{ParamJoinYes, ParamJoinNo}) {
		t.Fatalf("offer: reply %q state %#x %v", d.Reply, d.State, d.Buttons)
	}
	d.Click(d.Buttons[0])
	if len(h.m.Players) != 2 || h.m.Players[1].RosterID != 3 || !h.m.QBits.Get(403) {
		t.Errorf("party %v", h.m.Players)
	}
	if !d.TakeBack() || len(d.Portraits) != 1 || h.npcs.Get(11).Flags&npc.FlagGone == 0 {
		t.Errorf("after join: portraits %v", d.Portraits)
	}
	// One portrait left: the back step shows Tenner's topics.
	if d.Sel != 1 || d.Menu != -1 || !d.Back() || d.Resident() != 10 {
		t.Errorf("after join back: sel %d menu %d resident %d", d.Sel, d.Menu, d.Resident())
	}
}

// TestNPCDialog: SpeakNPC's dialogue: a visit counts; the reply shows for topic states,
// the greeting otherwise; Esc ends it.
//
// mm8: 0x443b6f, 0x443441, 0x4bcbf2
func TestNPCDialog(t *testing.T) {
	h := newHost()
	d := OpenNPC(h, 10)
	h.d = &d
	h.reply = func(id int) string { return fmt.Sprint("reply", id) }
	if d.Text() != "Hello" || len(d.Buttons) != 4 {
		t.Fatalf("text %q, buttons %v", d.Text(), d.Buttons)
	}
	d.Click(d.Buttons[2])
	if d.State != 0x15 || d.Text() != "reply102" {
		t.Errorf("state %#x text %q", d.State, d.Text())
	}
	if d.Back() {
		t.Error("Back should end an NPC's dialogue")
	}
	if OpenNPC(h, 5000) != nil {
		t.Error("hirelings are M8's")
	}
	h.m.QBits.Set(0xe8, false)
	OpenNPC(h, 31)
	if !h.m.QBits.Get(0xe8) {
		t.Error("NPC 31 sets quest bit 232")
	}
}

// TestSetTopicRefresh: SetNPCTopic on the resident being talked to rebuilds the topics.
//
// mm8: 0x4446bd (case 0x27)
func TestSetTopicRefresh(t *testing.T) {
	h := newHost()
	d := OpenHouse(h, 5)
	d.SelectResident(0)
	h.npcs.SetTopic(10, 0, 0)
	h.npcs.SetTopic(10, 1, 0)
	d.NPCChanged(false, 11)
	if len(d.Buttons) != 4 {
		t.Errorf("another NPC's topics changed: %v", d.Buttons)
	}
	d.NPCChanged(false, 10)
	if got := params(d.Buttons); !slices.Equal(got, []int{0x15, 0x16, 0x17, 0x18}) {
		t.Errorf("after SetNPCTopic: %#x", got)
	}
}

// TestProprietor: a shop's clip has a proprietor portrait first, selected at once when
// alone, with the shop's service menu.
//
// mm8: 0x443da1, 0x41c235 (type 0x19 with one portrait), 0x4b496e
func TestProprietor(t *testing.T) {
	h := newHost()
	d := OpenHouse(h, 6)
	if !d.Proprietor || len(d.Portraits) != 1 || d.Portraits[0].Icon != "npc2109" || d.Sel != 1 || !d.OnProprietor() {
		t.Fatalf("shop: %+v", d)
	}
	if got := params(d.Buttons); !slices.Equal(got, []int{2, 0x5f, 0x5e, 0x60}) {
		t.Errorf("menu %#x", got)
	}
	if d.Back() {
		t.Error("a one-portrait house is left by the first back step")
	}
}
