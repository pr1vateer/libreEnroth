package tables

import (
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
)

func load(t *testing.T) (*All, *assets.Data) {
	t.Helper()
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	a, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	return a, d
}

func TestLoadAll(t *testing.T) {
	a, d := load(t)
	count := func(n int, ok func(i int) bool) int {
		c := 0
		for i := 0; i < n; i++ {
			if ok(i) {
				c++
			}
		}
		return c
	}
	named := count(len(a.Houses), func(i int) bool { return a.Houses[i].Name != "" })
	npcs := count(len(a.NPC.NPCs), func(i int) bool { return a.NPC.NPCs[i].Name != "" })
	greets := count(len(a.NPC.Greetings), func(i int) bool { return a.NPC.Greetings[i][0] != "" })
	topics := count(len(a.Topics.Topic), func(i int) bool { return a.Topics.Topic[i] != "" })
	texts := count(len(a.Topics.Text), func(i int) bool { return a.Topics.Text[i] != "" })
	quests := count(len(a.Quests), func(i int) bool { return a.Quests[i] != "" })
	notes := count(len(a.Autonotes), func(i int) bool { return a.Autonotes[i].Text != "" })
	awards := count(len(a.Awards), func(i int) bool { return a.Awards[i].Text != "" })
	trans := count(len(a.Trans), func(i int) bool { return a.Trans[i] != "" })
	got := [...]int{len(a.Houses), named, len(a.NPC.NPCs), npcs, greets, topics, texts, quests, notes, awards, trans}
	want := [...]int{525, 524, 551, 550, 115, 741, 899, 247, 256, 73, 119}
	if got != want {
		t.Errorf("rows/named %v, want %v", got, want)
	}
	h := a.Houses[1]
	if h.Type != 1 || h.Video != 6 || h.Name != "True Mettle" || h.Owner != "Kervin" || h.Title != "Blacksmith" ||
		h.Val != 1.5 || h.A != 1 || h.Open != 6 || h.Closed != 18 || h.C != 7 {
		t.Errorf("house 1 = %+v", h)
	}
	if n := a.NPC.NPCs[31]; n.Name == "" || n.Portrait == 0 {
		t.Errorf("npc 31 = %+v", n)
	}
	if an := a.Anim(6); an.Video != "lwpshp" || an.Portrait != 0x83d || an.Type != 1 || an.RoomSound != 2 {
		t.Errorf("anim 6 = %+v", an)
	}
	if an := a.Anim(500); an.Video != "lmedhs1" || an.Portrait != 0 {
		t.Errorf("anim out of range = %+v", an)
	}
	for pic, want := range map[int]string{1: "ticon01", 2: "ticon02", 3: "ticon03"} {
		if got, err := ExitIcon(d.Exe, pic); err != nil || got != want {
			t.Errorf("ExitIcon(%d) = %q, %v", pic, got, err)
		}
	}
	if r := a.Roster[1]; r.Name != "Devlin Arcanus" || r.Class != 0 || r.Face != 8 || r.Voice != 8 || r.Level != 5 {
		t.Errorf("roster 1 = %+v", r)
	}
	if r := a.Roster[17]; r.Class != 1 || r.Birth != 0x484 {
		t.Errorf("roster 17 = %+v", r)
	}
	if a.Merchant.Buy[0] == "" || a.Merchant.Sell[0] != "n/a" {
		t.Errorf("merchant = %+v", a.Merchant)
	}
}
