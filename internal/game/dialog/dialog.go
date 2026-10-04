// Package dialog is the logic of the house and NPC dialogues: who is in a house, which
// topics an NPC offers, what clicking them does, and the replies (re/notes/houses.md).
// It holds what the original keeps in its dialogue globals; internal/game/ui draws it
// and feeds it clicks, internal/game/world runs the events it starts.
package dialog

import (
	"fmt"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/npc"
	"libre-enroth/internal/game/party"
)

// Host is what a dialogue acts through (internal/game/world implements it).
type Host interface {
	Tables() *tables.All
	NPCs() *npc.State
	Members() *party.Members
	// Global is global.txt string i.
	Global(i int) string
	// MapStatsName is the name of MapStats row i ("" when none).
	MapStatsName(i int) string
	// RunTopic runs global event id from a dialogue (source 1); its StatusText and
	// ShowMessage records set the reply through SetReply.
	RunTopic(id int)
	// CanShowTopic is evt.VM.CanShowTopic: 0 hidden, 1 shown, 2 no visibility records.
	CanShowTopic(id int) int
	// Status shows a timed status line message.
	Status(text string, seconds int)
	// Speak plays member i's reaction id (Player_Speak).
	Speak(member, id int)
	// SetAutonote sets autonote n for member 0 (Evt_Set var 0xe1).
	SetAutonote(n int)
	// Note logs something a later milestone does.
	Note(what string)

	// Ctx has the party's random generator and time hooks.
	Ctx() *party.Ctx
	// Location is the map's location header (+8 reputation), nil without a map.
	Location() []byte
	// MapName is the current map's games.lod name; MapStatsFile MapStats row i's.
	MapName() string
	MapStatsFile(i int) string
	// MapStatsIndex is MapStats_Find: the row of a map file, 0 for none.
	MapStatsIndex(name string) int
	// Teleport moves the party on this map; MoveToMap sends it to another, arriving at
	// its Party Start with the non-zero values replacing it.
	Teleport(x, y, z, dir, look, vz int32)
	MoveToMap(name string, x, y, z, dir, look, vz int32)
}

// Kind is the kind of dialogue.
type Kind int

const (
	KindHouse Kind = iota // a 2DEvents house: window type 0x19, g_screenMode 0xd
	KindNPC               // one NPC (SpeakNPC): window type 10, g_screenMode 4
)

// Button params (the window buttons' msgParam) and dialogue states (0x5a52c0).
const (
	ParamBio      = 0xd  // the "Join" topic of an NPC with Join set
	ParamTopic0   = 0x13 // ..0x18: topics A..F
	ParamHire     = 0x4c // hirelings (KindHireling): hire / dismiss
	ParamMoreInfo = 0x4d
	ParamLearn    = 0x4f // a teacher's paid lesson
	ParamAward    = 0x52 // a paid award
	ParamJoinYes  = 0x77
	ParamJoinNo   = 0x78

	StateLostItem = 0x54
	StateTeacher  = 0x4e
	StateAward    = 0x51
	StateJoin     = 0x77
)

// Button messages: house topics post 0xaf, an NPC's 0x88, the proprietor's menu 0x195.
const (
	MsgNPCTopic   = 0x88
	MsgHouseTopic = 0xaf
	MsgService    = 0x195
	MsgBack       = 0x71
	MsgEnterExit  = 0xbf
)

// Button is a dialogue button in the topic column (x 0x1e0): Param is its msgParam;
// the label is worked out when drawing (Label).
type Button struct {
	Msg, Param int
}

// Portrait is a picture in a house's portrait column: the proprietor, a resident, or the
// "Other Exits" icon.
type Portrait struct {
	Icon string // icons.lod name (npc%04u, ticon0N)
	NPC  int    // the resident's NPC id, 0 for the proprietor and the exit
	Exit bool
}

