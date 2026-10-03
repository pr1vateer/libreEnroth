// Package engine runs the game on Ebitengine: it turns Ebiten input into ui.Input,
// steps the state machine at 60 TPS and presents the 640x480 UI canvas scaled to the
// render resolution.
package engine

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"libre-enroth/internal/display"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/render"
)

// Filter selects how the UI canvas is scaled.
type Filter int

const (
	FilterSharp   Filter = iota // sharp bilinear: crisp pixels without uneven widths
	FilterNearest               // plain nearest neighbour
	FilterLinear                // plain bilinear
)

// ParseFilter parses sharp, nearest or linear.
func ParseFilter(s string) (Filter, error) {
	switch s {
	case "sharp":
		return FilterSharp, nil
	case "nearest":
		return FilterNearest, nil
	case "linear":
		return FilterLinear, nil
	}
	return 0, fmt.Errorf("unknown filter %q (want sharp, nearest or linear)", s)
}

// Config configures a Game.
type Config struct {
	Res        display.Res
	Filter     Filter
	Screenshot string       // write the frame to this PNG after Frames frames, then quit
	Frames     int          // frames to render before the screenshot
	Mouse      *image.Point // fixed mouse position in UI pixels (screenshots)
	Script     *ui.Script   // keys to replay; the screenshot waits for its end
}

// Game implements ebiten.Game.
type Game struct {
	cfg    Config
	app    *ui.App
	canvas *gfx.Canvas
	uiImg  *ebiten.Image
	sharp  *ebiten.Shader
	tr     display.Transform
	frame  *render.Frame // the 3D view at render resolution
	view   *ebiten.Image
	frames int
	done   bool
	err    error
}

// New wraps a UI state machine.
func New(app *ui.App, cfg Config) (*Game, error) {
	sh, err := ebiten.NewShader(sharpBilinear)
	if err != nil {
		return nil, fmt.Errorf("sharp shader: %w", err)
	}
	g := &Game{
		cfg:    cfg,
		app:    app,
		canvas: gfx.NewCanvas(),
		uiImg:  ebiten.NewImage(gfx.ScreenW, gfx.ScreenH),
		sharp:  sh,
	}
	if !cfg.Res.Auto {
		g.tr = display.Fit(cfg.Res.W, cfg.Res.H)
	}
	return g, nil
}

// Layout returns the render resolution: the configured one, or the window size in
// device pixels for -res auto.
func (g *Game) Layout(outsideW, outsideH int) (int, int) {
	w, h := g.cfg.Res.W, g.cfg.Res.H
	if g.cfg.Res.Auto {
		s := ebiten.Monitor().DeviceScaleFactor()
		w, h = int(math.Ceil(float64(outsideW)*s)), int(math.Ceil(float64(outsideH)*s))
		w, h = max(w, 1), max(h, 1)
	}
	if g.tr.W != w || g.tr.H != h {
		g.tr = display.Fit(w, h)
	}
	return w, h
}

// Transform is the current UI placement (M3 sizes the 3D view with it).
func (g *Game) Transform() display.Transform { return g.tr }

// Update reads input and steps the UI.
func (g *Game) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.done {
		return ebiten.Termination
	}
	in := g.input()
	quit, err := g.app.Update(in)
	if err != nil {
		return err
	}
	if quit {
		return ebiten.Termination
	}
	return nil
}

func (g *Game) input() *ui.Input {
	in := &ui.Input{}
	if g.cfg.Mouse != nil {
		in.X, in.Y = g.cfg.Mouse.X, g.cfg.Mouse.Y
	} else {
		cx, cy := ebiten.CursorPosition()
		in.X, in.Y = g.tr.ToUI(float64(cx)+0.5, float64(cy)+0.5)
		// Software cursor inside the UI, the system one over the borders.
		inside := image.Pt(in.X, in.Y).In(image.Rectangle{Max: display.UI})
		g.app.ShowCursor = inside
		if inside {
			ebiten.SetCursorMode(ebiten.CursorModeHidden)
		} else {
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
	}
	in.Left = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	in.Right = ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
	in.LeftPressed = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	in.LeftReleased = inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	in.RightPressed = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	in.RightReleased = inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonRight)
	for _, k := range inpututil.AppendPressedKeys(nil) {
		if !repeatTick(inpututil.KeyPressDuration(k)) {
			continue
		}
		if uk, ok := mapKey(k); ok {
			in.Keys = append(in.Keys, uk)
		}
	}
	for _, k := range inpututil.AppendPressedKeys(nil) {
		if uk, ok := mapKey(k); ok {
			in.Held = append(in.Held, uk)
		}
	}
	in.Runes = ebiten.AppendInputChars(nil)
	if g.cfg.Script != nil {
		g.cfg.Script.Next(in)
	}
	return in
}

// repeatTick: a key fires when pressed and then, after half a second, 20 times a second.
func repeatTick(d int) bool {
	return d == 1 || (d >= 30 && (d-30)%3 == 0)
}

