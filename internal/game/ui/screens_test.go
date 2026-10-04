package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

var update = flag.Bool("update", false, "write the rendered screens to <module>/out/ui_*.png and log their hashes")

func newTestApp(t *testing.T, start State) *App {
	t.Helper()
	return newTestAppParty(t, start, nil)
}

// newTestAppParty starts with a party of these faces (nil: the default one member).
func newTestAppParty(t *testing.T, start State, faces []int) *App {
	t.Helper()
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	r := NewResources(d)
	if faces != nil {
		r.Party.Players = nil
		for _, f := range faces {
			r.Party.Players = append(r.Party.Players, party.Player{Face: f, Voice: f, Class: ClassForFace(f)})
		}
	}
	a, err := NewApp(r, start)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// run feeds inputs and fails on errors; it returns whether the app asked to quit.
func run(t *testing.T, a *App, ins ...Input) bool {
	t.Helper()
	for _, in := range ins {
		quit, err := a.Update(&in)
		if err != nil {
			t.Fatal(err)
		}
		if quit {
			return true
		}
	}
	return false
}

func repeat(in Input, n int) []Input {
	out := make([]Input, n)
	for i := range out {
		out[i] = in
	}
	return out
}

// Frozen SHA-256s of the 640x480 RGBA canvas (no game-derived image is committed).
// Regenerate with: MM8_DATA=../games_mm8 go test ./internal/game/ui -run Screens -update
var screenHashes = map[string]string{
	"title_idle":         "a5f814033eaf54f6e44dd4cf5e3d7157113b9c3f79c7b45d2bed723fb795213c",
	"title_hover_new":    "61a8085d4adea3a7d95a2d95ebf70942b620b6a10dc7fb000d653722f0917125",
	"title_press_quit":   "9131d31a3d51d466841a17e1f0c9ef3721d0ce5d92723594691fbf957c848a8b",
	"title_load_toast":   "1c542d892afa13954666927ca1812eefca652b79f22a1e4baca59c5d9cba1187",
	"credits_300":        "4526fd63c8da5984c4dd53f03e7b29a945c51863e27bd09ae47643b583cf9ebd",
	"create_idle":        "6ae54a44656893fa797058299aea2787b59ea1de72d04c9c36d1713e664f8938",
	"create_name_face5":  "b82d890ff1d02aae347237339f753835f313cc7c762ac0911a9d265fd5a61a15",
	"create_troll":       "59d6f2d6338bc274f785bf668fe0829cf6fee1c1646953426ea9fa157be93458",
	"ingame":             "78a921cbfba3384176072faccd22d343db0faf930fcbcfd26490b7f268539b77",
	"ingame_party5_sel3": "7f85abd6c6baef83429079e43b40a3641ed772104a4ccd9d49adfc4e2940b777",
	// rest_test.go
	"rest_open":  "994896d6b406f9ab0509ef642000266cf883b89142a9a2fe1bbef07c4dec2236",
	"hover_time": "21c5f4b8a19efe3c3ff10ed0203c6cfc0f90dbf36e48d1a5eb4ee6084f61816a",
}

func TestScreens(t *testing.T) {
	type scene struct {
		name  string
		start State
		in    []Input
		state State
		party []int
	}
	typed := []Input{{X: 300, Y: 20, Runes: []rune("Zoltan")}}
	scenes := []scene{
		{"title_idle", StateTitle, []Input{{X: 100, Y: 100}}, StateTitle, nil},
		{"title_hover_new", StateTitle, []Input{{X: 560, Y: 215}}, StateTitle, nil},
		{"title_press_quit", StateTitle, []Input{{X: 560, Y: 330}, {X: 560, Y: 330, Left: true, LeftPressed: true}}, StateTitle, nil},
		{"title_load_toast", StateTitle, []Input{{X: 560, Y: 255, Left: true, LeftPressed: true}, {X: 560, Y: 255, LeftReleased: true}}, StateTitle, nil},
		{"credits_300", StateCredits, repeat(Input{X: 700, Y: 0}, 300), StateCredits, nil},
		{"create_idle", StateCreate, []Input{{X: 600, Y: 20}}, StateCreate, nil},
		{"create_name_face5", StateCreate, append(typed,
			Input{X: 170, Y: 170, Left: true, LeftPressed: true}, Input{X: 170, Y: 170, LeftReleased: true},
			Input{X: 170, Y: 170, Left: true, LeftPressed: true}, Input{X: 170, Y: 170, LeftReleased: true},
			Input{X: 170, Y: 170, Left: true, LeftPressed: true}, Input{X: 170, Y: 170, LeftReleased: true},
			Input{X: 170, Y: 170, Left: true, LeftPressed: true}, Input{X: 170, Y: 170, LeftReleased: true},
			Input{X: 170, Y: 170, Left: true, LeftPressed: true}, Input{X: 170, Y: 170, LeftReleased: true},
		), StateCreate, nil},
		{"create_troll", StateCreate, []Input{
			{X: 75, Y: 170, Left: true, LeftPressed: true}, {X: 75, Y: 170, LeftReleased: true},
			{X: 75, Y: 170, Left: true, LeftPressed: true}, {X: 75, Y: 170, LeftReleased: true},
		}, StateCreate, nil},
		{"ingame", StateInGame, []Input{{X: 320, Y: 200}}, StateInGame, nil},
		// Five members, '3' selects the third; after 440 ticks it shows an idle face.
		{"ingame_party5_sel3", StateInGame, append([]Input{{X: 320, Y: 200, Keys: []Key{'3'}}}, repeat(Input{X: 320, Y: 200}, 439)...), StateInGame, []int{0, 5, 12, 17, 22}},
	}
	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			a := newTestAppParty(t, sc.start, sc.party)
			if run(t, a, sc.in...) {
				t.Fatal("quit")
			}
			if a.State() != sc.state {
				t.Fatalf("state %v, want %v", a.State(), sc.state)
			}
			c := gfx.NewCanvas()
			a.Draw(c)
			sum := sha256.Sum256(c.Img.Pix)
			got := hex.EncodeToString(sum[:])
			if *update {
				writePNG(t, c, sc.name)
				t.Logf("%q: %q,", sc.name, got)
				return
			}
			if want, ok := screenHashes[sc.name]; !ok || got != want {
				t.Errorf("canvas sha256 %s, want %s (inspect with -update)", got, want)
			}
		})
	}
}