// Dialog is an open dialogue.
type Dialog struct {
	h     Host
	Kind  Kind
	House int // 2DEvents id (window param), KindHouse
	Def   *tables.House
	Anim  *tables.HouseAnim
	Type  int // the house type the screen goes by: g_houseAnims[video].type (0xffd2fc)
	NPC   int // KindNPC: the NPC talked to (0x5a52c4)

	Proprietor bool       // 0x5a52cc: the clip has a proprietor portrait (first)
	Residents  []int      // 0x5a5610: the house's NPCs
	Portraits  []Portrait // 0x5a5644, 0x5a565c (count)
	ExitMap    int        // 0x5db838: the MapStats row of the "Other Exits" portrait, 0 none
	Sel        int        // 0x5a5628: 1-based portrait selected, 0 none
	State      int        // 0x5a52c0
	Menu       int        // 0xffd300: 0/1 topics or the proprietor's menu, -1 a sub-dialogue
	Reply      string     // 0xffd350
	Buttons    []Button   // the topic column of the dialogue window (0x519274)
	// NoPortraits: MoveNPC reopened house 165 and dropped its portrait buttons.
	NoPortraits bool
	// VideoOnce: MoveNPC in house 553 stops the clip looping (g_video.loop = 0).
	VideoOnce bool

	// Input is the open gold entry of the bank or the town hall, nil for none.
	Input *GoldInput
	// Closed is set when a click left the house (a room for the night, a journey, the
	// exit portrait); RestInn when the inn's rest follows (msg 0x199).
	Closed, RestInn bool
	// Traded is set by a deposit or withdrawal (0xffd34c: the leaving sound, M11).
	Traded bool

	keepReply bool                        // 0xffd344: "Party is full!" survives the back step
	donations [party.MaxMembers + 1]uint8 // 0xffd35c: the temple's count per member
	topicSlot int                         // 0xffd33c: the topic button that offered a roster character
	posted    bool                        // a back step (msg 0x71) is due
}

// Back steps (msg 0x71) a dialogue asks for after a click are collected here: the UI
// calls Back for each.
func (d *Dialog) TakeBack() bool {
	p := d.posted
	d.posted = false
	return p
}

func (d *Dialog) post() { d.posted = true }

// npc is the dialogue's current NPC: the NPC of KindNPC, the selected resident in a
// house (nil for the proprietor and the exit).
func (d *Dialog) npc() *tables.NPC {
	if d.Kind == KindNPC {
		return d.h.NPCs().Get(d.NPC)
	}
	if id := d.Resident(); id > 0 {
		return d.h.NPCs().Get(id)
	}
	return nil
}

// Resident is the NPC id of the selected house portrait, 0 for none, the proprietor or
// the exit.
//
// mm8: 0x5a5610[sel - (proprietor + 1)]
func (d *Dialog) Resident() int {
	i := d.Sel - 1
	if d.Proprietor {
		i--
	}
	if d.Sel == 0 || i < 0 || i >= len(d.Residents) {
		return 0
	}
	return d.Residents[i]
}

// MapStatsName is MapStats row i's name.
func (d *Dialog) MapStatsName(i int) string { return d.h.MapStatsName(i) }

// TopicText is npctext.txt string i ("" when none).
func (d *Dialog) TopicText(i int) string {
	if t := d.h.Tables().Topics.Text; i > 0 && i < len(t) {
		return t[i]
	}
	return ""
}

// ExitName is the name of the map the "Other Exits" portrait leads to.
func (d *Dialog) ExitName() string { return d.h.MapStatsName(d.ExitMap) }

// ExitText is the transition text of the "Other Exits" portrait: trans.txt's row for its
// map, else "Enter %s".
//
// mm8: 0x4b363b (0x4fcdfc[exitMap], else globalTxt 0x19b)
func (d *Dialog) ExitText() string {
	if t := d.h.Tables().Trans; d.ExitMap > 0 && d.ExitMap < len(t) && t[d.ExitMap] != "" {
		return t[d.ExitMap]
	}
	return cfmt(d.h.Global(0x19b), d.ExitName())
}

