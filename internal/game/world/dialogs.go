package world

import (
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/npc"
)

// The world is the host of the house and NPC dialogues its events open (dialog.Host),
// and of the event message box (PressAnyKey, InputString).

var _ dialog.Host = (*World)(nil)

// message is an event waiting in the message box.
type message struct {
	susp evt.Suspension
	text string
}

// npcs is the session's NPC table, made from the templates on first use (a new game:
// Party_InitNewGame 0x49228d copies it); the roster comes with it.
func (w *World) npcs() *npc.State {
	s := w.S
	if s.NPCs == nil && w.tables != nil && w.tables.Game != nil {
		s.NPCs = npc.New(w.tables.Game.NPC)
	}
	return s.NPCs
}

// NPCs implements evt.Host and dialog.Host.
func (w *World) NPCs() *npc.State { return w.npcs() }

// Tables implements dialog.Host.
func (w *World) Tables() *tables.All { return w.tables.Game }

// MapStatsName implements dialog.Host: MapStats.txt row i's name.
func (w *World) MapStatsName(i int) string {
	if row := w.statsRow(i); len(row) > 1 {
		return row[1]
	}
	return ""
}

// MapStatsFile implements dialog.Host: MapStats.txt row i's map file.
func (w *World) MapStatsFile(i int) string {
	if row := w.statsRow(i); len(row) > 2 {
		return strings.TrimSpace(row[2])
	}
	return ""
}

// MapStatsIndex implements dialog.Host (MapStats_Find).
func (w *World) MapStatsIndex(name string) int { return w.tables.MapIndex(name) }

// RunTopic implements dialog.Host: a dialogue's topic runs its global.evt event with
// source 1.
//
// mm8: 0x4b2b74 / 0x4bcbf2 (g_evtSource = 1; reply = 0; Evt_Process(id, 0, 1))
func (w *World) RunTopic(id int) {
	w.ReplyText = ""
	w.vm.Run(evt.Source{Kind: evt.SourceGlobal}, id, 0, true)
}

// CanShowTopic implements dialog.Host.
func (w *World) CanShowTopic(id int) int { return w.vm.CanShowTopic(id) }

// Speak implements dialog.Host.
func (w *World) Speak(member, id int) { w.S.Party.Speak(member, id, w.S.Ctx) }

// SetAutonote implements dialog.Host (Evt_Set(member 0, 0xe1, n)).
func (w *World) SetAutonote(n int) {
	if len(w.S.Party.Players) > 0 {
		w.vm.Set(0, evt.VarAutonotes, uint32(n))
	}
}

// Note implements dialog.Host: logged once each.
func (w *World) Note(what string) {
	if w.S.noted == nil {
		w.S.noted = map[string]bool{}
	}
	if !w.S.noted[what] {
		w.S.noted[what] = true
		w.S.logf("dialog: %s", what)
	}
}

// NPCText implements evt.Host: npctext.txt string i.
func (w *World) NPCText(i int) string {
	if t := w.tables.Game; t != nil && i > 0 && i < len(t.Topics.Text) {
		return t.Topics.Text[i]
	}
	return ""
}

// QuestText implements evt.Host: quest bit n has a quest log text.
func (w *World) QuestText(n int) bool {
	t := w.tables.Game
	return t != nil && n > 0 && n < len(t.Quests) && t.Quests[n] != ""
}

// AwardText implements evt.Host: award n has a text.
func (w *World) AwardText(n int) bool {
	t := w.tables.Game
	return t != nil && n > 0 && n < len(t.Awards) && t.Awards[n].Text != ""
}

// AutonoteText implements evt.Host: autonote n has a text.
func (w *World) AutonoteText(n int) bool {
	t := w.tables.Game
	return t != nil && n > 0 && n < len(t.Autonotes) && t.Autonotes[n].Text != ""
}

// Reply implements evt.Host: the dialogue's reply text (0xffd350).
func (w *World) Reply(text string) {
	w.ReplyText = text
	if w.dialog != nil {
		w.dialog.Reply = text
	}
}

