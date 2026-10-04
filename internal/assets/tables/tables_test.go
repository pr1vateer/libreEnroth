package tables

import "testing"

func TestParseHousesSynthetic(t *testing.T) {
	// Two header lines, then: an Inn (no prefix match), a Spell Shop ("spe" -> 0xe), a
	// row with an empty Picture cell (skipped, stays 0).
	data := "h1\r\n#\t#\tType\r\n" +
		"1\t1\tThe Adventurer's Inn\t1\t96\t\"Inn, Nice\"\tOwn\tTitle\t2109\t0\t0\t0\t1.5\t2\t\t7\tn\tn\t6\t18\t2\t32\t-1\t\"Enter\"\r\n" +
		"2\t2\tSpell Shop\t1\t\t" + "Name\r\n"
	h := ParseHouses([]byte(data))
	if len(h) != NumHouses+1 {
		t.Fatalf("%d rows", len(h))
	}
	a := h[1]
	if a.Type != 0 || a.Video != 96 || a.Name != "Inn, Nice" || a.Picture != 2109 || a.Val != 1.5 || a.A != 2 ||
		a.C != 7 || a.Open != 6 || a.Closed != 18 || a.ExitPic != 2 || a.ExitMap != 32 || a.QBit != -1 || a.EnterText != "Enter" {
		t.Errorf("row 1 = %+v", a)
	}
	if b := h[2]; b.Type != 0xe || b.Video != 0 || b.Name != "Name" {
		t.Errorf("row 2 = %+v", b)
	}
}

func TestColumn1StopsAtEmpty(t *testing.T) {
	// quests.txt rows like "1\t\tNotes" have no text: the empty cell ends the row.
	q := ParseQuests([]byte("Q Bit\tText\r\n1\t\tnote\r\n2\t\"Quest two\"\tnote\r\n"))
	if q[1] != "" || q[2] != "Quest two" {
		t.Errorf("quests = %q", q[:3])
	}
}

func TestParseNPCs(t *testing.T) {
	npc := "NPC Data\r\n#\tName\r\n1\tBrekish\t833\t0\t0\t0\t173\t0\t3\tyes\t7\t8\t9\t0\t0\t11\t\"note\"\r\n"
	greet := "#\tG1\tG2\r\n1\t\"Hi\"\tAgain\r\n"
	group := "G\tN\r\n0\t0\r\n1\t12\r\n"
	news := "N\tT\r\n0\tzero\r\n1\t\"one\"\r\n"
	n := ParseNPCs([]byte(npc), []byte(greet), []byte(group), []byte(news))
	b := n.NPCs[1]
	if b.Name != "Brekish" || b.Portrait != 833 || b.House != 173 || b.Greeting != 3 || b.Join != 1 ||
		b.Topics != [6]int32{7, 8, 9, 0, 0, 11} {
		t.Errorf("npc 1 = %+v", b)
	}
	if n.Greetings[1] != [2]string{"Hi", "Again"} || n.GroupNews[1] != 12 || n.News[1] != "one" {
		t.Errorf("greet %q group %v news %q", n.Greetings[1], n.GroupNews[:2], n.News[:2])
	}
}

func TestClassByName(t *testing.T) {
	for s, want := range map[string]int{"necromancer": 0, "Lich": 1, "priestofsun": 3, "dragon2": 15, "darkelf": 10, "bogus": 0} {
		if got := ClassByName(s); got != want {
			t.Errorf("ClassByName(%q) = %d, want %d", s, got, want)
		}
	}
}