// NPCName is NPC id's name ("" for none).
func (d *Dialog) NPCName(id int) string {
	if n := d.h.NPCs().Get(id); n != nil {
		return n.Name
	}
	return ""
}

// PortraitIcon is the npc%04u picture of the selected NPC, "" when none.
func (d *Dialog) PortraitIcon() string {
	if n := d.npc(); n != nil {
		return fmt.Sprintf("npc%04d", n.Portrait)
	}
	return ""
}

// SelectedNPC is the NPC whose dialogue shows (KindNPC's, a house's selected resident).
func (d *Dialog) SelectedNPC() int {
	if d.Kind == KindNPC {
		return d.NPC
	}
	return d.Resident()
}

// OnExit reports the "Other Exits" portrait being selected.
func (d *Dialog) OnExit() bool { return d.Sel == len(d.Portraits) && d.ExitMap != 0 && d.Sel != 0 }

// OnProprietor reports the proprietor being selected.
func (d *Dialog) OnProprietor() bool { return d.Sel == 1 && d.Proprietor }

// OpenHouse enters house id: the closed-hours check, the residents and the portraits.
// It returns nil (after the status message) when the house is closed.
//
// mm8: 0x443f4b (House_Enter), 0x4446bd (case 2)
func OpenHouse(h Host, house int) *Dialog {
	t := h.Tables()
	def := t.House(house)
	if def == nil {
		return nil
	}
	if house == 600 || house == 601 {
		h.Note("the end-game screens of houses 600/601 (0x4bfa20)")
		return nil
	}
	m := h.Members()
	if !Open(int(def.Open), int(def.Closed), m.Calendar.Hour) {
		h.Status(ClosedText(h.Global(0x19e), int(def.Open), int(def.Closed), h.Global), 2)
		if m.Selected != 0 {
			h.Speak(m.Selected-1, 3)
		}
		return nil
	}
	// Houses below 0x35 can ban the party (party +0x344 + house*8, until a game time):
	// the town halls' fines are M6c's.
	d := &Dialog{h: h, Kind: KindHouse, House: house, Def: def, Anim: t.Anim(int(def.Video))}
	d.Type = int(d.Anim.Type)
	if d.Anim.Type == 0x23 {
		h.Note("the Adventurer's Inn roster screen (0x4caac9)")
	}
	d.loadResidents()
	// mm8: 0x41c235 (type 0x19): with one portrait it is selected at once
	if len(d.Portraits) == 1 {
		d.Sel = 1
		d.SelectResident(0)
	}
	return d
}

// Open reports a house with these hours open at hour (the 2DEvents Open/Closed columns;
// open >= closed - 1 means it is open from open on, or before closed).
//
// mm8: 0x443f4b
func Open(open, closed, hour int) bool {
	if open < closed-1 {
		return hour >= open && hour <= closed-1
	}
	return hour >= open || hour <= closed-1
}

// ClosedText formats "This place is open from %d%s to %d%s" with 12-hour times.
//
// mm8: 0x443f4b (hours past 12 lose 12 and take g_amPm[1])
func ClosedText(format string, open, closed int, global func(int) string) string {
	ampm := func(h int) (int, string) {
		if h > 12 {
			return h - 12, global(0x1d9) // g_amPm[1] "pm"
		}
		return h, global(0x1d8) // g_amPm[0] "am"
	}
	o, os := ampm(open)
	c, cs := ampm(closed)
	return cfmt(format, o, os, c, cs)
}

