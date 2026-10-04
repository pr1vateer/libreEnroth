// Command libre-enroth runs the game on the original Might and Magic VIII data.
//
//	libre-enroth [-data dir] [-res WxH|auto] [-window WxH] [-fullscreen] [-filter sharp|nearest|linear]
//	             [-state title|credits|create|ingame] [-map name.odm|name.blv] [-cam x,y,z,yaw,pitch]
//	             [-time HH:MM] [-screenshot out.png -frames N] [-mouse x,y] [-input script] [-freecam]
//	             [-party face,face,...] [-event N] [-house N] [-npc N]
//
// In game, the original's default keys: Up/Down walk, Left/Right turn (Ctrl: strafe),
// [ and ] strafe, Shift runs (U toggles always-run), X jumps, PgDn/Delete/End look
// up/down/ahead, PgUp/Insert fly up/down and Home lands (with the fly buff).
// 1-5 or a click on a portrait selects a party member. A click in the view or Space
// (the object in the middle of the view) runs a door's, lever's or decoration's event.
// Debug keys: F2 doors, F3 free camera, F4 fly buff, F5 water walking.
// Free camera: W/S or Up/Down move, A/D strafe, Left/Right turn, PgUp/PgDn pitch,
// Space/C up and down, Shift faster, right mouse drag looks around.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/display"
	"libre-enroth/internal/engine"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/game/world"
)

const inputHelp = "keys to replay, keys:ticks steps, e.g. Up:120,X:1,Right+Shift:30,-:60; Mouse@x/y and " +
	"Click@x/y move and click the mouse (UI pixels; a click picks from the last drawn frame, so wait " +
	"first: -:30,Click@320/200:1)"

