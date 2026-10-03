package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/gfx"
)

// State is a top-level game state.
type State int

// The states M2 knows. The original keeps them in g_mainMenuState (0x6cea38).
const (
	StateTitle State = iota
	StateCredits
	StateCreate
	StateInGame
)

var stateNames = [...]string{"title", "credits", "create", "ingame"}

func (s State) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// ParseState parses a state name (title, credits, create, ingame).
func ParseState(s string) (State, error) {
	for i, n := range stateNames {
		if n == s {
			return State(i), nil
		}
	}
	return 0, fmt.Errorf("unknown state %q (want title, credits, create or ingame)", s)
}

// Screen is one state's UI.
type Screen interface {
	Update(in *Input) Transition
	Draw(c *gfx.Canvas)
}

// Transition is what a screen asks for after a tick; the zero value means stay.
type Transition struct {
	to   State
	set  bool
	quit bool
}

func goTo(s State) Transition { return Transition{to: s, set: true} }

var quitGame = Transition{quit: true}

// App runs the state machine: title -> credits / party creation -> in-game.
type App struct {
	r          *Resources
	state      State
	screen     Screen
	cursor     *gfx.Sprite
	mx, my     int
	ShowCursor bool
}

// NewApp builds the screen for start.
func NewApp(r *Resources, start State) (*App, error) {
	cur, err := r.Cache.Icon("MICON1", false)
	if err != nil {
		return nil, err
	}
	a := &App{r: r, cursor: cur, ShowCursor: true, mx: -100, my: -100}
	if err := a.enter(start); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *App) enter(s State) error {
	var sc Screen
	var err error
	switch s {
	case StateTitle:
		sc, err = newTitle(a.r)
	case StateCredits:
		sc, err = newCredits(a.r)
	case StateCreate:
		sc, err = newPartyCreate(a.r)
	case StateInGame:
		sc, err = newInGame(a.r)
	default:
		err = fmt.Errorf("ui: no screen for %v", s)
	}
	if err != nil {
		return fmt.Errorf("%v screen: %w", s, err)
	}
	a.state, a.screen = s, sc
	return nil
}

// State is the current state.
func (a *App) State() State { return a.state }

// Screen is the current screen.
func (a *App) Screen() Screen { return a.screen }

// Update runs one tick. It reports quit when the player chose Quit.
func (a *App) Update(in *Input) (quit bool, err error) {
	a.mx, a.my = in.X, in.Y
	t := a.screen.Update(in)
	switch {
	case t.quit:
		return true, nil
	case t.set:
		return false, a.enter(t.to)
	}
	return false, nil
}

// Draw composes the current screen and the mouse cursor into c.
//
// mm8: 0x469304 (Mouse_SetCursor: MICON1, hotspot 0,0)
func (a *App) Draw(c *gfx.Canvas) {
	c.Clear()
	a.screen.Draw(c)
	c.ResetClip()
	if a.ShowCursor {
		c.BlitKeyed(a.cursor, a.mx, a.my)
	}
}

// Viewport is the 3D view rectangle in UI pixels for the current screen (empty when the
// screen has none). M3 renders the world into display.Transform.UIRectToScreen(Viewport()).
func (a *App) Viewport() image.Rectangle {
	if g, ok := a.screen.(*inGame); ok {
		return g.Viewport()
	}
	return image.Rectangle{}
}