// loadResidents fills the portraits: the proprietor (when the clip has one), the NPCs
// living there, and the "Other Exits" icon when its quest bit allows.
//
// mm8: 0x443da1 (House_LoadResidents)
func (d *Dialog) loadResidents() {
	def, m := d.Def, d.h.Members()
	exitPic := int(def.ExitPic)
	if exitPic != 0 && def.QBit > 0 && !m.QBits.Get(int(def.QBit)) {
		exitPic = 0
	}
	d.Portraits = d.Portraits[:0]
	d.Proprietor = d.Anim.Portrait != 0
	if d.Proprietor {
		d.Portraits = append(d.Portraits, Portrait{Icon: fmt.Sprintf("npc%04d", d.Anim.Portrait)})
	}
	d.Residents = d.h.NPCs().Residents(d.House)
	for _, id := range d.Residents {
		d.Portraits = append(d.Portraits, Portrait{Icon: fmt.Sprintf("npc%04d", d.h.NPCs().Get(id).Portrait), NPC: id})
	}
	d.ExitMap = 0
	if exitPic != 0 {
		d.Portraits = append(d.Portraits, Portrait{Icon: fmt.Sprintf("ticon%02d", exitPic), Exit: true})
		d.ExitMap = int(def.ExitMap)
	}
}

// SelectResident shows portrait i's dialogue (msg 0x19a): the exit's "Enter" question,
// the proprietor's menu, or a resident's topics (at most 4: the Join topic, then the
// topics whose events CanShowTopic allows).
//
// mm8: 0x4b509f (NPC_RefreshTopics)
func (d *Dialog) SelectResident(i int) {
	d.Sel = i + 1
	d.Buttons = d.Buttons[:0]
	if d.OnExit() {
		d.Buttons = append(d.Buttons, Button{MsgBack, 0}, Button{MsgEnterExit, 1})
		return
	}
	if d.OnProprietor() {
		d.Buttons = ServiceMenu(d.Type, d.h.Members())
		d.Menu = 1
		return
	}
	if n := d.npc(); n != nil {
		d.Buttons = d.topics(n, MsgHouseTopic)
	}
	d.Menu = 1
}

// topics lists n's topic buttons: Bio/Join when n.Join, then up to 4 in all of the
// topics whose events can show.
//
// mm8: 0x4b509f, 0x41c235 (type 10)
func (d *Dialog) topics(n *tables.NPC, msg int) []Button {
	var out []Button
	if n.Join != 0 {
		out = append(out, Button{msg, ParamBio})
	}
	for k, ev := range n.Topics {
		if ev == 0 || len(out) >= 4 {
			continue
		}
		if c := d.h.CanShowTopic(int(ev)); c == 1 || c == 2 {
			out = append(out, Button{msg, ParamTopic0 + k})
		}
	}
	return out
}

// OpenNPC starts NPC id's dialogue (SpeakNPC): a visit counts; NPC 31 sets quest bit 232.
//
// mm8: 0x443b6f (Evt_SpeakNPC), 0x41c235 (type 10)
func OpenNPC(h Host, id int) *Dialog {
	n := h.NPCs().Get(id)
	if n == nil {
		h.Note(fmt.Sprintf("SpeakNPC %d: hirelings and street NPCs (M8)", id))
		return nil
	}
	npc.Visit(n)
	d := &Dialog{h: h, Kind: KindNPC, NPC: id}
	if id == 31 {
		h.Members().QBits.Set(0xe8, true)
	}
	switch npc.Kind(id) {
	case npc.KindNPC:
		d.Buttons = d.topics(n, MsgNPCTopic)
	default:
		d.Buttons = []Button{{MsgNPCTopic, ParamMoreInfo}, {MsgNPCTopic, ParamHire}}
	}
	return d
}

// topicEvent is the event of topic button param of n (0 for others).
func topicEvent(n *tables.NPC, param int) int {
	if param >= ParamTopic0 && param < ParamTopic0+6 {
		return int(n.Topics[param-ParamTopic0])
	}
	return 0
}

