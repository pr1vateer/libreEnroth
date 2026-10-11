package dialog

import (
	"fmt"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// bountyHost: NPC 10 in house 5 offers the bounty topic; monsters 1..0x100 have level
// id % 50 + 1; house 102 is a town hall.
func bountyHost() *fakeHost {
	h := newHost()
	h.npcs.SetTopic(10, 0, EventBounty)
	mt := &tables.Monsters{Infos: make([]tables.MonsterInfo, 0x100)}
	for id := range mt.Infos {
		mt.Infos[id] = tables.MonsterInfo{Name: fmt.Sprint("m", id), Level: uint8(id%50 + 1), ID: uint16(id)}
	}
	h.t.Monsters = mt
	text := h.t.Topics.Text
	text[textBounty], text[textBountyPaid] = "on %s for %lu", "paid %s %lu"
	text[textBountyTaken], text[textBountyKills] = "taken", "kills"
	h.t.Houses[102] = tables.House{Name: "Hall", Type: TypeTownHall, Video: 120, Owner: "O", Title: "T"}
	h.t.HouseAnims[120] = tables.HouseAnim{Video: "v", Portrait: 1, Type: TypeTownHall}
	h.m.Time = clock.NewGame
	h.m.Calendar = h.m.Time.Calendar()
	h.m.Players = append(h.m.Players, party.Player{RosterID: 1})
	return h
}

func colour(name string) string { return "\f65523" + name + "\f65535" }

// The bounty topic: a new month draws the monster (rand() % 0xc3 + 1, again while 1..3)
// and runs to the next month's start; a kill pays level * 100, the award for everyone
// and the bounty total, after which the month's bounty is taken; the next month brings
// a new one.
//
// mm8: 0x4bc140
func TestBountyTopic(t *testing.T) {
	h := bountyHost()
	d := OpenHouse(h, 5)
	d.SelectResident(0)
	topic := Button{MsgHouseTopic, ParamTopic0}
	ref := party.NewRand(1)
	mon := ref.Int()%0xc3 + 1
	for mon <= 3 {
		mon = ref.Int()%0xc3 + 1
	}
	d.Click(topic)
	hunt := &h.m.Hunts[0]
	lvl := mon%50 + 1
	if hunt.Monster != uint16(mon) || hunt.Refresh != clock.Month || d.State != StateBounty || d.Menu != -1 ||
		len(d.Buttons) != 1 || d.Buttons[0].Msg != 0 {
		t.Fatalf("hunt %+v state %#x buttons %v", hunt, d.State, d.Buttons)
	}
	if want := fmt.Sprintf("on %s for %d", colour(fmt.Sprint("m", mon)), lvl*100); d.Reply != want {
		t.Errorf("reply %q, want %q", d.Reply, want)
	}
	d.Click(d.Buttons[0]) // posts nothing
	if d.State != StateBounty {
		t.Error("the topic button acted")
	}
	hunt.Killed = 1
	gold := h.m.Gold
	d.Click(topic)
	if h.m.Gold != gold+int32(lvl*100) || h.m.Bounty != int32(lvl*100) || hunt.Monster != 0 || hunt.Killed != 0 ||
		len(h.awards) != 2 || h.awards[1] != [2]int{1, awardBounty} {
		t.Errorf("paid: gold %d bounty %d hunt %+v awards %v", h.m.Gold, h.m.Bounty, hunt, h.awards)
	}
	if want := fmt.Sprintf("paid %s %d", colour(fmt.Sprint("m", mon)), lvl*100); d.Reply != want {
		t.Errorf("reply %q, want %q", d.Reply, want)
	}
	d.Click(topic)
	if d.Reply != "taken" {
		t.Errorf("claimed: %q", d.Reply)
	}
	hunt.Kills = 3
	h.m.Time = clock.Month + 1
	h.m.Calendar = h.m.Time.Calendar()
	d.Click(topic)
	if hunt.Monster == 0 || hunt.Refresh != 20643841 /* ftol(2 * 0x12750000 * (float)1/30) */ || d.Reply != "kills" {
		t.Errorf("next month: %+v %q", hunt, d.Reply)
	}
}

// The town hall's Bounty Hunt keeps its own hunt (house 102: slot 0): any monster
// (rand() % count + 1) after a kill count (rand() % 10 + 1).
//
// mm8: 0x4bd028 (code 99)
func TestTownHallBounty(t *testing.T) {
	h := bountyHost()
	d := OpenHouse(h, 102)
	ref := party.NewRand(1)
	kills := ref.Int()%10 + 1
	mon := ref.Int()%0x100 + 1
	clickService(d, SvcBounty)
	hunt := &h.m.Hunts[0]
	if d.Menu != SvcBounty || int(hunt.Kills) != kills || int(hunt.Monster) != mon {
		t.Fatalf("menu %d hunt %+v, want %d kills of %d", d.Menu, hunt, kills, mon)
	}
}

// nextMonth is the start of the next month: (year * 12 + month - 0x36ef) months of
// 0x12750000 / 30 ticks.
func TestNextMonth(t *testing.T) {
	for _, c := range []struct {
		t    clock.Time
		want clock.Time
	}{{clock.NewGame, clock.Month}, {clock.Year + 5*clock.Month + 3*clock.Day, 18 * clock.Month}} {
		if got := nextMonth(c.t.Calendar()); got-c.want > 10 || c.want-got > 0 {
			t.Errorf("nextMonth(%d) = %d, want about %d", c.t, got, c.want)
		}
	}
}