func writePNG(t *testing.T, c *gfx.Canvas, name string) {
	dir := filepath.Join("..", "..", "..", "out")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "ui_"+name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, c.Img); err != nil {
		t.Fatal(err)
	}
}

// The state machine end to end: title -> create -> in-game -> title -> credits -> title -> quit.
func TestFlow(t *testing.T) {
	a := newTestApp(t, StateTitle)
	click := func(x, y int) []Input {
		return []Input{{X: x, Y: y}, {X: x, Y: y, Left: true, LeftPressed: true}, {X: x, Y: y, LeftReleased: true}}
	}
	step := func(want State, ins ...Input) {
		t.Helper()
		if run(t, a, ins...) {
			t.Fatal("unexpected quit")
		}
		if a.State() != want {
			t.Fatalf("state %v, want %v", a.State(), want)
		}
	}
	step(StateCreate, click(560, 215)...)           // New Game
	step(StateTitle, Input{Keys: []Key{KeyEscape}}) // Cancel hotkey
	step(StateCreate, Input{Keys: []Key{'N'}})      // N hotkey
	step(StateInGame, click(580, 455)...)           // OK
	if v := a.Viewport(); v.Dx() != 640 || v.Min.Y != 29 || v.Max.Y != 367 {
		t.Errorf("in-game viewport %v", v)
	}
	step(StateTitle, Input{Keys: []Key{KeyEscape}})        // game menu -> title for now
	step(StateCredits, click(560, 295)...)                 // Credits
	step(StateTitle, Input{X: 5, Y: 5, LeftPressed: true}) // any click leaves
	step(StateCredits, Input{Keys: []Key{'C'}})
	step(StateCredits, repeat(Input{}, 100)...)
	if !run(t, a, Input{Keys: []Key{KeyEscape}}) && a.State() != StateTitle {
		t.Fatalf("Esc in credits: %v", a.State())
	}
	if !run(t, a, click(560, 330)...) {
		t.Fatal("Quit did not quit")
	}
}

// Every credits line fits the 600 px scroller, so skipping the scroller's word wrap
// (0x44b226) changes nothing.
func TestCreditsFit(t *testing.T) {
	a := newTestApp(t, StateCredits)
	cr := a.Screen().(*credits)
	if len(cr.lines) < 50 {
		t.Fatalf("%d credit lines", len(cr.lines))
	}
	for _, ln := range cr.lines {
		if w := ln.font.TextWidth(ln.text); w >= creditsWin.Dx() {
			t.Errorf("line %q is %d px wide", ln.text, w)
		}
	}
	// Scrolls back to the title when done.
	for i := 0; i < cr.height+1 && a.State() == StateCredits; i++ {
		run(t, a, Input{})
	}
	if a.State() != StateTitle {
		t.Errorf("credits did not end: %v", a.State())
	}
}

// The creation tables read from MM8-Rel.exe match what the decompiled code implies.
func TestCreateTables(t *testing.T) {
	a := newTestApp(t, StateCreate)
	p := a.Screen().(*partyCreate)
	if p.className[4] != "Knight" || p.skillName[skillNone] != "None" || p.statNames[0] != "Might" {
		t.Errorf("names: %q %q %q", p.className[4], p.skillName[skillNone], p.statNames[0])
	}
	if p.PointsLeft() != 15 || p.statVal[0] != 11 {
		t.Errorf("knight: points %d might %d", p.PointsLeft(), p.statVal[0])
	}
	p.setFace(22) // Troll: Might 14 (cheap, green), Intellect 7 (dear, red)
	if p.statVal[0] != 14 || p.statVal[1] != 7 || p.statLbl[0].Color != gfx.RGB16(0, 0xff, 0) || p.statLbl[1].Color != gfx.RGB16(0xff, 0, 0) {
		t.Errorf("troll: %v colours %#x %#x", p.statVal, p.statLbl[0].Color, p.statLbl[1].Color)
	}
	p.statVal[0] = 16 // two points over base at 1/2 per point
	if p.PointsLeft() != 14 {
		t.Errorf("troll might 16: points %d", p.PointsLeft())
	}
	for k := 0; k < 2; k++ {
		if p.CreationSkill(k) == skillNone {
			t.Errorf("troll class skill %d missing", k)
		}
	}
}