// Click handles a topic or service button (msgs 0xaf, 0x88, 0x195).
//
// mm8: 0x4b2b74 (house topics), 0x4bcbf2 (an NPC's topics)
func (d *Dialog) Click(b Button) {
	switch b.Msg {
	case MsgService:
		d.service(b.Param)
		return
	case MsgEnterExit:
		d.enterExit()
		return
	case MsgBack:
		d.post()
		return
	}
	n := d.npc()
	if n == nil {
		return
	}
	param := b.Param
	if d.Kind == KindHouse {
		d.State = param + 1
	} else {
		d.State = param
		if n.Flags == 0 {
			n.Flags = 1
		}
	}
	switch {
	case param >= ParamTopic0 && param < ParamTopic0+6:
		d.topic(n, param, topicEvent(n, param))
	case param == ParamJoinYes:
		d.join(n, true)
	case param == ParamJoinNo:
		d.join(n, false)
	case param == ParamLearn:
		d.h.Note("paid skill lessons (msg 0x4f, M7)")
		d.post()
	case param == ParamAward:
		d.h.Note("paid awards (msg 0x52, M6c)")
	case d.Kind == KindNPC && param >= 0x55 && param <= 0x58:
		d.h.Note("the arena (0x4bc7b5, M12)")
	}
}

// topic runs topic event ev: teachers (300..416), the lost-item quest (0x2c1), the
// arena (0x243, 0x2c0), roster characters offering to join (600..649), else the global
// event with the reply cleared first.
//
// mm8: 0x4b2b74, 0x4bcbf2
func (d *Dialog) topic(n *tables.NPC, param, ev int) {
	switch {
	case ev >= 300 && ev <= 0x1a0:
		d.teacher(ev)
	case ev == 0x2c1:
		d.lostItem()
	case ev == 0x243 || d.Kind == KindNPC && ev == 0x2c0:
		d.h.Note("the arena (0x4bc140, 0x4bc37d, M12)")
	case ev >= 600 && ev <= 0x289:
		d.topicSlot = param
		d.joinOffer(ev)
	default:
		d.Reply = ""
		d.h.RunTopic(ev)
	}
}

// teacher shows a teacher's topic: its text and a "Learn" button when the lesson is
// possible (the price and the check are M7's skills). In a house the text is NPCText(ev),
// in an NPC's own dialogue NPCText(169 + ev).
//
// mm8: 0x4b4c6f (house), 0x4b4f9e (NPC), 0x4b31fe (the price, M7)
func (d *Dialog) teacher(ev int) {
	t := d.h.Tables().Topics.Text
	i := ev
	if d.Kind == KindNPC {
		i = 169 + ev
	}
	if i < len(t) {
		d.Reply = t[i]
	}
	d.State = StateTeacher
	d.h.Note("teacher prices and lessons (0x4b31fe, M7)")
	d.Buttons = []Button{{MsgHouseTopic, ParamLearn}}
	d.Menu = -1
}

// lostItem is the lost-item return (topic 0x2c1): NPCText 851, or 852 and the item when
// the party lost one of the listed quest items (M7).
//
// mm8: 0x4b2ac7
func (d *Dialog) lostItem() {
	d.State = StateLostItem
	if t := d.h.Tables().Topics.Text; len(t) > 851 {
		d.Reply = t[851]
	}
	d.h.Note("returning lost quest items (0x4b2ac7, M7)")
}

// joinOffer is a roster character offering to join: NPCText(198 + 2k) and Yes/No.
//
// mm8: 0x4b4d70
func (d *Dialog) joinOffer(ev int) {
	k := ev - 600
	if t := d.h.Tables().Topics.Text; 198+2*k < len(t) {
		d.Reply = t[198+2*k]
	}
	d.State = StateJoin
	msg := MsgHouseTopic
	if d.Kind == KindNPC {
		msg = MsgNPCTopic
	}
	d.Buttons = []Button{{msg, ParamJoinYes}, {msg, ParamJoinNo}}
	d.Menu = -1
}

