// Command libre-enroth runs the game on the original Might and Magic VIII data.
//
//	libre-enroth [-data dir] [-res WxH|auto] [-window WxH] [-fullscreen] [-filter sharp|nearest|linear]
//	             [-state title|credits|create|ingame] [-screenshot out.png -frames N] [-mouse x,y]
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/display"
	"libre-enroth/internal/engine"
	"libre-enroth/internal/game/ui"
)

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
	cfg := engine.Config{Res: res, Filter: filter, Screenshot: *screenshot, Frames: *frames}
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
	app, err := ui.NewApp(ui.NewResources(data), start)
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
