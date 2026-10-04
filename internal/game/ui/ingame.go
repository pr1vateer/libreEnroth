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

// Minimap zoom buttons (msgs of the map+/map- buttons in GuiGame_Build).
const (
	msgZoomIn  = 0x16f
	msgZoomOut = 0x170
)

type inGame struct {
	ct                          Container
	topbar, basebar             *gfx.Sprite
	compass, compcovr, mapframe *gfx.Sprite
	Yaw                         int // party yaw, 0..2047 (M4)
	world                       World
	minimap                     *minimap
	portraits                   *portraits
}

// newInGame builds the HUD frame around the viewport and loads mapName into it when
// the resources can load worlds (else the viewport stays transparent).
//
// mm8: 0x4c9885 (GuiGame_Build)
func newInGame(r *Resources, mapName string) (*inGame, error) {
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
	zoomIn := NewButton(624, 373, Msg{ID: msgZoomIn}, l.icon("map+up", false), nil, l.icon("map+ht", false))
	zoomOut := NewButton(624, 460, Msg{ID: msgZoomOut}, l.icon("map-up", false), nil, l.icon("map-ht", false))
	g.ct.Add(zoomIn)
	g.ct.Add(zoomOut)
	// A new game: Party_InitNewGame runs before the HUD is built. Loading a saved game
	// (M10) will skip it.
	r.Party.NewGame(r.Rand)
	g.portraits = newPortraits(l, &g.ct) // the last child (GuiGame_Build)
	if l.err == nil && r.LoadWorld != nil && mapName != "" {
		w, err := r.LoadWorld(mapName)
		if err != nil {
			return nil, err
		}
		g.world = w
		g.minimap = newMinimap(l, mapName)
		_, _, g.Yaw = w.Party()
	}
	return g, l.err
}

// Viewport is where the world shows through.
func (g *inGame) Viewport() image.Rectangle { return Viewport }

func (g *inGame) Update(in *Input) Transition {
	g.ct.Update(in)
	for _, m := range g.ct.Queue.Drain() {
		switch m.ID {
		case msgSelectPlayer:
			g.portraits.m.ClickPortrait(m.Param)
		case msgGameMenu:
			// The original opens the game menu (Esc); until M10/M12 it leads back
			// to the title.
			return goTo(StateTitle)
		case msgZoomIn:
			if g.minimap != nil {
				g.minimap.zoom = min(g.minimap.zoom*2, minimapZoomMax)
			}
		case msgZoomOut:
			if g.minimap != nil {
				g.minimap.zoom = max(g.minimap.zoom/2, minimapZoomMin)
			}
		}
	}
	if g.world != nil {
		g.world.Update(in)
		_, _, g.Yaw = g.world.Party()
	}
	g.portraits.update()
	return Transition{}
}

// Draw draws the HUD frame and the party panel. The viewport is left untouched
// (transparent).
//
// mm8: 0x4c96cd (GuiGame_Draw)
func (g *inGame) Draw(c *gfx.Canvas) {
	c.Blit(g.topbar, 0, 0)
	c.Blit(g.basebar, 0, 367)
	if g.minimap != nil && g.world.Outdoor() {
		g.minimap.draw(c, g.world)
	}
	c.SetClip(image.Rect(307, 0, 333, 20))
	c.Blit(g.compass, int(math.Round(float64(g.Yaw)*0.1171875))+51, 10)
	c.ResetClip()
	c.BlitKeyed(g.compcovr, 319-g.compcovr.W/2, 4)
	c.BlitKeyed(g.mapframe, 482, 367)
	// The status line (0x41bf09) and the gold/food counters (0x41b3de) come later.
	g.ct.Draw(c)
	g.portraits.draw(c) // the portrait panel is the last child
}