// join answers a join offer: Yes adds roster character k (a full party gets
// NPCText(199 + 2k) instead); either way the NPC is gone from now on and quest bit
// ev - 200 (400 + k) is set. A house reloads its portraits.
//
// mm8: 0x4b2b74 (0x77, 0x78), 0x4bcbf2 (0x77, 0x78)
func (d *Dialog) join(n *tables.NPC, yes bool) {
	ev := topicEvent(n, d.topicSlot)
	if ev == 0 {
		return
	}
	m := d.h.Members()
	if yes {
		k := ev - 600
		if m.AddRoster(k) == -2 {
			if t := d.h.Tables().Topics.Text; 199+2*k < len(t) {
				d.Reply = t[199+2*k]
			}
			d.keepReply = true
		}
	}
	n.Flags |= npc.FlagGone
	m.QBits.Set(ev-200, true)
	if yes && d.Kind == KindHouse {
		d.loadResidents()
		if len(d.Portraits) == 1 {
			d.Menu, d.Sel = -1, 1
		} else {
			d.Menu = 1
		}
	}
	d.post()
}

// Back is the back step (Esc, msg 0x71): from a sub-dialogue to the topics, from the
// topics to the portraits; it returns false when the dialogue closes.
//
// mm8: 0x4bda0e (houses), 0x42f877 (msg 0x71 in g_screenMode 4: an NPC's dialogue closes)
func (d *Dialog) Back() bool {
	if d.Kind == KindNPC {
		return false
	}
	d.Input = nil
	if d.keepReply {
		d.keepReply = false
	} else {
		d.Reply = ""
	}
	d.State = 0
	if d.Sel == 0 {
		return false
	}
	switch d.Menu {
	case 0, 1:
		d.Sel, d.Menu = 0, 0
		d.Buttons = d.Buttons[:0]
		return len(d.Portraits) != 1
	case -1:
		d.SelectResident(d.Sel - 1)
	case SvcArcomageRules, SvcArcomageVictory, SvcArcomagePlay:
		d.Menu = SvcArcomage
		d.Buttons = arcomageMenu()
	default:
		// The other sub-menus step back to the main menu (the shops' sell, identify and
		// repair to their display, M7).
		d.Menu = 1
		d.Buttons = ServiceMenu(d.Type, d.h.Members())
	}
	return true
}

// NPCChanged reacts to an event changing NPC id while the dialogue is open:
// SetNPCTopic on the resident being talked to rebuilds the topics; MoveNPC reopens
// house 165 (without portrait buttons) and stops house 553's clip looping.
//
// mm8: 0x4446bd (cases 0x27, 0x28)
func (d *Dialog) NPCChanged(moved bool, id int) {
	if d.Kind != KindHouse {
		return
	}
	if !moved {
		if d.Resident() == id && id != 0 {
			d.Menu = -1
			d.SelectResident(d.Sel - 1)
		}
		return
	}
	switch d.House {
	case 165:
		d.Sel, d.Menu, d.State = 0, 0, 0
		d.Buttons = d.Buttons[:0]
		d.loadResidents()
		d.NoPortraits = true
	case 553:
		d.VideoOnce = true
	}
}

// Greeting is the selected NPC's greeting, shown while nothing else is: for an NPC's
// own dialogue below state 0x13, for a house resident in state 0 of a house without a
// proprietor.
//
// mm8: 0x443441, 0x4b363b
func (d *Dialog) Greeting() string {
	n := d.npc()
	if n == nil {
		return ""
	}
	if d.Kind == KindHouse && (d.Proprietor || d.State != 0) {
		return ""
	}
	return d.h.NPCs().Greeting(n)
}

// Text is the text of the reply box: in an NPC's own dialogue the reply for states
// 0x13..0x18 and 0x77 (and the arena's), else the greeting; in a house the reply.
//
// mm8: 0x443441 (NPC dialogue), 0x4b3dec / 0x4b363b (house)
func (d *Dialog) Text() string {
	if d.Kind == KindNPC {
		s := d.State
		switch {
		case s >= 0x13 && s <= 0x18, s == StateJoin:
			return d.Reply
		case s == 0x59:
			return d.h.Global(0x23e)
		case s == 0x5a:
			return d.h.Global(0x241)
		case s == 0x5c:
			return d.h.Global(0x246)
		}
		if npc.Kind(d.NPC) == npc.KindNPC {
			return d.Greeting()
		}
		return ""
	}
	return d.Reply
}

