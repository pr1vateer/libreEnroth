package world

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
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
	r.LoadWorld = func(name string) (ui.World, error) { return Load(e.d, e.tables, e.tex, name) }
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
}

// Frozen SHA-256s of the composed frames (3D view and HUD). Regenerate with:
// MM8_DATA=../games_mm8 go test ./internal/game/world -run Golden -update
var worldHashes = map[string]string{
	"out01_spawn": "4912cbd4a6ceabded1b5b2b15ee652e0a92d5f3f54954386a89971a2b992aaf5",
	"out01_town":  "95d7ba2edc669e9aa8ec224e2e75dd9232f4c7f1ebc0cf9c92665d853d5c6282",
	"out01_sky":   "c043596c9cf3605b0d962484bc4de6578e3b602dc1bdcefc4384360326a440e3",
	"out02_wide":  "f99cdbcae529270e6cffe997edc5f33d5f621210e6bb45810ee14207eabf017b",
	"d05_start":   "c163444b1de455455565db13afa29443169c028e1d698bebeaf01b1c31b76f73",
	"d05_door":    "fbd5c2cdc7df5a787c8056ad259fb4855b767b577c1f6d1c971742167d4a5d5e",
	"d13_torch":   "aff2b5344b19b2bf651221e8633307f5cf66e8d61750aec39bfc0772961dc0f3",
	"d16_start":   "b7da1a107401dcd402b87148fc61497f2e1ca60d8911e9bf3f1730d0a3cfe42c",
	"d28_stairs":  "9ec44357c4e3b098a6a51ef2a3c9cd5d70dc1905b0044d0defc6611611b3d2b2",
	"d28_climb":   "ee6063478ef190262c8404fd78768657e8ba10d01a186adba3084f7dcfec8e16",
	"out01_run":   "de72e72af65332928c9852829920338b3a397426a6cf26dc61c4775378786cdc",
	"d05_walk":    "802ef595fee3000a7aa67eeb81cccbfb06cb1eb1acd9c40131abe3e29c24bfe1",
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
}

func TestGolden(t *testing.T) {
	e := newEnv(t)
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			a := e.app(t, v.mapName)
			w := a.World().(*World)
			w.FreeCamOn = v.input == "" // the M3 views
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
		w, err := Load(e.d, e.tables, e.tex, name)
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
