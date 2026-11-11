// Golden-image tests for the world: whole frames (the 3D view and the HUD over it) are
// rendered in software and compared, by SHA-256, against hashes frozen in worldHashes.
// Any change to what reaches the screen (renderer, map loading, lighting, sprites, the
// UI, the actors' AI, the event interpreter, ...) shows up as a hash mismatch.
//
// The game data is needed: assettest.Dir reads $MM8_DATA (relative paths are taken from
// the module root) and skips the test when it is unset.
//
// # Running
//
//	MM8_DATA=<data_path> go test ./internal/game/world -run Golden
//	MM8_DATA=<data_path> go test ./internal/game/world -run Golden/d05_melee
//	MM8_DATA=<data_path> go test ./internal/game/world -run Golden -update -v
//
// With -update nothing is compared: every view is written to <module>/out/world_<name>.png
// and its hash is logged as a ready-to-paste `"name": "hash",` line for worldHashes. The
// intended workflow is to look at the PNGs, and only when they look right copy the new
// hashes in. A view missing from worldHashes always fails (want is "").
//
// # What one view does
//
// Each entry of views is a subtest. The setup is the same as the real game's:
//
//  1. newEnv (once for all views) opens the game data and loads the shared tables and
//     texture cache.
//  2. env.app builds a ui.App straight into StateInGame on view.mapName, with
//     LoadWorld wired to world.Load over a UISession, i.e. the UI's party (the default
//     one-member party), its RNG (MSVC rand() seeded with 1, so every run is
//     deterministic), status line and global.txt. The mouse cursor is hidden.
//  3. If the map opened with a dialogue (d05's OnMapReload makes Simon Templar speak),
//     one Esc closes it, as a player would.
//  4. FreeCamOn is set for the plain camera views (no input, no party), AIFrozen from
//     view.frozen (F7: the monsters stand still in views about something else).
//
// Then the fields of the view pick one of three kinds of driving, applied in this order:
//
//   - setup / party / click: the whole in-game screen. setup runs on the loaded world
//     (open a house, a dialogue, a chest, tweak the party's gold or skills, ...); party
//     places the party at that camera (SetPartyFromCam, as -cam does in the game). One
//     frame is composed first, so the screen has been laid out, then click (a ui.Script)
//     is replayed through App.Update, one 60 Hz tick per Script step tick. Mouse moves,
//     clicks, right-button popups and keys all go through the real UI and world update,
//     so the AI, doors and events run for those ticks too.
//   - input: a ui.Script fed to World.Update only (no HUD input), driving the party from
//     the map's start: walking, turning, jumping, looking up and down (M4).
//   - cam: a free camera (M3). Outdoors its Z is taken as height above the ground at
//     (X, Y). With doorTicks set (indoors), every door is toggled and moved that many
//     ticks before the shot.
//
// Last, the clock is set to view.hour:view.mins (outdoors this picks the sky and the
// light) and the frame is composed at view.w x view.h and hashed.
//
// # Scripts
//
// click and input use ui.ParseScript: comma-separated "keys:ticks" steps, keys joined by
// '+' or '-' for none. "Mouse@x/y:n" keeps the mouse at UI point (x, y) of the 640x480
// UI for n ticks, "Click@x/y:n" also clicks the left button there, "Right@x/y:n" holds
// the right button. E.g. "-:2,Click@555/160:1,Mouse@290/150:3" waits two ticks, clicks
// the house's first service button, then hovers a shelf item for three ticks.
//
// # Composing a frame
//
// Compose is the software equivalent of engine.Game.Draw: display.Fit places the
// 640x480 UI in the w x h target (largest uniform scale, centred), the world is
// rendered into the viewport's screen rectangle at native resolution, and the UI canvas
// is laid over it nearest-neighbour scaled, its transparent pixels letting the 3D view
// through. The output is opaque black outside both.
//
// BenchmarkFrame at the end times World.Render alone (no UI) for an outdoor and an
// indoor map at a few screen sizes.
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
	"libre-enroth/internal/game/physics"
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
	// The ready gem (M9a) is on every frame where a member can act, so these hashes
	// moved when it appeared. d05_melee did not: that member is still recovering.
	"out01_spawn": "b7443a6e5ab7cc51ab7ca2088578ac4a2bbdfbfa94213789141f382405cd3896",
	"out01_town":  "5de7d4ece22f95185002f7ceebd167c06362977a0baa6949553a85494fe60923",
	"out01_sky":   "fada3d73b1ea5c899d1bc0e46b37ec1f6fea731d3501f65a96fbb4e8a3bccae7",
	"out02_wide":  "23f082a3ff0d3aa74540562bd75331c55d3d77c639c8b60fef1ad7d5f7bfdcda",
	"d05_start":   "cd9969e1e22105b4c00eda27e3562bed0356099940abebe0d4b28cc11edc291d",
	"d05_door":    "471c5791fb5fb1fac04e78de6e000adf76804e08f0b9c9548ce9abb4f7387232",
	"d13_torch":   "859a4565f8032d037fe812392c380c109f7eb5f3ad9df54103b019be72235e86",
	"d16_start":   "a61841f559b036e76eabc91bac11a1243e0a1eeb5c6e98f4ebee8f4ce1a48b89",
	"d28_stairs":  "a71b56e6ea44dca941de20408a78c572ab906009755321d17f8e7ea5614e4422",
	"d28_climb":   "e49f1c406b78c0a3a54c24a002db0d6c4b1971e07baecda230064adbb73b364c",
	"out01_run":   "d90b2340594d60f14f879329c6aff22e1d6bb5efc235155e964e26ce27f63a3e",
	"d05_walk":    "7f4c0b637a82f60ff21a1ec44cbe5c697a58256619504a774c33da5d776bd990",
	"d05_click":   "a54e7f639bfdb5025828f0ef101228ddf150e4c06905e385d5483ba7f244bf0a",
	// M6
	"npc_dialog":     "7c092eaa98a128078c3388c307b6d7d066758b7927e130642adc36cf68d24202",
	"house_resident": "7200b148ddc7117b2f8f3ed42a4a3ed313f1167e3b048840e5060b236731614a",
	"evt_message":    "c861552eb63a190aaf061e57ce542de2925e6c9a8c21cb3aafabadbf9fa74a7e",
	// M6c
	"house_tavern":   "44f4904206ecfa91960f34e0a78e612bd62df0ef6697fa16076585425350f4e6",
	"house_temple":   "fadd5aafee938a1ba853114eaa723a6de512cad655ff1b1fa42c8b9fa236ccc4",
	"house_bank":     "789d8815b0b61ab86f876e726fc8e23ed9fe5b5a330ba5ff0e09c71e1b28f451",
	"house_stables":  "6d66e1d309311ce58b86c7f6089bfc8c10c387d62c6b4cdee10971a825448128",
	"transition_d05": "a80d15c44568322f94aed86dfbea74a5f2ad42547bcaec0be0db49ef6d2f006d",
	"edge_out02":     "9e878ff8db6bf1457bcfea95a3704dfa708d236726d86f489b571505a04f484d",
	// M7b
	"chest_open":  "815c30f04d31724f521d8915f53dddb2dc2a5b384c7d11c14e02145ddf258ebb",
	"chest_popup": "a83ed2ac69954843744a0a5f325c9cdba0b98d21296916d7201af9e815d51ea3",
	// M7c
	"shop_weapon":    "eaa738b1074df9aa9ff3e38432a0f1b8d33c1bb398f7caff01426dee50343c40",
	"shop_armor":     "e5102c8ddab255d85151460cfa5b6153c09470e906de0ffe82ba828fa3979475",
	"shop_magic":     "f9b326ccec2d1b4387f047ba4be3fbcb12840bf6205d28cb28ac49ed6faae17d",
	"shop_alchemist": "5dec8f2ede28c65436a6ae60677cf925e473dfceaa795dae5f8de771c431c3e5",
	"guild_shelf":    "15663ba05185632b35862c5918c437fe31704a8055776e642383cac833d693ba",
	"shop_sell":      "1a4492372a2350537d440abbad76987390479760b4a89eb404c5348c4ce15305",
	"shop_popup":     "eaa738b1074df9aa9ff3e38432a0f1b8d33c1bb398f7caff01426dee50343c40",
	// M7d
	"house_learn":    "557e3a268f993d539c044262f1e5531065dec0016c19e7ff8558e24c820b98b8",
	"house_training": "48434bf6b000d079099fe59d6860082afe68eaac9462ca44d002f75a008d5217",
	"teacher_topic":  "c866769838e7b941975fd7f1d6adac7bddf0c95cc9e5e2d59ced8e7eedc80628",
	"inn_roster":     "d8c3dfe74aed68dabc496c6df0be8dd2ee72aac534a0a289bcf07ab8fde8a43d",
	// M8a
	"out01_actors": "79c190cc835892399925f90f00a72f7698207aaa5f8276cc4c25dcb2543949df",
	"d05_spawned":  "0ea10910fdde777b0e07feed2882ba8dd0856cff40869e3f9e903f7c18e0b61a",
	"hover_actor":  "8210651bfc5394366527eb38b3ddc69e78191b9c34d6b604272e7807c36a3cd7",
	// M8b
	"out01_pursuit":  "818d8f0e1712dac2f2440f6a8837cdc837d656817bc00d49fbda0774f515a3ee",
	"d05_melee":      "813750eba21222aff0dd33aed92414bb78e6d5bbb356f623e6069747b1ccd975",
	"ground_items":   "6580e5df4dd046108fde67535af5d8f5f6f3258da8bbb4762ea173c248e8ba24",
	"actor_dead":     "404428d4c2e34e0c0a1597f584d9f75322361a946430d70d119f07779df63fac",
	"bounty_topic":   "614764f89d0d2c8a21c961cd0844e9bb01799fb910e51824deb4f0bf20321a49",
	"rest_encounter": "96f4ea3e656ac8565cbdf796df04f62b534686f8e301a66a87910aed926f0aa3",
	// M9a
	"melee_hit":  "b5cac47081865bb0a87fae3049eefafe4b4948f74977c22ff47ff42ffe4b2031",
	"party_dead": "ef0001dc3d21775ff099aafa0459cde0978305bde486130aba4fa75b476f1268",
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
	// M8c: d13's items on the floor of sector 37 among its gogs, the mouse on the gold.
	{name: "ground_items", mapName: "d13.blv", party: &FreeCam{X: 0, Y: 5300, Z: -1540, Yaw: 512},
		click: "Mouse@552/326:2", w: 640, h: 480, frozen: true},
	// M8c: the guard before the party dies (Actor_Die, as F6 does); two seconds later
	// it lies dead.
	{name: "actor_dead", mapName: "out01.odm", party: &FreeCam{X: 5880, Y: 6950, Z: 740, Yaw: 512},
		setup: func(w *World) { w.brain().Die(nearestActor(w, 5880, 7500)) },
		click: "Mouse@600/50:150", w: 640, h: 480, hour: 9},
	// M8c: Bryant Conlan's bounty topic in house 272: this month's monster and reward.
	{name: "bounty_topic", mapName: "out01.odm", setup: func(w *World) {
		w.SpeakInHouse(272)
		d := w.Dialog()
		d.SelectResident(0)
		d.Click(dialog.Button{Msg: dialog.MsgHouseTopic, Param: dialog.ParamTopic0})
	}, click: "Mouse@556/440:2", w: 640, h: 480, hour: 9},
	// M8c: a rest encounter's monsters (out01's first type) arrive near the party,
	// which turns to them; four seconds later they close in.
	{name: "rest_encounter", mapName: "out01.odm", setup: func(w *World) {
		p := w.group
		p.Teleport(2000, 3000, int32(w.outdoor.Map.GroundZ(2000, 3000)), 0)
		n := len(w.Actors())
		w.spawnEncounter(w.mapStats(), 1)
		a := &w.Actors()[n]
		p.Dir = physics.Atan2(int32(a.Pos[0])-p.X, int32(a.Pos[1])-p.Y)
		w.FreeCamOn = false
	}, click: "Mouse@600/50:240", w: 640, h: 480, hour: 9},
	// M9a: a blow on the guard in front of the party. The ready gem sits under the portrait.
	{name: "melee_hit", mapName: "out01.odm", setup: func(w *World) {
		w.FreeCamOn = false
		i := nearestActor(w, 5880, 7500)
		a := &w.Actors()[i]
		a.Info.AC = 0
		a.Info.Resist = [10]uint16{}
		a.HP = 80
		x, y := int32(a.Pos[0])-80, int32(a.Pos[1])
		w.group.Teleport(x, y, int32(a.Pos[2]), physics.Atan2(int32(a.Pos[0])-x, int32(a.Pos[1])-y))
		pl := &w.S.Party.Players[0]
		pl.Name = "Zoltan"
		pl.Skills = [party.NumSkills]uint16{}
		pl.Skills[party.SkillUnarmed] = 60
		pl.Equip = [party.NumSlots]int32{}
	}, click: "A:1,-:25", w: 640, h: 480, hour: 9},
	// M9a: the party is dead, so the temple in Jadame takes them back.
	{name: "party_dead", mapName: "d05.blv", setup: func(w *World) {
		w.FreeCamOn = false
		w.S.Party.Players[0].Conditions[party.CondDead] = 1
		w.S.Party.Players[0].HP = 0
	}, click: "-:8", w: 640, h: 480, hour: 9},
}

// nearestActor is the living actor nearest to (x, y).
func nearestActor(w *World, x, y int32) int {
	best, bestD := -1, int64(1)<<62
	for i, a := range w.Actors() {
		if !a.CanAct() {
			continue
		}
		dx, dy := int64(a.Pos[0])-int64(x), int64(a.Pos[1])-int64(y)
		if d := dx*dx + dy*dy; d < bestD {
			best, bestD = i, d
		}
	}
	return best
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
