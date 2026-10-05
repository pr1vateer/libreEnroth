package world

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
	"libre-enroth/internal/game/party"
	"os"
	"path/filepath"
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
	"libre-enroth/internal/display"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/render"
)

var update = flag.Bool("update", false, "write the composed frames to <module>/out/world_*.png and log their hashes")

type env struct {
	d      *assets.Data
	tables *Tables
	tex    *TextureCache
}

func newEnv(t testing.TB) *env {
	t.Helper()
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	tables, err := LoadTables(d)
	if err != nil {
		t.Fatal(err)
	}
	return &env{d: d, tables: tables, tex: NewTextureCache(d)}
}

func (e *env) app(t testing.TB, mapName string) *ui.App {
	t.Helper()
	r := ui.NewResources(e.d)
	r.StartMap = mapName
	var sess *Session
	r.LoadWorld = func(name string) (ui.World, error) {
		if sess == nil {
			var err error
			if sess, err = UISession(r); err != nil {
				return nil, err
			}
		}
		return Load(e.d, e.tables, e.tex, name, sess)
	}
	a, err := ui.NewApp(r, ui.StateInGame)
	if err != nil {
		t.Fatal(err)
	}
	a.ShowCursor = false
	return a
}

// Compose renders the world into the viewport at w x h and lays the UI canvas over it
// (nearest-neighbour scaled), the software equivalent of engine.Game.Draw.
func Compose(a *ui.App, w, h int) *image.RGBA {
	tr := display.Fit(w, h)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 0xff
	}
	if wd := a.World(); wd != nil {
		rect := tr.UIRectToScreen(ui.Viewport)
		f := render.NewFrame(rect.Dx(), rect.Dy())
		wd.Render(f)
		src := f.Bytes()
		for y := 0; y < f.H; y++ {
			copy(out.Pix[out.PixOffset(rect.Min.X, rect.Min.Y+y):], src[4*y*f.W:4*(y+1)*f.W])
		}
	}
	c := gfx.NewCanvas()
	a.Draw(c)
	ui := tr.UIRect()
	for y := ui.Min.Y; y < ui.Max.Y; y++ {
		for x := ui.Min.X; x < ui.Max.X; x++ {
			ux, uy := tr.ToUI(float64(x)+0.5, float64(y)+0.5)
			if ux < 0 || uy < 0 || ux >= gfx.ScreenW || uy >= gfx.ScreenH {
				continue
			}
			s := c.Img.PixOffset(ux, uy)
			if c.Img.Pix[s+3] == 0 {
				continue
			}
			copy(out.Pix[out.PixOffset(x, y):][:4], c.Img.Pix[s:s+4])
		}
	}
	return out
}

func hashImage(img *image.RGBA) string {
	s := sha256.Sum256(img.Pix)
	return hex.EncodeToString(s[:])
}

func moduleRoot(t testing.TB) string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
}

type view struct {
	name       string
	mapName    string
	cam        *FreeCam // nil: the map's default camera; outdoors Z is above the ground
	w, h       int
	hour, mins int
	doorTicks  int // indoors: toggle every door and let them move this long
	// input, when set, drives the party from the map's start with a ui.Script through
	// World.Update; the frame shows the party's view.
	input string
	// party places the party (as -cam does); click then replays a ui.Script through
	// the whole in-game screen (clicks, Space, hover) after a first frame.
	party *FreeCam
	click string
	// setup runs on the loaded world before click (M6: open a dialogue).
	setup func(w *World)
}

