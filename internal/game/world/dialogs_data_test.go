package world

import (
	"slices"
	"testing"

	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/ui"
)

// TestTopicSweep: every topic event of every NPC goes through CanShowTopic and runs as a
// dialogue topic (source 1) from that NPC's own dialogue, without a panic or a loop; the
// number of distinct topic events and how many have visibility records are frozen.
//
// mm8: 0x4444fe (Evt_CanShowTopic), 0x4b2b74 (Evt_Process with source 1)
func TestTopicSweep(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "out01.odm", quietSession())
	seen := map[int32]bool{}
	gated := 0
	runGuarded(t, "topics", func() {
		for id := 1; id < len(w.npcs().NPCs); id++ {
			n := w.npcs().Get(id)
			for _, ev := range n.Topics {
				if ev == 0 || seen[ev] {
					continue
				}
				seen[ev] = true
				if w.CanShowTopic(int(ev)) != 2 {
					gated++
				}
				w.dialog = dialog.OpenNPC(w, id)
				if w.dialog != nil {
					w.dialog.Click(dialog.Button{Msg: dialog.MsgNPCTopic, Param: dialog.ParamTopic0})
				}
				w.RunTopic(int(ev))
				w.message, w.dialog = nil, nil
			}
		}
	})
	if len(seen) != 502 || gated != 60 {
		t.Errorf("%d topic events, %d with visibility records", len(seen), gated)
	}
}

// TestOut01SpeakNPC: out01's timer event 500 at 10:00 has S'ton (NPC 31) speak once:
// the dialogue opens and quest bit 232 keeps it from opening again.
//
// mm8: 0x4446bd (case 0x16), 0x443b6f (Evt_SpeakNPC)
func TestOut01SpeakNPC(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.SetTimeOfDay(9, 59)
	w := e.load(t, "out01.odm", s)
	if w.Dialog() != nil {
		t.Fatal("a dialogue before 10:00")
	}
	for range 60 * 5 {
		w.Update(&ui.Input{})
		if w.Dialog() != nil {
			break
		}
	}
	d := w.Dialog()
	if d == nil || d.Kind != dialog.KindNPC || d.NPC != 31 || !s.Party.QBits.Get(232) {
		t.Fatalf("dialogue %+v, qbit 232 %v", d, s.Party.QBits.Get(232))
	}
	if d.Name() != "S'ton" || d.Text() == "" || len(d.Buttons) != 3 {
		t.Errorf("S'ton: name %q, text %q, %d topics", d.Name(), d.Text(), len(d.Buttons))
	}
	w.CloseDialog()
	w.vm.Run(evt.Source{Kind: evt.SourceMap}, 500, 1, true)
	if w.Dialog() != nil {
		t.Error("S'ton speaks again after quest bit 232")
	}
}

// TestHouseThistle: out01's House of Thistle (230): two residents; Thistle's
// "Ingredients" topic replies through ShowMessage.
func TestHouseThistle(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "out01.odm", quietSession())
	w.SpeakInHouse(230)
	d := w.Dialog()
	if d == nil || d.Def.Name != "House of Thistle" || !slices.Equal(d.Residents, []int{88, 315}) || d.Anim.Video != "lporhs1" {
		t.Fatalf("house 230: %+v", d)
	}
	d.SelectResident(0)
	var labels []string
	for _, b := range d.Buttons {
		labels = append(labels, d.Label(b))
	}
	if !slices.Equal(labels, []string{"Potion of Pure Speed", "Ingredients"}) {
		t.Fatalf("Thistle's topics %q", labels)
	}
	d.Click(d.Buttons[1])
	if d.Reply == "" || d.Reply != w.ReplyText {
		t.Errorf("reply %q / %q", d.Reply, w.ReplyText)
	}
	if !d.Back() || d.Back() {
		t.Error("two back steps leave the house")
	}
	// Closed at night: house 1 (a weapon shop) opens 6..18.
	s := w.S
	s.SetTimeOfDay(22, 0)
	w.syncClock()
	w.CloseDialog()
	w.SpeakInHouse(1)
	if w.Dialog() != nil {
		t.Error("the weapon shop is open at 22:00")
	}
	_ = clock.Hour
}

// TestRiddle: Escaton's riddle (global event 164, InputString) waits in the message box
// with its question; a wrong answer goes on to the "wrong" branch.
//
// mm8: 0x4446bd (case 0x1a), 0x44328b, 0x442dc0
func TestRiddle(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "pbp.odm", quietSession())
	w.npcs().SetTopic(26, 0, 164)
	w.SpeakInHouse(184)
	d := w.Dialog()
	if d == nil || d.Sel != 1 || d.Resident() != 26 {
		t.Fatalf("house 184: %+v", d)
	}
	d.Click(d.Buttons[0])
	text, input, ok := w.MessageBox()
	if !ok || !input || text != w.NPCText(595) || text == "" {
		t.Fatalf("message box %q %v %v", text, input, ok)
	}
	w.Answer("wrong", true)
	if _, _, ok := w.MessageBox(); ok || w.dialog.Reply != w.NPCText(597) {
		t.Errorf("after a wrong answer: reply %q", w.dialog.Reply)
	}
}