func main() {
	log.SetFlags(0)
	log.SetPrefix("libre-enroth: ")
	var (
		dataDir    = flag.String("data", "", "game install directory (contains Data/ and MM8-Rel.exe); default $MM8_DATA")
		resFlag    = flag.String("res", "640x480", "render resolution WxH, or auto (window size x device scale)")
		windowFlag = flag.String("window", "", "window size WxH (default: the render resolution, or 640x480 with -res auto)")
		fullscreen = flag.Bool("fullscreen", false, "start in fullscreen")
		filterFlag = flag.String("filter", "sharp", "UI scaling filter: sharp, nearest or linear")
		stateFlag  = flag.String("state", "title", "start state: title, credits, create or ingame")
		screenshot = flag.String("screenshot", "", "write a PNG of the frame after -frames frames, then quit")
		frames     = flag.Int("frames", 5, "frames to render before -screenshot")
		mouseFlag  = flag.String("mouse", "", "pin the mouse at UI position x,y (for screenshots)")
		mapFlag    = flag.String("map", "", "start in game on this games.lod map (e.g. out01.odm, d05.blv); implies -state ingame")
		camFlag    = flag.String("cam", "", "start position x,y,z,yaw,pitch (z of the feet, dropped to the floor for the party; angles in 2048ths of a turn)")
		timeFlag   = flag.String("time", "9:00", "game time of day HH:MM on the first day")
		inputFlag  = flag.String("input", "", inputHelp)
		freeCam    = flag.Bool("freecam", false, "start with the free camera (F3) instead of the party")
		partyFlag  = flag.String("party", "", "debug party: 1-5 portrait faces 0-27 (as in party creation), e.g. 0,5,12; a created hero replaces the first")
		eventFlag  = flag.Int("event", 0, "run this map event (of the map's .evt) once the first map is loaded")
		houseFlag  = flag.Int("house", 0, "open this 2DEvents house's dialogue once the first map is loaded")
		npcFlag    = flag.Int("npc", 0, "open this NPC's dialogue once the first map is loaded")
	)
	flag.Parse()

	res, err := display.ParseRes(*resFlag)
	if err != nil {
		log.Fatal(err)
	}
	win := res
	if *windowFlag != "" {
		if win, err = display.ParseRes(*windowFlag); err != nil || win.Auto {
			log.Fatalf("-window: want WxH")
		}
	} else if res.Auto {
		win = display.Res{W: 640, H: 480}
	}
	filter, err := engine.ParseFilter(*filterFlag)
	if err != nil {
		log.Fatal(err)
	}
	start, err := ui.ParseState(*stateFlag)
	if err != nil {
		log.Fatal(err)
	}
	if *mapFlag != "" {
		start = ui.StateInGame
	}
	var cam *world.FreeCam
	if *camFlag != "" {
		cam = &world.FreeCam{}
		if _, err := fmt.Sscanf(*camFlag, "%g,%g,%g,%g,%g", &cam.X, &cam.Y, &cam.Z, &cam.Yaw, &cam.Pitch); err != nil {
			log.Fatalf("-cam: want x,y,z,yaw,pitch")
		}
	}
	var hour, minute int
	if _, err := fmt.Sscanf(*timeFlag, "%d:%d", &hour, &minute); err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		log.Fatalf("-time: want HH:MM")
	}
	var faces []int
	if *partyFlag != "" {
		if faces, err = parseParty(*partyFlag); err != nil {
			log.Fatalf("-party: %v", err)
		}
	}
	cfg := engine.Config{Res: res, Filter: filter, Screenshot: *screenshot, Frames: *frames}
	if *inputFlag != "" {
		if cfg.Script, err = ui.ParseScript(*inputFlag); err != nil {
			log.Fatalf("-input: %v", err)
		}
	}
	if *mouseFlag != "" {
		var p image.Point
		if _, err := fmt.Sscanf(*mouseFlag, "%d,%d", &p.X, &p.Y); err != nil {
			log.Fatalf("-mouse: want x,y")
		}
		cfg.Mouse = &p
	}

	dir, err := assets.DataDir(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	data, err := assets.OpenAll(dir)
	if err != nil {
		log.Fatalf("%v (set -data or MM8_DATA to the game directory)", err)
	}
	defer data.Close()
	tables, err := world.LoadTables(data)
	if err != nil {
		log.Fatal(err)
	}
	tex := world.NewTextureCache(data)
	resources := ui.NewResources(data)
	if *mapFlag != "" {
		resources.StartMap = *mapFlag
	}
	for i, f := range faces {
		// Debug members after the hero are not roster characters.
		p := party.Player{Face: f, Voice: f, Class: ui.ClassForFace(f), RosterID: -1}
		if i == 0 {
			p.RosterID = 0
			resources.Party.Players[0] = p
		} else {
			resources.Party.Players = append(resources.Party.Players, p)
		}
	}
	// The game seeds rand() with GetTickCount() (Game_Init 0x464974); screenshots keep
	// the runtime's default seed so they are reproducible.
	if *screenshot == "" {
		resources.Rand = party.NewRand(uint32(time.Now().UnixMilli()))
	}
	var sess *world.Session
	resources.LoadWorld = func(name string) (ui.World, error) {
		if sess == nil {
			var err error
			if sess, err = world.UISession(resources); err != nil {
				return nil, err
			}
			sess.SetTimeOfDay(hour, minute) // -time: the hour of the first day
		}
		w, err := world.Load(data, tables, tex, name, sess)
		if err != nil {
			return nil, err
		}
		w.FreeCamOn = *freeCam
		if cam != nil {
			w.Cam = *cam
			w.SetPartyFromCam()
			cam = nil // only the first map
		}
		if *eventFlag != 0 {
			w.RunEvent(*eventFlag, true)
			*eventFlag = 0
		}
		if *houseFlag != 0 {
			w.SpeakInHouse(*houseFlag)
			*houseFlag = 0
		}
		if *npcFlag != 0 {
			w.SpeakNPC(*npcFlag, true)
			*npcFlag = 0
		}
		return w, nil
	}
	app, err := ui.NewApp(resources, start)
	if err != nil {
		log.Fatal(err)
	}
	g, err := engine.New(app, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := engine.Run(g, "libre-enroth", win.W, win.H, *fullscreen); err != nil {
		data.Close()
		log.Print(err)
		os.Exit(1)
	}
}

// parseParty parses -party: 1 to 5 comma-separated faces.
func parseParty(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	if len(parts) > party.MaxMembers {
		return nil, fmt.Errorf("%d faces, at most %d", len(parts), party.MaxMembers)
	}
	faces := make([]int, len(parts))
	for i, p := range parts {
		f, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || f < 0 || f >= ui.PortraitFaces {
			return nil, fmt.Errorf("face %q: want 0..%d", p, ui.PortraitFaces-1)
		}
		faces[i] = f
	}
	return faces, nil
}