func mapKey(k ebiten.Key) (ui.Key, bool) {
	switch {
	case k >= ebiten.KeyA && k <= ebiten.KeyZ:
		return ui.Key('A' + (k - ebiten.KeyA)), true
	case k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9:
		return ui.Key('0' + (k - ebiten.KeyDigit0)), true
	}
	switch k {
	case ebiten.KeyEscape:
		return ui.KeyEscape, true
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		return ui.KeyEnter, true
	case ebiten.KeyBackspace:
		return ui.KeyBackspace, true
	case ebiten.KeyTab:
		return ui.KeyTab, true
	case ebiten.KeySpace:
		return ui.KeySpace, true
	case ebiten.KeyArrowLeft:
		return ui.KeyLeft, true
	case ebiten.KeyArrowRight:
		return ui.KeyRight, true
	case ebiten.KeyArrowUp:
		return ui.KeyUp, true
	case ebiten.KeyArrowDown:
		return ui.KeyDown, true
	case ebiten.KeyHome:
		return ui.KeyHome, true
	case ebiten.KeyEnd:
		return ui.KeyEnd, true
	case ebiten.KeyDelete:
		return ui.KeyDelete, true
	case ebiten.KeyPageUp:
		return ui.KeyPageUp, true
	case ebiten.KeyPageDown:
		return ui.KeyPageDown, true
	case ebiten.KeyShiftLeft, ebiten.KeyShiftRight:
		return ui.KeyShift, true
	case ebiten.KeyF1:
		return ui.KeyF1, true
	case ebiten.KeyF2:
		return ui.KeyF2, true
	case ebiten.KeyF3:
		return ui.KeyF3, true
	case ebiten.KeyF4:
		return ui.KeyF4, true
	case ebiten.KeyF5:
		return ui.KeyF5, true
	case ebiten.KeyInsert:
		return ui.KeyInsert, true
	case ebiten.KeyControlLeft, ebiten.KeyControlRight:
		return ui.KeyControl, true
	case ebiten.KeyBracketLeft:
		return ui.KeyBracketLeft, true
	case ebiten.KeyBracketRight:
		return ui.KeyBracketRight, true
	}
	return 0, false
}

// Draw renders the 3D view natively into the viewport rectangle, then lays the UI over
// it, scaled.
func (g *Game) Draw(screen *ebiten.Image) {
	g.app.Draw(g.canvas)
	g.uiImg.WritePixels(g.canvas.Img.Pix)
	screen.Fill(image.Black)
	g.drawWorld(screen)

	filter := g.cfg.Filter
	if g.tr.IntegerScale() && filter == FilterSharp {
		filter = FilterNearest // sharp bilinear degenerates to nearest at integer scales
	}
	var geo ebiten.GeoM
	geo.Scale(g.tr.Scale, g.tr.Scale)
	geo.Translate(float64(g.tr.OffX), float64(g.tr.OffY))
	if filter == FilterSharp {
		op := &ebiten.DrawRectShaderOptions{GeoM: geo}
		op.Images[0] = g.uiImg
		op.Uniforms = map[string]any{"Scale": float32(g.tr.Scale)}
		screen.DrawRectShader(gfx.ScreenW, gfx.ScreenH, g.sharp, op)
	} else {
		op := &ebiten.DrawImageOptions{GeoM: geo, Filter: ebiten.FilterNearest}
		if filter == FilterLinear {
			op.Filter = ebiten.FilterLinear
		}
		screen.DrawImage(g.uiImg, op)
	}

	g.frames++
	scripted := g.cfg.Script == nil || g.cfg.Script.Done()
	if g.cfg.Screenshot != "" && !g.done && scripted && g.frames >= max(g.cfg.Frames, 1) {
		g.done = true
		if err := writePNG(screen, g.cfg.Screenshot); err != nil {
			g.err = err
		}
	}
}

func (g *Game) drawWorld(screen *ebiten.Image) {
	w := g.app.World()
	if w == nil {
		return
	}
	rect := g.tr.UIRectToScreen(g.app.Viewport())
	if rect.Empty() {
		return
	}
	if g.frame == nil {
		g.frame = render.NewFrame(rect.Dx(), rect.Dy())
	}
	g.frame.Resize(rect.Dx(), rect.Dy())
	w.Render(g.frame)
	if g.view == nil || g.view.Bounds().Dx() != rect.Dx() || g.view.Bounds().Dy() != rect.Dy() {
		if g.view != nil {
			g.view.Deallocate()
		}
		g.view = ebiten.NewImage(rect.Dx(), rect.Dy())
	}
	g.view.WritePixels(g.frame.Bytes())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	screen.DrawImage(g.view, op)
}

func writePNG(screen *ebiten.Image, path string) error {
	b := screen.Bounds()
	img := image.NewRGBA(b)
	screen.ReadPixels(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Run opens the window and runs until quit. A nil error is returned on a normal exit.
func Run(g *Game, title string, windowW, windowH int, fullscreen bool) error {
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(fullscreen)
	ebiten.SetTPS(60)
	err := ebiten.RunGame(g)
	if errors.Is(err, ebiten.Termination) {
		return nil
	}
	return err
}
