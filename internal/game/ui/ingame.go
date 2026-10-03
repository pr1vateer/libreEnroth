package ui

import (
	"image"
	"math"

	"libre-enroth/internal/gfx"
)

// Viewport is the 3D view in UI pixels: Viewport_Set(&g_viewport, 0, 29, 639, 366),
// inclusive corners.
//
// mm8: 0x4c0487 (Viewport_Set)
var Viewport = image.Rect(0, 29, 640, 367)

// In-game messages (GuiGame_Build 0x4c9885).
const msgGameMenu = 0x6b

type inGame struct {
	ct                          Container
	topbar, basebar             *gfx.Sprite
	compass, compcovr, mapframe *gfx.Sprite
	Yaw                         int // party yaw, 0..2047 (M4)
}

// newInGame builds the HUD frame around an empty (transparent) viewport.
//
// mm8: 0x4c9885 (GuiGame_Build)
func newInGame(r *Resources) (*inGame, error) {
	l := &loader{r: r}
	g := &inGame{
		topbar:   l.icon("topbar", false),
		basebar:  l.icon("Basebar", false),
		compass:  l.icon("IB-COMP-A", false),
		compcovr: l.icon("compcovr", false),
		mapframe: l.icon("mapframe", false),
	}
	for _, b := range []struct {
		x, msg int
		icon   string
		hotkey Key
	}{
		{380, 0xcd, "butn1", 0},
		{425, 0x68, "butn2", 0},
		{470, 0x6a, "butn3", 0},
		{525, msgGameMenu, "butn4", KeyEscape},
	} {
		btn := l.button(b.x, 2, Msg{ID: b.msg}, b.icon+"u", b.icon+"d", b.icon+"d", false)
		btn.Keyed = true
		btn.Hotkey = b.hotkey
		g.ct.Add(btn)
	}
	zoomIn := NewButton(624, 373, Msg{ID: 0x16f}, l.icon("map+up", false), nil, l.icon("map+ht", false))
	zoomOut := NewButton(624, 460, Msg{ID: 0x170}, l.icon("map-up", false), nil, l.icon("map-ht", false))
	g.ct.Add(zoomIn)
	g.ct.Add(zoomOut)
	return g, l.err
}

// Viewport is where the world shows through.
func (g *inGame) Viewport() image.Rectangle { return Viewport }

func (g *inGame) Update(in *Input) Transition {
	g.ct.Update(in)
	for _, m := range g.ct.Queue.Drain() {
		if m.ID == msgGameMenu {
			// The original opens the game menu (Esc); until M10/M12 it leads back
			// to the title.
			return goTo(StateTitle)
		}
	}
	return Transition{}
}

// Draw draws the HUD frame. The viewport is left untouched (transparent).
//
// mm8: 0x4c96cd (GuiGame_Draw)
func (g *inGame) Draw(c *gfx.Canvas) {
	c.Blit(g.topbar, 0, 0)
	c.Blit(g.basebar, 0, 367)
	// Minimap (FUN_0043f7f4 into 498,373-635,478) arrives with the maps (M3).
	c.SetClip(image.Rect(307, 0, 333, 20))
	c.Blit(g.compass, int(math.Round(float64(g.Yaw)*0.1171875))+51, 10)
	c.ResetClip()
	c.BlitKeyed(g.compcovr, 319-g.compcovr.W/2, 4)
	c.BlitKeyed(g.mapframe, 482, 367)
	// Party portraits and bars (FUN_0041bf09, FUN_0041b3de) need the party (M7).
	g.ct.Draw(c)
}