// Frozen SHA-256s of the composed frames (3D view and HUD). Regenerate with:
// MM8_DATA=../games_mm8 go test ./internal/game/world -run Golden -update
var worldHashes = map[string]string{
	"out01_spawn": "7b0fb22b8057b8d063b3d0343776f7c5417c19f1499c678107a9986ea015a70f",
	"out01_town":  "f1edec5eb155675a187e3dfa9f3f44350d2666d2aca9c7877442ad21abe97ea2",
	"out01_sky":   "a5caaa4d4265bad848b159407492b0d09f981bf0743cf0cd95017b6007b52b68",
	"out02_wide":  "cddbc4073a608b1aa5d0a6164a4453772e9e37d10e17a91b6126bd346827a001",
	"d05_start":   "39151aeebcf3da875b8cc19cedcfb6b1bb2cb9a87112b19a108c20dc9749af38",
	"d05_door":    "c3de256d2c17f28211c18b58b21b3529064da596003150fb95c3d77a79e6799e",
	"d13_torch":   "bdede009ba188dc6ddc6ca43dde95a5bbb8a3271d0626b00d35d45edaa3e8110",
	"d16_start":   "5ff9357815c4e29b80b1e721d16d7d8172b36d861551118e7ec6b24de892ed90",
	"d28_stairs":  "4611bddef0140ac496bc78efd66f0e0fb685d1be516efa3cd8427b31111f506a",
	"d28_climb":   "e61b434fab1b2a818a6df159cea06345c7bc33cf8d2ea653cee177409322bc88",
	"out01_run":   "6c1c9522234f6a1c2be942aa1f806b52766c5ecb683dd66e97b4d1cb08a9abd3",
	"d05_walk":    "0d411990e53e2b8089ff5dc45d8ad1dcb2dd3f645d6bf7abf9891fe7d1f45b3d",
	"d05_click":   "a56ed09721ec0ea3c7058f825d1d08f4a5b4e1ae9d16ae40d1881d520d9153cd",
	// M6
	"npc_dialog":     "c2db329648ee87a25bfab1868f6427357e11df7586844cdbb4a5d6b38d66f0c7",
	"house_resident": "8a1d602c3ff48599bf4fed6ea9dbeb657dbc2f0c118ba6a191932d93a956b876",
	"evt_message":    "95f2d6cd86d60db5d249adbba67277ef2424cd7b1109ef63c9937bd6cf932332",
	// M6c
	"house_tavern":   "92d577f460ca99b9a23a25b6dfd78571d5dcd2d6ccea6aef8e67505e8b857b66",
	"house_temple":   "f36ed53d89f27aaaa011e48ec083dfac3292f1c854f30c9ca52d5d076be0f4d2",
	"house_bank":     "ed57489348e3fb469f1d9a15209d60ab73eb1dc2dfa42d0b95cfe0c3e4039464",
	"house_stables":  "68d89186417c3af826de4f2f42d5801a51f2147271ba40a3e077439f78c0f27f",
	"transition_d05": "028f4f90d84a1ba719c2cddfd5ad5fc0ca93dca83613606ff25e3553c28e69b1",
	"edge_out02":     "d721897ec89a68958b95ffaf071f81dd956457a0b56154ef72a7efc4645384c8",
}

var views = []view{
	{name: "out01_spawn", mapName: "out01.odm", w: 640, h: 480, hour: 9},
	{name: "out01_town", mapName: "out01.odm", cam: &FreeCam{X: 3766, Y: 7649, Z: 700, Yaw: 100, Pitch: -50}, w: 640, h: 480, hour: 9},
	{name: "out01_sky", mapName: "out01.odm", cam: &FreeCam{X: 2000, Y: 9000, Z: 100, Yaw: 1536, Pitch: 120}, w: 1280, h: 720, hour: 15},
	{name: "out02_wide", mapName: "out02.odm", cam: &FreeCam{X: 0, Y: -4000, Z: 300, Yaw: 512, Pitch: -30}, w: 1280, h: 720, hour: 11},
	{name: "d05_start", mapName: "d05.blv", w: 640, h: 480},
	{name: "d05_door", mapName: "d05.blv", cam: &FreeCam{X: 8512, Y: 1800, Z: -640, Yaw: 512}, w: 640, h: 480, doorTicks: 300},
	{name: "d13_torch", mapName: "d13.blv", cam: &FreeCam{X: -1650, Y: 3176, Z: -1545}, w: 640, h: 480},
	{name: "d16_start", mapName: "d16.blv", w: 1280, h: 720},
	{name: "d28_stairs", mapName: "d28.blv", cam: &FreeCam{X: 0, Y: -448, Z: 0, Pitch: 60}, w: 1280, h: 720},
	// M4: the party walking (party mode, the original's default keys)
	{name: "d28_climb", mapName: "d28.blv", input: "Left:14,Up:45,Delete:1,-:2,Delete:1", w: 640, h: 480},
	{name: "out01_run", mapName: "out01.odm", input: "Up+Shift:150,PgDn:1,-:2,PgDn:1", w: 640, h: 480, hour: 9},
	{name: "d05_walk", mapName: "d05.blv", input: "Up:90,Right:20", w: 640, h: 480},
	// M5: a click on d05's door (event 11) opens it; the mouse stays on it ("Door").
	{name: "d05_click", mapName: "d05.blv", party: &FreeCam{X: 8512, Y: 2150, Z: -640, Yaw: 512},
		click: "Click@320/200:1,Mouse@320/200:150", w: 640, h: 480},
	// M6: S'ton's dialogue (out01 event 500's SpeakNPC 31), the mouse on his second topic.
	{name: "npc_dialog", mapName: "out01.odm", setup: func(w *World) { w.SpeakNPC(31, true) },
		click: "Mouse@556/224:2", w: 640, h: 480, hour: 10},
	// M6: the House of Thistle (230): Thistle's "Ingredients" answered, the clip 40 ticks
	// in.
	{name: "house_resident", mapName: "out01.odm", setup: func(w *World) {
		w.SpeakInHouse(230)
		d := w.Dialog()
		d.SelectResident(0)
		d.Click(d.Buttons[1])
	}, click: "Mouse@556/200:40", w: 640, h: 480, hour: 9},
	// M6: Escaton's riddle (global event 164, InputString) in the message box.
	{name: "evt_message", mapName: "pbp.odm", setup: func(w *World) {
		w.npcs().SetTopic(26, 0, 164)
		w.SpeakInHouse(184)
		d := w.Dialog()
		d.Click(d.Buttons[0])
	}, click: "-:31", w: 640, h: 480, hour: 9},
	// M6c: the Grog and Grub (107), the mouse on Fill Packs.
	{name: "house_tavern", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(107) },
		click: "Mouse@555/210:40", w: 640, h: 480, hour: 9},
	// M6c: Mystic Medicine (74) for a weak member: Heal for 10 gold, the mouse on it.
	{name: "house_temple", mapName: "out01.odm", setup: func(w *World) {
		w.S.Party.Players[0].Conditions[party.CondWeak] = 1
		w.SpeakInHouse(74)
	}, click: "Mouse@555/185:20", w: 640, h: 480, hour: 9},
	// M6c: the Some Place Safe (128): Deposit, 50 typed.
	{name: "house_bank", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(128) },
		click: "-:2,Click@555/160:1,5:1,0:1,Mouse@300/300:20", w: 640, h: 480, hour: 9},
	// M6c: the Ravenshore stable (54) on day 1: 2 days to Alvar.
	{name: "house_stables", mapName: "out02.odm", setup: func(w *World) { w.SpeakInHouse(54) },
		click: "Mouse@300/300:20", w: 640, h: 480, hour: 9},
	// M6c: d05's exit (event 501) asks first.
	{name: "transition_d05", mapName: "d05.blv", setup: func(w *World) { w.RunEvent(501, true) },
		click: "Mouse@300/300:2", w: 640, h: 480},
	// M6c: walking north off Ravenshore.
	{name: "edge_out02", mapName: "out02.odm", party: &FreeCam{X: 0, Y: 0x5700, Z: 2000, Yaw: 512},
		click: "Up:150,Mouse@300/300:2", w: 640, h: 480, hour: 9},
}

