package world

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/items"
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
	// frozen stops the monsters (F7), for views about something else.
	frozen bool
}

// Frozen SHA-256s of the composed frames (3D view and HUD). Regenerate with:
// MM8_DATA=../games_mm8 go test ./internal/game/world -run Golden -update
var worldHashes = map[string]string{
	"out01_spawn": "25731855575c6104c7c5fa79dda2176a2a82e81f4102537a7f645b9c7d9e5ada",
	"out01_town":  "ec03119b655761cd40dfab48db759d6df8d591c57246894832a9ce1041634856",
	"out01_sky":   "a5caaa4d4265bad848b159407492b0d09f981bf0743cf0cd95017b6007b52b68",
	"out02_wide":  "cddbc4073a608b1aa5d0a6164a4453772e9e37d10e17a91b6126bd346827a001",
	"d05_start":   "39151aeebcf3da875b8cc19cedcfb6b1bb2cb9a87112b19a108c20dc9749af38",
	"d05_door":    "c3de256d2c17f28211c18b58b21b3529064da596003150fb95c3d77a79e6799e",
	"d13_torch":   "bdede009ba188dc6ddc6ca43dde95a5bbb8a3271d0626b00d35d45edaa3e8110",
	"d16_start":   "23af5fec8878f212756504dd677340e1645f5f4344bfb3f219e446df408f7f49",
	"d28_stairs":  "d0c04b7a2c01bcf7e1664cbe1d252d4e6eadd5ce627fe4382a9a3396faeaa79a",
	"d28_climb":   "e61b434fab1b2a818a6df159cea06345c7bc33cf8d2ea653cee177409322bc88",
	"out01_run":   "1e621b8cee4a217e61d71d23071032cfef0d0ac58227afe3dddfe7d6d42cc7bf",
	"d05_walk":    "0d411990e53e2b8089ff5dc45d8ad1dcb2dd3f645d6bf7abf9891fe7d1f45b3d",
	"d05_click":   "a56ed09721ec0ea3c7058f825d1d08f4a5b4e1ae9d16ae40d1881d520d9153cd",
	// M6
	"npc_dialog":     "4eb299cc91b7a86ec4f2596049133f2037c2f5cbd0e92ac9d7ae9b2dfb27de00",
	"house_resident": "8a1d602c3ff48599bf4fed6ea9dbeb657dbc2f0c118ba6a191932d93a956b876",
	"evt_message":    "95f2d6cd86d60db5d249adbba67277ef2424cd7b1109ef63c9937bd6cf932332",
	// M6c
	"house_tavern":   "92d577f460ca99b9a23a25b6dfd78571d5dcd2d6ccea6aef8e67505e8b857b66",
	"house_temple":   "f36ed53d89f27aaaa011e48ec083dfac3292f1c854f30c9ca52d5d076be0f4d2",
	"house_bank":     "ed57489348e3fb469f1d9a15209d60ab73eb1dc2dfa42d0b95cfe0c3e4039464",
	"house_stables":  "68d89186417c3af826de4f2f42d5801a51f2147271ba40a3e077439f78c0f27f",
	"transition_d05": "028f4f90d84a1ba719c2cddfd5ad5fc0ca93dca83613606ff25e3553c28e69b1",
	"edge_out02":     "d721897ec89a68958b95ffaf071f81dd956457a0b56154ef72a7efc4645384c8",
	// M7b
	"chest_open":  "c2622bfc8cde0d40f87c3c8e963e6cd81e882217feb3aca9abd9420ed51bc873",
	"chest_popup": "3f7093e3a403ac80f2a15cb6ea2530d997ea7ad406890f14a32ca254b7e2794b",
	// M7c
	"shop_weapon":    "cbb4ef9ff2b1a04aec0646a2f0b831317999529cf31fffce115eb5ce884c3fa5",
	"shop_armor":     "d42478e123be7206875bb20575ab85fed524baf9904cbdf12544fa7aab47a686",
	"shop_magic":     "bd80435c332f2e3f3b71d743701eb23675c721819f61a3408a27f3c813fbaab4",
	"shop_alchemist": "d11835db565f4a405a02f77b481db034a50b7d9ec3b1aa6f970880f38bee5f92",
	"guild_shelf":    "28a557afb717dd5cc2a5de73945fb1bf9c2ab5a5a9f10948984304f30bd5f4ea",
	"shop_sell":      "04416850a246afb8c77088a7733166d50d3f26ad3713975271d9f1faa36dafb3",
	"shop_popup":     "cbb4ef9ff2b1a04aec0646a2f0b831317999529cf31fffce115eb5ce884c3fa5",
	// M7d
	"house_learn":    "44bad96ad4163d71544e76a72b90e67af33f372319768078d361a5e0d9c137d5",
	"house_training": "0dd41d711cea0e15442b1e7275c27b13ab58f49071fe6d5939cf61f37b2ad11a",
	"teacher_topic":  "655fde40df79d5477b125598ba44b8d0215263d25a93b85d48ceab107d4eeb67",
	"inn_roster":     "372b92c2515917b29a72149c83cbfebe1e3d19c3bbe62e9893837bf9ee12f308",
	// M8a
	"out01_actors": "f2ca33a0b66a0434ad85cffe6fec16535e27decbc74f052c338a2898679120c1",
	"d05_spawned":  "83fad542b7c2cbbf5a3c07fe5ba552ce3759208e91e70cdc1e562f74371e5bcf",
	"hover_actor":  "96bfde9f7c8dcfe0df57643fb729d83fa06d7612e1474dec349414d5058e9901",
	// M8b
	"out01_pursuit": "2e54f7fd9ba5cff3f091d511a63e8e4ef91722469409ac811bc86548fe4842fc",
	"d05_melee":     "813750eba21222aff0dd33aed92414bb78e6d5bbb356f623e6069747b1ccd975",
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
		click: "Click@320/200:1,Mouse@320/200:150", w: 640, h: 480, frozen: true},
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
	// M7b: d05's chest 1, its trap disarmed (event 82 opens it), the mouse off the items.
	{name: "chest_open", mapName: "d05.blv", setup: func(w *World) {
		w.chests()[1].Flags &^= items.ChestTrapped
		w.RunEvent(82, true)
	}, click: "Mouse@600/300:2", w: 640, h: 480},
	// M7b: the same, the right button held on the light crossbow.
	{name: "chest_popup", mapName: "d05.blv", setup: func(w *World) {
		w.chests()[1].Flags &^= items.ChestTrapped
		w.RunEvent(82, true)
	}, click: "Mouse@600/300:2,Right@300/120:2", w: 640, h: 480},
	// M7c: True Mettle's (1) standard shelf, the mouse on the two-handed axe.
	{name: "shop_weapon", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(1) },
		click: "-:2,Click@555/160:1,Mouse@290/150:3", w: 640, h: 480, hour: 9},
	// M7c: the Tannery's (15) special shelves.
	{name: "shop_armor", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(15) },
		click: "-:2,Click@555/197:1,Mouse@220/220:3", w: 640, h: 480, hour: 9},
	// M7c: Fearsome Fetishes' (29) standard shelves, the mouse on a potion.
	{name: "shop_magic", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(29) },
		click: "-:2,Click@555/160:1,Mouse@115/180:3", w: 640, h: 480, hour: 9},
	// M7c: Herbal Elixirs' (42) special shelves.
	{name: "shop_alchemist", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(42) },
		click: "-:2,Click@555/197:1,Mouse@115/320:3", w: 640, h: 480, hour: 9},
	// M7c: Cures and Curses' (139) fire shelf, the mouse on a book.
	{name: "guild_shelf", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(139) },
		click: "-:2,Click@555/180:1,Mouse@60/150:3", w: 640, h: 480, hour: 9},
	// M7c: True Mettle's Sell with a longsword, a broken Templar's sword, unidentified
	// chain mail (green) and a stolen mace in the pack, the mouse on the longsword.
	{name: "shop_sell", mapName: "out01.odm", setup: func(w *World) {
		p, t := &w.S.Party.Players[0], w.Tables().Items
		p.AddItem(t, -1, items.Item{Number: 1, Flags: items.FlagIdentified})
		p.AddItem(t, -1, items.Item{Number: 4, Flags: items.FlagIdentified | items.FlagBroken})
		p.AddItem(t, -1, items.Item{Number: 90})
		p.AddItem(t, -1, items.Item{Number: 66, Flags: items.FlagIdentified | items.FlagStolen})
		w.SpeakInHouse(1)
	}, click: "-:2,Click@555/245:1,Mouse@300/300:2,Click@555/167:1,Mouse@20/50:3", w: 640, h: 480, hour: 9},
	// M7c: the right button held on a shelf item of True Mettle.
	{name: "shop_popup", mapName: "out01.odm", setup: func(w *World) { w.SpeakInHouse(1) },
		click: "-:2,Click@555/160:1,Mouse@290/150:2,Right@290/150:2", w: 640, h: 480, hour: 9},
	// M7d: Rites of Passage (89): Learn Skills, the mouse on the first skill.
	{name: "house_learn", mapName: "out01.odm", setup: func(w *World) {
		w.S.Party.Gold = 1000
		w.SpeakInHouse(89)
		w.Dialog().Click(dialog.Button{Msg: dialog.MsgService, Param: dialog.SvcLearn})
	}, click: "Mouse@555/215:3", w: 640, h: 480, hour: 9},
	// M7d: Rites of Passage with the experience for level 2, the mouse on Train.
	{name: "house_training", mapName: "out01.odm", setup: func(w *World) {
		w.S.Party.Players[0].Exp = 1500
		w.SpeakInHouse(89)
	}, click: "Mouse@555/200:3", w: 640, h: 480, hour: 9},
	// M7d: Puddle Thain (248) offering the staff's expert rank to a level 4 staff.
	{name: "teacher_topic", mapName: "out01.odm", setup: func(w *World) {
		w.S.Party.Gold = 5000
		w.S.Party.Players[0].Skills[party.SkillStaff] = 4
		w.SpeakInHouse(248)
		d := w.Dialog()
		if d.Sel == 0 {
			d.SelectResident(len(d.Portraits) - 1)
		}
		for _, b := range d.Buttons {
			if d.Label(b) == w.Tables().Topics.Topic[300] {
				d.Click(b)
				break
			}
		}
	}, click: "Mouse@555/240:3", w: 640, h: 480, hour: 9},
	// M7d: the Adventurer's Inn (185) with roster characters 1..9 waiting, the second
	// picked.
	{name: "inn_roster", mapName: "out01.odm", setup: func(w *World) {
		for id := 1; id <= 9; id++ {
			w.S.Party.QBits.Set(400+id, true)
		}
		w.SpeakInHouse(185)
	}, click: "-:2,Click@120/80:1,Mouse@300/300:20", w: 640, h: 480, hour: 9},
	// M8a: out01's actors as the map loads them (a guard on the path, villagers further).
	{name: "out01_actors", mapName: "out01.odm", party: &FreeCam{X: 5880, Y: 6950, Z: 740, Yaw: 512},
		click: "Mouse@600/50:2", w: 640, h: 480, hour: 9, frozen: true},
	// M8a: d05's lower hall with the couatls its spawn points made.
	{name: "d05_spawned", mapName: "d05.blv", party: &FreeCam{X: 3328, Y: 1200, Z: -1000, Yaw: 512},
		click: "Mouse@600/50:2", w: 640, h: 480, frozen: true},
	// M8a: the mouse on the guard: its name on the status line.
	{name: "hover_actor", mapName: "out01.odm", party: &FreeCam{X: 5880, Y: 6950, Z: 740, Yaw: 512},
		click: "Mouse@325/200:20", w: 640, h: 480, hour: 9, frozen: true},
	// M8b: three pirates south of Ravenshore's town see the party and close in.
	{name: "out01_pursuit", mapName: "out01.odm", party: &FreeCam{X: 4000, Y: 3000, Z: 300, Yaw: 1536},
		click: "Mouse@600/50:150", w: 640, h: 480, hour: 9},
	// M8b: d05's lizardmen at the party's throat, its HP bar down.
	{name: "d05_melee", mapName: "d05.blv", party: &FreeCam{X: -2950, Y: -1150, Z: 945, Yaw: 1650},
		click: "Mouse@600/50:150", w: 640, h: 480},
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
			w.AIFrozen = v.frozen
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
