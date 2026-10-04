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
	"out01_spawn": "f6cb8470cbec97f3566f4e69024335058a783556d6f82e31cf300999376c50f2",
	"out01_town":  "1262268ebc8d34b399d7c1c2e3a80c29fb1a49d4e44f6f01ed4b88fac02b9503",
	"out01_sky":   "ee006b8c268243d7e0afe87db635af83a272c5f07c878ed99987fdc810d1efed",
	"out02_wide":  "937305642d051d6d6a4da5a6333113eae7d1a03455b2598e3a40136c0d5500fc",
	"d05_start":   "da7a470ac610504997950a763d367ac12a9edfa4405c9f99f69f195fb839c3f0",
	"d05_door":    "3e683ba830a09e72fa1695db26361be4d89d95e03599c8df4800d5ef4c470c30",
	"d13_torch":   "c7f5c5ec31a2c17705858e937849a390e446c6cad46e32a90618209fbfb767f1",
	"d16_start":   "c493927786e3353b1c05aba39734e61eade1dc7ece7900699043fbe710cf3676",
	"d28_stairs":  "aa903b859138b163c2579fe7aa7d39420cae4134c2a4d243a3793b2dc8c92c62",
	"d28_climb":   "0aac6431371c88cb77acf00bc942308afd433dc5525fe33aba77f79a4b388277",
	"out01_run":   "32004ae1eecfe6d2eea5f6c08e43d817b4e8ac5dfb67572cec5ada6964646e5e",
	"d05_walk":    "4408b53d5d46094448b3d766d4a366b84061510f68a69e07fb45317e9f3a80e5",
	"d05_click":   "38f758ce60de1775149fbf85e2c715a1a354f563a83f65641bcf5de200f7ad08",
	// M6
	"npc_dialog":     "f9868ac59e5cc2bad6ff81b89c4c9965c74fe780c47bb372f7ebe7d1f89acd16",
	"house_resident": "98f7fa0d049e83f59a08019caebb51d7f4b82c6d9334bbabba6979b41d692756",
	"evt_message":    "8bb11a34baaa24f4d530482af5abe9c905cfc79c75ec7a1c7b9d0274befb56e2",
	// M6c
	"house_tavern":   "3c9a4468ad902dca1e71c623212102841253264487e0d10bee8746b2b696a7f4",
	"house_temple":   "89ed9631ff611b76158336c7aebd0c8af6487a478b4665e57a4f9e4ea9e8c4d6",
	"house_bank":     "331c87c8bb0ac5ad775289e6246d6bfbf2cceeb2b9833a663fc454b886db038e",
	"house_stables":  "eb72e31e2745bdcc89413a5dff8035e30ae5ea087dad5ff5fb5dafe224ebc6a5",
	"transition_d05": "949375486fc712de9eb942db19ae5d1797891d9a1ec08a6d16f1bc59c1496083",
	"edge_out02":     "50d845ae82cc3cb1dfeee1809f4058ab8a57202ce7b50559be47d3d99b169f55",
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