// Label is the text of a topic column button, worked out at draw time like the
// original: a topic's name (npctopic.txt), Join/Hire/More Information, Yes/No, Learn.
// Viewing a teacher's topic name sets its autonote (id - 172).
//
// mm8: 0x443441 (NPC dialogue labels), 0x4b363b (house resident labels)
func (d *Dialog) Label(b Button) string {
	g := d.h.Global
	if b.Msg == MsgService {
		return "" // the proprietors' screens label their own (ui/house_services.go)
	}
	if b.Msg == MsgBack {
		return g(0x22) // Cancel
	}
	if b.Msg == MsgEnterExit {
		return fmt.Sprintf(g(0x19b), d.h.MapStatsName(d.ExitMap))
	}
	n := d.npc()
	switch b.Param {
	case ParamBio, ParamHire:
		if n != nil && n.Flags&npc.FlagGone != 0 {
			return cfmt(g(0x198), n.Name)
		}
		if b.Param == ParamHire {
			return g(0x196)
		}
		return g(0x7a)
	case ParamMoreInfo:
		return g(0x197)
	case ParamJoinYes:
		return g(0x2c0)
	case ParamJoinNo:
		return g(0x2c1)
	case ParamLearn:
		return "" // g(0x217) "Learn" once the lesson is possible (M7)
	}
	if n == nil || b.Param < ParamTopic0 || b.Param >= ParamTopic0+6 {
		return ""
	}
	ev := topicEvent(n, b.Param)
	if d.Kind == KindHouse && ev >= 300 && ev <= 0x1a0 {
		d.h.SetAutonote(ev - 0xac)
	}
	if d.Kind == KindHouse && ev >= 600 && ev <= 0x289 {
		return g(0x7a) // a roster character's topic reads "Join"
	}
	if t := d.h.Tables().Topics.Topic; ev > 0 && ev < len(t) {
		return t[ev]
	}
	return ""
}

// Name is the dialogue's NPC as shown above its portrait: "Name" or "Name the
// Profession".
//
// mm8: 0x443441, 0x4b363b (globalTxt 0x1ad with the profession names 0x779fe8)
func (d *Dialog) Name() string {
	n := d.npc()
	if n == nil {
		return ""
	}
	if p := int(n.Profession); p != 0 {
		prof := ""
		if p > 0 && p < len(professionText) {
			prof = d.h.Global(professionText[p])
		}
		return cfmt(d.h.Global(0x1ad), n.Name, prof)
	}
	return n.Name
}

// professionText are the global.txt strings of the profession names (0x779fe8, filled
// by Txt_LoadGlobal 0x45181f). No NPC of MM8's npcdata.txt has a profession.
var professionText = [...]int{0x99, 0x134, 0x135, 7, 0x132, 0x136, 0x137, 0x138, 0x139, 0x13a,
	0x69, 0x13b, 0x13c, 0x13d, 0x73, 0x13e, 0x13f, 0x140, 0x141, 0x142, 0x143, 0x125, 0x144,
	0x1f2, 0x20d, 0x147, 0x148, 0x149, 0x14a, 0x14b, 0x14c, 0x14d, 0x14e, 0x14f, 0x150, 0x151,
	0x152, 0x153, 0x154, 0x155, 0x156, 0x157, 0x254, 0x159, 0x15a, 0x15b, 0x15c, 0x15d, 0x15e,
	0x255, 0x160, 0x161, 0x256, 0x158, 0x1a, 599, 0x15, 600, 0x172}

// cfmt is the subset of C printf the game's format strings use (%s, %d, %u, %lu).
func cfmt(f string, args ...any) string {
	f = strings.NewReplacer("%lu", "%d", "%u", "%d", "%i", "%d", "%ld", "%d").Replace(f)
	return fmt.Sprintf(f, args...)
}
