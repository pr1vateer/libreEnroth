package ui

import (
	"fmt"
	"image"
	"math"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
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

// The hotspot over the minimap: clicking it posts 0x1f0 (the map screen, M6),
// hovering it 0x5c, the date and time on the status line.
const (
	msgMapScreen = 0x1f0
	msgHoverTime = 0x5c
)

// conditionNames are the global.txt names of the conditions (g_conditionNames
// 0xbb2f0c, filled by Txt_LoadGlobal 0x45181f), indexed by party.Condition.
var conditionNames = [...]int{0x34, 0xf1, 0xe, 4, 0x45, 0x75, 0xa6, 0x41, 0xa6, 0x41, 0xa6, 0x41,
	0xa2, 0xe7, 0x3a, 0xdc, 0x4c, 0x259, 0x62}

type inGame struct {
	r                           *Resources
	ct                          Container
	topbar, basebar             *gfx.Sprite
	compass, compcovr, mapframe *gfx.Sprite
	Yaw                         int // party yaw, 0..2047 (M4)
	world                       World
	minimap                     *minimap
	portraits                   *portraits
	timeSpot                    *Hotspot
	lucida                      *text.Font
	rest                        *restScreen
	dialog                      *dialogScreen
	message                     *messageBox
	transition                  *transitionScreen
	scoreBG                     *gfx.Sprite
	smallnum                    *text.Font
}

// newInGame builds the HUD frame around the viewport and loads mapName into it when
// the resources can load worlds (else the viewport stays transparent).
//
// mm8: 0x4c9885 (GuiGame_Build)
func newInGame(r *Resources, mapName string) (*inGame, error) {
	l := &loader{r: r}
	g := &inGame{
		r:        r,
		lucida:   l.font("lucida.fnt"),
		topbar:   l.icon("topbar", false),
		basebar:  l.icon("Basebar", false),
		compass:  l.icon("IB-COMP-A", false),
		compcovr: l.icon("compcovr", false),
		mapframe: l.icon("mapframe", false),
		scoreBG:  l.icon("ScoreBG", false),
		smallnum: l.font("smallnum.fnt"),
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
	g.timeSpot = NewHotspot(0x1f2, 0x17d, 0x7d, 0x55, Msg{ID: msgMapScreen})
	g.ct.Add(g.timeSpot)
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
		if l.err == nil {
			if _, err := g.openDialogs(); err != nil {
				return nil, err
			}
		}
	}
	return g, l.err
}

// Viewport is where the world shows through.
// The rest screen and a house (its clip) cover it.
func (g *inGame) Viewport() image.Rectangle {
	if g.rest != nil || g.dialog != nil && g.dialog.video != nil || g.transition != nil && g.transition.video != nil {
		return image.Rectangle{}
	}
	return Viewport
}

func (g *inGame) Update(in *Input) Transition {
	g.r.Status.Tick()
	if g.rest != nil {
		if g.rest.update(in) {
			g.rest = nil
		}
		g.r.Status.ClearHover()
		g.portraits.update()
		return Transition{}
	}
	if busy, err := g.updateDialogs(in); busy || err != nil {
		g.r.Status.ClearHover()
		g.portraits.update()
		return Transition{err: err}
	}
	g.ct.Update(in)
	if in.Pressed('R') {
		g.ct.Queue.Post(Msg{ID: msgRest})
	}
	if in.Pressed(KeyEnter) {
		g.toggleTurnBased()
	}
	for _, m := range g.ct.Queue.Drain() {
		switch m.ID {
		case msgRest:
			if err := g.openRest(); err != nil {
				return Transition{err: err}
			}
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
	if g.world != nil && g.rest == nil {
		if it, ok := g.world.(Interactor); ok {
			if in.LeftPressed && image.Pt(in.X, in.Y).In(Viewport) {
				it.Click(in.X, in.Y)
			}
			if in.Pressed(KeySpace) {
				it.Interact()
			}
		}
		g.world.Update(in)
		_, _, g.Yaw = g.world.Party()
		if err := g.travel(); err != nil {
			return Transition{err: err}
		}
		if _, err := g.openDialogs(); err != nil {
			return Transition{err: err}
		}
	}
	g.hover(in)
	g.portraits.update()
	return Transition{}
}

// updateDialogs runs the message box or the open dialogue for one tick instead of the
// game view (the game timer is paused while they are open, as the original pauses it).
// busy reports that one of them had the tick.
//
// mm8: 0x443b6f / 0x443f4b / 0x44328b (Timer_Pause when they open)
func (g *inGame) updateDialogs(in *Input) (busy bool, err error) {
	w, ok := g.world.(Dialogs)
	if !ok {
		return false, nil
	}
	if g.message != nil {
		if done, answer, ok := g.message.update(in); done {
			g.message = nil
			w.Answer(answer, ok)
			if err := g.afterEvent(w); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if g.transition != nil {
		if done, ok := g.transition.update(in); done {
			g.transition = nil
			w.AnswerTransition(ok)
			if err := g.afterEvent(w); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if g.dialog != nil {
		d := g.dialog.d
		if g.dialog.update(in) {
			g.dialog = nil
			if d.RestInn {
				if err := g.openInnRest(); err != nil {
					return true, err
				}
			}
		}
		if err := g.afterEvent(w); err != nil {
			return true, err
		}
		return true, nil
	}
	return g.openDialogs()
}

// afterEvent follows up a dialogue click or an answer: a map change closes the
// dialogue, a new message box or dialogue opens.
func (g *inGame) afterEvent(w Dialogs) error {
	if t, ok := g.world.(Traveler); ok {
		if name, ok := t.Travel(); ok {
			w.CloseDialog()
			g.dialog, g.message, g.transition = nil, nil, nil
			if err := g.loadMap(name); err != nil {
				return err
			}
		}
	}
	_, err := g.openDialogs()
	return err
}

// openDialogs opens the screens for what events asked for: the message box first,
// then the dialogue (a new one replaces the old).
func (g *inGame) openDialogs() (bool, error) {
	w, ok := g.world.(Dialogs)
	if !ok {
		return false, nil
	}
	if d := w.Dialog(); d == nil {
		g.dialog = nil
	} else if g.dialog == nil || g.dialog.d != d {
		ds, err := newDialogScreen(g.r, w, d)
		if err != nil {
			return false, err
		}
		g.dialog = ds
	}
	if t := w.TransitionDialog(); t == nil {
		g.transition = nil
	} else if g.transition == nil || g.transition.t != t {
		ts, err := newTransitionScreen(g.r, t)
		if err != nil {
			return false, err
		}
		g.transition = ts
	}
	if text, input, ok := w.MessageBox(); ok && g.message == nil {
		var d *dialog.Dialog
		if g.dialog != nil {
			d = g.dialog.d
		}
		mb, err := newMessageBox(g.r, text, input, d)
		if err != nil {
			return false, err
		}
		g.message = mb
	}
	return g.dialog != nil || g.message != nil || g.transition != nil, nil
}

// travel loads the map a MoveToMap event asked for into the view.
//
// mm8: 0x42f877 (map change: Evt_MapLeave 0x441c68, then the map load 0x46411c)
func (g *inGame) travel() error {
	t, ok := g.world.(Traveler)
	if !ok || g.r.LoadWorld == nil {
		return nil
	}
	name, ok := t.Travel()
	if !ok {
		return nil
	}
	return g.loadMap(name)
}

// loadMap puts map name in the view.
func (g *inGame) loadMap(name string) error {
	w, err := g.r.LoadWorld(name)
	if err != nil {
		return err
	}
	g.world = w
	zoom := g.minimap.zoom
	l := &loader{r: g.r}
	g.minimap = newMinimap(l, name)
	g.minimap.zoom = zoom
	_, _, g.Yaw = w.Party()
	return l.err
}

// toggleTurnBased starts or ends turn-based mode (Enter). Only the clock part is there
// yet: while it is on the game timer is stopped, so time and timers stand still; the
// combat queue and rounds are M8/M9.
//
// mm8: 0x42efd9 (Input_GameKeys), 0x406332 (TurnBased_Start), 0x406687 (TurnBased_End)
func (g *inGame) toggleTurnBased() {
	m := g.r.Party
	m.TurnBased = !m.TurnBased
}

// openRest opens the rest screen, or says why the party cannot rest here.
//
// mm8: 0x42f877 (msg 0x68)
func (g *inGame) openRest() error {
	site, ok := g.world.(RestSite)
	if !ok {
		return nil
	}
	if msg := site.RestRefusal(); msg != 0 {
		g.r.Status.Show(g.r.GlobalText(msg), 2)
		return nil
	}
	rs, err := newRestScreen(g.r, site, false)
	if err != nil {
		return err
	}
	g.rest = rs
	return nil
}

// openInnRest is the night a tavern's room pays for (msg 0x199): the rest screen opens
// already resting, until an hour past the next 5 AM, at no food.
//
// mm8: 0x42f877 (msg 0x199)
func (g *inGame) openInnRest() error {
	site, ok := g.world.(RestSite)
	if !ok {
		return nil
	}
	rs, err := newRestScreen(g.r, site, true)
	if err != nil {
		return err
	}
	rs.restAtInn(0)
	g.rest = rs
	return nil
}

// hover sets the status line's hover text from what the mouse is over: a portrait
// (name, class and condition), the minimap (the date and time), an object in the view,
// else nothing.
//
// mm8: 0x420aab (Mouse_UpdateHover), 0x42f877 (msg 0x5e, msg 0x5c)
func (g *inGame) hover(in *Input) {
	st := g.r.Status
	for i, s := range g.portraits.slots {
		if s.hovered && i < len(g.r.Party.Players) {
			st.SetHover(g.memberText(&g.r.Party.Players[i]))
			return
		}
	}
	if g.timeSpot.hovered {
		names := clock.LoadNames(g.r.GlobalText)
		st.SetHover(names.Format(g.r.Party.Calendar))
		return
	}
	if it, ok := g.world.(Interactor); ok && image.Pt(in.X, in.Y).In(Viewport) {
		if s := it.Hover(in.X, in.Y); s != "" {
			st.SetHover(s)
			return
		}
	}
	st.ClearHover()
}

// memberText is a portrait's hover text: "Name the Class: Condition".
//
// mm8: 0x42f877 (msg 0x5e: globalTxt[0x1ad], class name, ": ", condition name)
func (g *inGame) memberText(p *party.Player) string {
	class := g.r.GlobalText(classNameGlobal + p.Class)
	s := fmt.Sprintf(g.r.GlobalText(0x1ad), p.Name, class) + ": "
	if c := int(p.MainCondition()); c < len(conditionNames) {
		s += g.r.GlobalText(conditionNames[c])
	}
	return s
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
	if g.rest == nil { // Hud_DrawGoldFood skips g_screenMode 5
		drawGoldFood(c, g.scoreBG, g.smallnum, g.r.Party)
	}
	g.ct.Draw(c)
	if g.rest != nil {
		g.rest.draw(c)
	}
	if g.dialog != nil {
		g.dialog.draw(c)
	}
	if g.transition != nil {
		g.transition.draw(c)
	}
	if g.message != nil {
		g.message.draw(c)
	}
	g.r.Status.Draw(c, g.lucida)
	g.portraits.draw(c) // the portrait panel is the last child
}
