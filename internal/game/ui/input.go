// Package ui is the menu/HUD layer: widgets that behave like the original GUI framework
// (re/notes/ui.md#framework) and the screens built from them. It draws into a
// gfx.Canvas and reads a ui.Input, so it runs headless in tests without Ebiten.
package ui

import "slices"

// Key identifies a key. Letters and digits are their upper-case ASCII codes, as the
// original's hotkeys are (GuiWidget_SetHotkey uppercases).
type Key int

// Non-printing keys. Values below 0x100 match the Windows virtual-key codes the game sees.
const (
	KeyBackspace Key = 0x08
	KeyTab       Key = 0x09
	KeyEnter     Key = 0x0d
	KeyEscape    Key = 0x1b
	KeySpace     Key = 0x20
	KeyLeft      Key = 0x100 + iota
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyDelete
	KeyPageUp
	KeyPageDown
	KeyShift
	KeyF1
	KeyF2
	KeyF3
	KeyF4
)

// Input is one tick of input, with the mouse already mapped to 640x480 UI pixels.
type Input struct {
	X, Y                        int
	Left, Right                 bool // held
	LeftPressed, LeftReleased   bool // edges this tick
	RightPressed, RightReleased bool
	Keys                        []Key  // pressed this tick (including key repeat)
	Held                        []Key  // down during this tick
	Runes                       []rune // text typed this tick
}

// Pressed reports whether k was pressed this tick.
func (in *Input) Pressed(k Key) bool { return slices.Contains(in.Keys, k) }

// Down reports whether k is held.
func (in *Input) Down(k Key) bool { return slices.Contains(in.Held, k) }

// Clicked reports a left or right press this tick.
func (in *Input) Clicked() bool { return in.LeftPressed || in.RightPressed }
