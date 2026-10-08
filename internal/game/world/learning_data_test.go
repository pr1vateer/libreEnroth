package world

import (
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/gfx"
)

// TestLearningTables: the training halls' level caps, the lost quest items (all real
// items) and the one Adventurer's Inn (house 185, clip type 0x23).
//
// mm8: 0x502c4a, 0x502c08, 0x4446bd (case 2)
func TestLearningTables(t *testing.T) {
	e := newEnv(t)
	g := e.tables.Game
	var caps []int
	for h := tables.FirstTrainingHall; h < tables.FirstTrainingHall+tables.NumTrainingHalls; h++ {
		caps = append(caps, g.Learning.LevelCap(h))
	}
	if want := []int{5, 15, 25, 25, 200, 200, 65535, 50, 50, 100, 0, 0, 0}; !slices.Equal(caps, want) {
		t.Errorf("caps %v", caps)
	}
	li := g.Learning.LostItems
	if li[0] != (tables.LostItem{QBit: 199, Item: 539}) || li[24] != (tables.LostItem{QBit: 224, Item: 662}) {
		t.Errorf("lost items %v .. %v", li[0], li[24])
	}
	for _, l := range li {
		if g.Items.Item(l.Item).Name == "" {
			t.Errorf("lost item %d has no name", l.Item)
		}
	}
	var inns []int
	for h := 1; h < len(g.Houses); h++ {
		if dialog.IsInn(g, h) {
			inns = append(inns, h)
		}
	}
	if !slices.Equal(inns, []int{185}) {
		t.Errorf("inns %v", inns)
	}
}

// TestTeacherSweep: every teacher of npcdata.txt teaches in its house: its topic shows
// its text and a Learn label, and none falls in the topics Teacher_SkillOfTopic misses.
//
// mm8: 0x4b2b74, 0x4b4c6f, 0x4b31fe
func TestTeacherSweep(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "out01.odm")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	w.S.Party.Calendar.Hour = 12
	n := 0
	for id, np := range e.tables.Game.NPC.NPCs {
		for _, ev := range np.Topics {
			if ev < 300 || ev > 0x1a0 || np.House == 0 {
				continue
			}
			w.CloseDialog()
			w.S.noted = nil
			w.SpeakInHouse(int(np.House))
			d := w.Dialog()
			if d == nil {
				t.Errorf("npc %d: house %d did not open", id, np.House)
				continue
			}
			k := slices.IndexFunc(d.Portraits, func(p dialog.Portrait) bool { return p.NPC == id })
			if k < 0 {
				t.Errorf("npc %d not in house %d", id, np.House)
				continue
			}
			d.SelectResident(k)
			i := slices.IndexFunc(d.Buttons, func(b dialog.Button) bool {
				return b.Param >= dialog.ParamTopic0 && np.Topics[b.Param-dialog.ParamTopic0] == ev
			})
			if i < 0 {
				t.Errorf("npc %d: topic %d not offered", id, ev)
				continue
			}
			d.Click(d.Buttons[i])
			if d.State != dialog.StateTeacher || d.Reply == "" || d.Label(d.Buttons[0]) == "" {
				t.Errorf("npc %d topic %d: state %#x reply %q label %q", id, ev, d.State, d.Reply, d.Label(d.Buttons[0]))
			}
			if len(w.S.noted) != 0 {
				t.Errorf("npc %d topic %d: %v", id, ev, w.S.noted)
			}
			n++
		}
	}
	if n != 105 {
		t.Errorf("%d teachers", n)
	}
}

// TestInnHireDismiss: at the Adventurer's Inn a roster character picked and hired joins
// the party and leaves the list; the character screen's dismiss button (clicked twice)
// sends it back, and the inn lists it again.
//
// mm8: 0x42f877 (msgs 0x1c4, 0x1c6, 0x1c7), 0x4cab24
func TestInnHireDismiss(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "out01.odm")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	m := w.S.Party
	for id := 1; id <= 3; id++ {
		m.QBits.Set(400+id, true)
	}
	click := func(x, y int) {
		a.Update(&ui.Input{X: x, Y: y})
		a.Update(&ui.Input{X: x, Y: y, Left: true, LeftPressed: true})
		a.Update(&ui.Input{X: x, Y: y, LeftReleased: true})
		a.Draw(gfx.NewCanvas())
	}
	w.SpeakInHouse(185)
	if w.Dialog() != nil || w.OpenedInn() != 185 {
		t.Fatalf("inn: dialog %v, inn %d", w.Dialog(), w.OpenedInn())
	}
	a.Update(&ui.Input{X: 300, Y: 300})
	a.Draw(gfx.NewCanvas())
	click(120, 80)      // the second face: roster character 2
	click(0x210, 0x184) // Hire
	if len(m.Players) != 2 || m.Players[1].RosterID != 2 || m.Selected != 2 {
		t.Fatalf("hire: %d members, selected %d", len(m.Players), m.Selected)
	}
	a.Update(&ui.Input{X: 300, Y: 300, Keys: []ui.Key{ui.KeyEscape}})
	if w.OpenedInn() != 0 {
		t.Fatal("Esc left the inn open")
	}
	// The character screen on member 2 (a second click on its portrait), Dismiss twice.
	click(118+30, 389+40)
	click(0x208+20, 0x1a4+10)
	if len(m.Players) != 2 {
		t.Fatal("the first click dismissed")
	}
	click(0x208+20, 0x1a4+10)
	if len(m.Players) != 1 || m.Selected != 1 || !m.QBits.Get(402) {
		t.Fatalf("dismiss: %d members, selected %d", len(m.Players), m.Selected)
	}
}

// TestOracleLostItem: the Oracle's (NPC 502, house 187) lost-item topic answers
// NPCText 851 until quest bit 199 is set; then it gives item 539 back (NPCText 852 with
// its name), and only while nobody has it.
//
// mm8: 0x4b2ac7 (NPC_LostItemTopic)
func TestOracleLostItem(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "out01.odm")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	m := w.S.Party
	m.Calendar.Hour = 12
	ask := func() *dialog.Dialog {
		w.CloseDialog()
		w.SpeakInHouse(187)
		d := w.Dialog()
		if d == nil {
			t.Fatal("house 187 closed")
		}
		k := slices.IndexFunc(d.Portraits, func(p dialog.Portrait) bool { return p.NPC == 502 })
		d.SelectResident(k)
		i := slices.IndexFunc(d.Buttons, func(b dialog.Button) bool {
			return b.Param >= dialog.ParamTopic0 && e.tables.Game.NPC.NPCs[502].Topics[b.Param-dialog.ParamTopic0] == 0x2c1
		})
		if i < 0 {
			t.Fatal("no lost-item topic")
		}
		d.Click(d.Buttons[i])
		return d
	}
	text := e.tables.Game.Topics.Text
	if d := ask(); d.State != dialog.StateLostItem || d.Reply != text[851] || m.MouseItem.Number != 0 {
		t.Errorf("nothing lost: %q", d.Reply)
	}
	m.QBits.Set(199, true)
	d := ask()
	if m.MouseItem.Number != 539 || d.Reply == text[851] {
		t.Fatalf("lost item: mouse %d, %q", m.MouseItem.Number, d.Reply)
	}
	if d = ask(); m.MouseItem.Number != 539 || d.Reply != text[851] {
		t.Errorf("given twice: %q", d.Reply)
	}
}