// NPCChanged implements evt.Host.
func (w *World) NPCChanged(op evt.Op, id int) {
	if w.dialog != nil {
		w.dialog.NPCChanged(op == evt.OpMoveNPC, id)
	}
}

// SpeakInHouse implements evt.Host: the house dialogue opens at once (the rest of the
// event runs with it open), unless the house is closed.
//
// mm8: 0x4446bd (case 2), 0x443f4b (House_Enter)
func (w *World) SpeakInHouse(house int) {
	if d := dialog.OpenHouse(w, house); d != nil {
		w.dialog = d
	}
}

// SpeakNPC implements evt.Host.
//
// mm8: 0x4446bd (case 0x16), 0x443b6f (Evt_SpeakNPC)
func (w *World) SpeakNPC(id int, canShow bool) {
	if !canShow {
		w.S.deferredNPC = id
		return
	}
	if d := dialog.OpenNPC(w, id); d != nil {
		w.dialog = d
	}
}

// Suspend implements evt.Host: the message box shows the map's message (or the reply)
// for PressAnyKey, the question for InputString. A box already up keeps its event.
//
// mm8: 0x44328b (Evt_Suspend: nothing when the message window 0x519298 exists),
// 0x442dc0 (the box's text: g_evtMessage, else the reply; g_statusTimed while typing)
func (w *World) Suspend(s evt.Suspension) {
	if w.message != nil {
		return
	}
	m := &message{susp: s, text: w.MessageText}
	if m.text == "" {
		m.text = w.ReplyText
	}
	if s.Op == evt.OpInputString {
		m.text = s.Question
	}
	w.message = m
}

// Dialog is the open dialogue, nil when none. An NPC deferred by OnMapReload opens now.
//
// mm8: 0x42f877 (the message loop opens g_evtDeferredNpc first)
func (w *World) Dialog() *dialog.Dialog {
	if w.dialog == nil && w.S.deferredNPC != 0 {
		id := w.S.deferredNPC
		w.S.deferredNPC = 0
		w.dialog = dialog.OpenNPC(w, id)
	}
	return w.dialog
}

// CloseDialog ends the open dialogue: an NPC's, or the house is left.
//
// mm8: 0x443d74 (NPC dialogue), 0x42f877 (LAB_004309af: leaving a house)
func (w *World) CloseDialog() {
	if w.dialog == nil {
		return
	}
	w.dialog = nil
	m := w.S.Party
	if m.Selected < 1 || m.Selected > len(m.Players) {
		m.Selected = m.NextSelectable()
	}
}

// MessageBox is the message box an event waits in: its text and whether it asks for an
// answer (InputString).
func (w *World) MessageBox() (text string, input, ok bool) {
	if w.message == nil {
		return "", false, false
	}
	return w.message.text, w.message.susp.Op == evt.OpInputString, true
}

// Answer closes the message box and resumes its event with the typed answer (ok false
// is Esc, which empties it).
//
// mm8: 0x442dc0 (Enter: the answer into g_statusTimed, msg 0x1ef 1; Esc: emptied,
// msg 0x1ef 0), 0x42f877 (msg 0x1ef: Evt_Resume), 0x4433b0 (PressAnyKey: Evt_ResumeUnpause)
func (w *World) Answer(text string, ok bool) {
	m := w.message
	if m == nil {
		return
	}
	w.message = nil
	if !ok {
		text = ""
	}
	if m.susp.Op == evt.OpPressAnyKey {
		w.MessageText = ""
	}
	w.vm.Resume(m.susp, text)
}

// interactiveName is the hover text of an interactive decoration: the topic name of its
// event.
//
// mm8: 0x420aab (Mouse_UpdateHover: 0x75e348[Dec_DecodeEvent(mapVar)] )
func (w *World) interactiveName(event int) string {
	t := w.tables.Game
	if t == nil || event <= 0 || event >= len(t.Topics.Topic) {
		return ""
	}
	return strings.TrimSpace(t.Topics.Topic[event])
}