func TestGolden(t *testing.T) {
	e := newEnv(t)
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			a := e.app(t, v.mapName)
			w := a.World().(*World)
			// d05's OnMapReload has Simon Templar speak (a deferred SpeakNPC): Esc ends
			// it, as the player would, so the view shows.
			if w.Dialog() != nil {
				a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
			}
			w.FreeCamOn = v.input == "" && v.party == nil // the M3 views
			if v.setup != nil {
				v.setup(w)
			}
			if v.party != nil || v.setup != nil {
				if v.party != nil {
					w.Cam = *v.party
					w.SetPartyFromCam()
				}
				Compose(a, v.w, v.h)
				sc, err := ui.ParseScript(v.click)
				if err != nil {
					t.Fatal(err)
				}
				for !sc.Done() {
					in := &ui.Input{}
					sc.Next(in)
					a.Update(in)
				}
			}
			if v.input != "" {
				sc, err := ui.ParseScript(v.input)
				if err != nil {
					t.Fatal(err)
				}
				for !sc.Done() {
					in := &ui.Input{}
					sc.Next(in)
					w.Update(in)
				}
			}
			if v.cam != nil {
				w.Cam = *v.cam
				if w.outdoor != nil {
					w.Cam.Z += w.outdoor.Map.GroundZ(w.Cam.X, w.Cam.Y)
				}
			}
			if v.doorTicks > 0 {
				w.indoor.ToggleDoors()
				w.indoor.UpdateDoors(v.doorTicks)
			}
			w.Clock = Clock{Hour: v.hour, Minute: v.mins}
			img := Compose(a, v.w, v.h)
			got := hashImage(img)
			if *update {
				dir := filepath.Join(moduleRoot(t), "out")
				os.MkdirAll(dir, 0o755)
				f, err := os.Create(filepath.Join(dir, "world_"+v.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				png.Encode(f, img)
				f.Close()
				t.Logf("%q: %q,", v.name, got)
				return
			}
			if want, ok := worldHashes[v.name]; !ok || want != got {
				t.Errorf("%s: hash %s, want %s", v.name, got, want)
			}
		})
	}
}

// BenchmarkFrame renders the out01 start view and an indoor view (d16) into the
// viewport of a few screen sizes.
func BenchmarkFrame(b *testing.B) {
	e := newEnv(b)
	for _, name := range []string{"out01.odm", "d16.blv"} {
		w, err := Load(e.d, e.tables, e.tex, name, nil)
		if err != nil {
			b.Fatal(err)
		}
		for _, res := range [][2]int{{640, 480}, {1024, 768}, {1280, 720}, {1920, 1080}} {
			rect := display.Fit(res[0], res[1]).UIRectToScreen(ui.Viewport)
			b.Run(fmt.Sprintf("%s/%dx%d", name, res[0], res[1]), func(b *testing.B) {
				f := render.NewFrame(rect.Dx(), rect.Dy())
				w.Render(f) // warm the texture cache
				for b.Loop() {
					w.Render(f)
				}
			})
		}
	}
}
