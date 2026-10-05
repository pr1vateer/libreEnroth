package ui

import (
	"image"

	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Msg is a GUI message: what a widget posts when activated.
type Msg struct{ ID, Param, Param2 int }

// MsgQueue holds the messages posted during a tick.
type MsgQueue struct{ msgs []Msg }

const msgQueueCap = 40

// Post appends m; like the original's 40-slot queue, a full queue drops it.
//
// mm8: 0x4c4f36 (GuiButton_Activate posting to g_msgQueue 0x51e280)
func (q *MsgQueue) Post(m Msg) {
	if len(q.msgs) < msgQueueCap {
		q.msgs = append(q.msgs, m)
	}
}

// Drain returns and clears the queue.
func (q *MsgQueue) Drain() []Msg {
	m := q.msgs
	q.msgs = nil
	return m
}

// Widget is a child of a Container. The event methods return true when the widget
// consumed the event, which stops the dispatch.
type Widget interface {
	Draw(c *gfx.Canvas)
	OnMouseMove(x, y int, q *MsgQueue) bool
	OnLButtonDown(x, y int, q *MsgQueue) bool
	OnLButtonUp(x, y int, q *MsgQueue) bool
	OnKey(k Key, q *MsgQueue) bool
	base() *widget
}

// hit is the original's inclusive hit test: x <= px <= x+w, y <= py <= y+h.
//
// mm8: 0x4c3980 (GuiWidget_HitTest)
func hit(r image.Rectangle, x, y int) bool {
	return r.Min.X <= x && x <= r.Max.X && r.Min.Y <= y && y <= r.Max.Y
}

// widget is the GuiWidget state shared by every widget.
type widget struct {
	Msg      Msg // posted by Activate (0 ID = none)
	Hotkey   Key // 0 = none
	disabled bool

	lbuttonDown, hovered bool
	activateOnPress      bool

	rect     func() image.Rectangle
	activate func(q *MsgQueue) // default: post Msg
	onEnter  func(q *MsgQueue)
	onLeave  func()
}

func (w *widget) base() *widget { return w }

func (w *widget) doActivate(q *MsgQueue) {
	if w.activate != nil {
		w.activate(q)
	} else if w.Msg.ID != 0 {
		q.Post(w.Msg)
	}
}

// OnMouseMove detects entering and leaving.
//
// mm8: 0x4c3b57 (GuiWidget_OnMouseMove)
func (w *widget) OnMouseMove(x, y int, q *MsgQueue) bool {
	if w.disabled {
		return false
	}
	if hit(w.rect(), x, y) {
		if !w.hovered {
			w.hovered = true
			if w.onEnter != nil {
				w.onEnter(q)
			}
		}
		return true
	}
	if w.hovered {
		w.hovered = false
		if w.onLeave != nil {
			w.onLeave()
		}
	}
	return false
}

// OnLButtonDown arms the widget when pressed inside it.
//
// mm8: 0x4c3a6a (GuiWidget_OnLButtonDown)
func (w *widget) OnLButtonDown(x, y int, q *MsgQueue) bool {
	if w.disabled || !hit(w.rect(), x, y) {
		return false
	}
	w.lbuttonDown = true
	if w.activateOnPress {
		w.doActivate(q)
	}
	return true
}

// OnLButtonUp activates the widget if it was pressed and is released inside.
//
// mm8: 0x4c3aad (GuiWidget_OnLButtonUp)
func (w *widget) OnLButtonUp(x, y int, q *MsgQueue) bool {
	if !w.lbuttonDown {
		return false
	}
	w.lbuttonDown = false
	if w.disabled || !hit(w.rect(), x, y) || w.activateOnPress {
		return false
	}
	w.doActivate(q)
	return true
}

// OnKey activates the widget when its hotkey is pressed.
//
// mm8: 0x4c3be9 (GuiWidget_OnKey; the auto-repeat throttle is left to the key source)
func (w *widget) OnKey(k Key, q *MsgQueue) bool {
	if w.disabled || w.Hotkey == 0 || k != w.Hotkey {
		return false
	}
	w.doActivate(q)
	return true
}

// Button states, indexing Button.Icons.
const (
	StateUp = iota
	StateDown
	StateHover
	StateDisabled
)

// Button is an icon button (GuiButton): four icons, one per state.
type Button struct {
	widget
	X, Y  int
	Icons [4]*gfx.Sprite // up, down, hover, disabled
	Keyed bool
	State int
}

// NewButton places a button at x, y posting msg.
//
// mm8: 0x4c4c22 (GuiButton_Ctor), 0x4c4cdc (GuiButton_SetIcons)
func NewButton(x, y int, msg Msg, up, down, hover *gfx.Sprite) *Button {
	b := &Button{X: x, Y: y, Icons: [4]*gfx.Sprite{up, down, hover}}
	b.Msg = msg
	b.rect = b.Rect
	b.onEnter = b.enter
	b.onLeave = b.leave
	return b
}

// Rect is x, y and the size of the first non-empty icon.
//
// mm8: 0x4c4dc8 (GuiButton_GetRect)
func (b *Button) Rect() image.Rectangle {
	for _, s := range b.Icons {
		if s != nil {
			return image.Rect(b.X, b.Y, b.X+s.W, b.Y+s.H)
		}
	}
	return image.Rectangle{}
}

// mm8: 0x4c4b47 (GuiIconBase_OnMouseEnter), 0x4c4fef (GuiButton_OnMouseEnter)
func (b *Button) enter(q *MsgQueue) {
	if b.disabled {
		return
	}
	if b.lbuttonDown {
		b.State = StateDown
	} else {
		b.State = StateHover
	}
}

// mm8: 0x4c4b82 (GuiIconBase_OnMouseLeave)
func (b *Button) leave() {
	if b.State == StateDown || b.State == StateHover {
		b.State = StateUp
	}
}

// OnLButtonDown shows the pressed icon when the press lands on the button.
//
// mm8: 0x4c4aea (GuiIconBase_OnLButtonDown)
func (b *Button) OnLButtonDown(x, y int, q *MsgQueue) bool {
	if !b.widget.OnLButtonDown(x, y, q) {
		return false
	}
	b.State = StateDown
	return true
}

// OnLButtonUp releases the pressed icon (to "up", even while still hovered, as in the
// original) and activates on a release inside.
//
// mm8: 0x4c4b18 (GuiIconBase_OnLButtonUp)
func (b *Button) OnLButtonUp(x, y int, q *MsgQueue) bool {
	if b.State == StateDown {
		b.State = StateUp
	}
	return b.widget.OnLButtonUp(x, y, q)
}

// OnKey: a hotkey shows the pressed icon and activates.
//
// mm8: 0x4c4bdc (GuiIconBase_OnHotkey)
func (b *Button) OnKey(k Key, q *MsgQueue) bool {
	if !b.widget.OnKey(k, q) {
		return false
	}
	b.State = StateDown
	return true
}

// SetDisabled switches to the disabled icon (or back to up).
//
// mm8: 0x4c4bab (GuiIconBase_Enable), 0x4c4bc2 (GuiIconBase_Disable)
func (b *Button) SetDisabled(d bool) {
	b.disabled = d
	if d {
		b.State = StateDisabled
	} else {
		b.State = StateUp
	}
}

// Draw draws the icon for the current state, falling back to the "up" icon.
//
// mm8: 0x4c4e90 (GuiButton_Draw)
func (b *Button) Draw(c *gfx.Canvas) {
	s := b.Icons[b.State]
	if s == nil {
		s = b.Icons[StateUp]
	}
	if b.Keyed {
		c.BlitKeyed(s, b.X, b.Y)
	} else {
		c.Blit(s, b.X, b.Y)
	}
}

// Label alignments.
const (
	AlignClipped = 0 // one line, truncated (not used by the M2 screens)
	AlignCenter  = 1 // wrapped, centred lines
	AlignLeft    = 2 // left, unwrapped (the original passes y+w as maxY)
)

// Label is a text box (GuiLabel).
type Label struct {
	widget
	R      text.Rect
	Font   *text.Font
	Text   string
	Color  gfx.Color16
	Shadow gfx.Color16
	Align  int
}

// NewLabel makes a white, black-shadowed, left-aligned label.
//
// mm8: 0x4c504e (GuiLabel_Ctor), 0x4c520a (GuiLabel_SetRect)
func NewLabel(r text.Rect, f *text.Font, s string) *Label {
	l := &Label{R: r, Font: f, Text: s, Color: gfx.RGB16(0xff, 0xff, 0xff), Align: AlignLeft}
	l.rect = l.Rect
	return l
}

// Rect is the label box.
func (l *Label) Rect() image.Rectangle {
	return image.Rect(l.R.X, l.R.Y, l.R.X+l.R.W, l.R.Y+l.R.H)
}

// Draw draws the text.
//
// mm8: 0x4c542a (GuiLabel_DrawText), 0x4c6d5b (GuiFont_DrawCentered), 0x4c6da9 (GuiFont_DrawLeft)
func (l *Label) Draw(c *gfx.Canvas) {
	if l.Text == "" {
		return
	}
	switch l.Align {
	case AlignCenter:
		l.Font.DrawCentered(c, l.R, 0, 0, l.Color, l.Text, 3)
	default:
		// GuiFont_DrawLeft passes rect.y + rect.w as maxY (sic), which also disables
		// wrapping.
		l.Font.Draw(c, l.R, 0, 0, l.Color, l.Shadow, l.Text, l.R.Y+l.R.W)
	}
}

// Edit is a single-line text entry. The original GuiEdit (0x4c57c6) is not yet fully
// reverse-engineered; this keeps its observable behaviour: typed characters the font can
// draw are appended up to MaxLen, Backspace deletes, a caret blinks after the text.
type Edit struct {
	Label
	MaxLen int
	ticks  int
}

// NewEdit makes an edit box.
func NewEdit(r text.Rect, f *text.Font, s string, maxLen int) *Edit {
	e := &Edit{Label: *NewLabel(r, f, s), MaxLen: maxLen}
	e.rect = e.Rect
	return e
}

// Type applies one tick of typing.
func (e *Edit) Type(in *Input) {
	e.ticks++
	b := []byte(e.Text)
	for _, r := range in.Runes {
		if r < 0x100 && r >= 0x20 && e.Font.IsPrintable(byte(r)) && e.Font.Metrics[byte(r)].Width > 0 && len(b) < e.MaxLen {
			b = append(b, byte(r))
		}
	}
	if in.Pressed(KeyBackspace) && len(b) > 0 {
		b = b[:len(b)-1]
	}
	e.Text = string(b)
}

// Draw draws the text and a caret that blinks twice a second.
func (e *Edit) Draw(c *gfx.Canvas) {
	e.Label.Draw(c)
	if (e.ticks/15)%2 == 0 {
		x := e.R.X + e.Font.TextWidth(e.Text)
		e.Font.Draw(c, text.Rect{X: x, Y: e.R.Y, W: e.R.W, H: e.R.H}, 0, 0, e.Color, e.Shadow, "_", e.R.Y+e.R.W)
	}
}

// Hotspot is an invisible clickable rectangle (GuiHotspot).
type Hotspot struct {
	widget
	R image.Rectangle
}

// NewHotspot makes a hotspot over x, y, w, h.
//
// mm8: 0x4c47ca (GuiHotspot_Ctor)
func NewHotspot(x, y, w, h int, msg Msg) *Hotspot {
	s := &Hotspot{R: image.Rect(x, y, x+w, y+h)}
	s.Msg = msg
	s.rect = func() image.Rectangle { return s.R }
	return s
}

// Draw draws nothing.
func (s *Hotspot) Draw(*gfx.Canvas) {}

// Container holds a screen's widgets and routes input to them.
type Container struct {
	Children []Widget
	hover    Widget
	Queue    MsgQueue
}

// Add appends w. Children draw in the order added and get events in the reverse order:
// the last added first (the original's circular child list is walked from its tail).
//
// mm8: 0x4c4444 (GuiContainer_AddChild -> 0x4c68b7 appends at the tail)
func (ct *Container) Add(w Widget) { ct.Children = append(ct.Children, w) }

// dispatch offers an event to the hovered widget, then to every child from the last
// added back, until one consumes it.
//
// mm8: 0x4c3fde, 0x4c40de, 0x4c41de (GuiContainer_OnLButtonDown/Up, OnMouseMove: from
// childHead->prev, following prev)
func (ct *Container) dispatch(f func(Widget) bool) {
	if ct.hover != nil && f(ct.hover) {
		return
	}
	for i := len(ct.Children) - 1; i >= 0; i-- {
		if f(ct.Children[i]) {
			return
		}
	}
}

// Update feeds one tick of input to the widgets. Messages land in ct.Queue.
func (ct *Container) Update(in *Input) {
	q := &ct.Queue
	ct.dispatch(func(w Widget) bool { return w.OnMouseMove(in.X, in.Y, q) })
	ct.hover = nil
	for i := len(ct.Children) - 1; i >= 0; i-- {
		if w := ct.Children[i]; w.base().hovered {
			ct.hover = w
			break
		}
	}
	if in.LeftPressed {
		ct.dispatch(func(w Widget) bool { return w.OnLButtonDown(in.X, in.Y, q) })
	}
	if in.LeftReleased {
		ct.dispatch(func(w Widget) bool { return w.OnLButtonUp(in.X, in.Y, q) })
	}
	// mm8: 0x4c4338 (GuiContainer_OnKey: the children from the last added back)
	for _, k := range in.Keys {
		for i := len(ct.Children) - 1; i >= 0; i-- {
			if ct.Children[i].OnKey(k, q) {
				break
			}
		}
	}
}

// Draw draws the children in order.
//
// mm8: 0x4c731c (GuiScreen_DrawChildren)
func (ct *Container) Draw(c *gfx.Canvas) {
	for _, w := range ct.Children {
		w.Draw(c)
	}
}
