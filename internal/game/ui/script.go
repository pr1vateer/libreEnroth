package ui

import (
	"fmt"
	"image"
	"strconv"
	"strings"
)

// Script is a fixed sequence of held keys for headless runs: each step holds its keys
// for a number of 60 Hz ticks, pressing them on the step's first tick.
type Script struct {
	steps []scriptStep
	tick  int
}

type scriptStep struct {
	keys  []Key
	ticks int
	mouse *image.Point // the mouse stays here for the step
	click bool         // the left button goes down on the first tick, up on the last
}

// scriptKeys names the non-character keys a script may use.
var scriptKeys = map[string]Key{
	"up": KeyUp, "down": KeyDown, "left": KeyLeft, "right": KeyRight,
	"shift": KeyShift, "ctrl": KeyControl, "pgup": KeyPageUp, "pgdn": KeyPageDown,
	"home": KeyHome, "end": KeyEnd, "insert": KeyInsert, "delete": KeyDelete,
	"space": KeySpace, "enter": KeyEnter, "esc": KeyEscape, "tab": KeyTab,
	"f1": KeyF1, "f2": KeyF2, "f3": KeyF3, "f4": KeyF4, "f5": KeyF5,
	"[": KeyBracketLeft, "]": KeyBracketRight,
}

// ParseScript parses steps "keys:ticks" separated by commas, where keys are key names
// joined by '+' (letters, digits, Up, Shift, PgUp, F3, '[', ...) or '-' for none:
// "Up:120,X:1,Right+Shift:30,-:60". "Mouse@x/y" puts the mouse at UI point (x, y) for
// the step and "Click@x/y" also clicks the left button there: "Click@320/200:2".
func ParseScript(s string) (*Script, error) {
	sc := &Script{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		i := strings.LastIndex(part, ":")
		if i < 0 {
			return nil, fmt.Errorf("script step %q: want keys:ticks", part)
		}
		n, err := strconv.Atoi(part[i+1:])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("script step %q: bad tick count", part)
		}
		st := scriptStep{ticks: n}
		if names := part[:i]; names != "-" {
			for _, name := range strings.Split(names, "+") {
				if verb, at, found := strings.Cut(name, "@"); found {
					var p image.Point
					if _, err := fmt.Sscanf(at, "%d/%d", &p.X, &p.Y); err != nil {
						return nil, fmt.Errorf("script step %q: want %s@x/y", part, verb)
					}
					switch strings.ToLower(verb) {
					case "mouse":
					case "click":
						st.click = true
					default:
						return nil, fmt.Errorf("script step %q: unknown action %q", part, verb)
					}
					st.mouse = &p
					continue
				}
				k, ok := scriptKeys[strings.ToLower(name)]
				if !ok && len(name) == 1 {
					c := strings.ToUpper(name)[0]
					if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
						k, ok = Key(c), true
					}
				}
				if !ok {
					return nil, fmt.Errorf("script step %q: unknown key %q", part, name)
				}
				st.keys = append(st.keys, k)
			}
		}
		sc.steps = append(sc.steps, st)
	}
	return sc, nil
}

// Len is the script's length in ticks.
func (s *Script) Len() int {
	n := 0
	for _, st := range s.steps {
		n += st.ticks
	}
	return n
}

// Done reports whether every step has run.
func (s *Script) Done() bool { return s.tick >= s.Len() }

// Next adds the current tick's keys to in and advances the script.
func (s *Script) Next(in *Input) {
	t := s.tick
	for _, st := range s.steps {
		if t < st.ticks {
			in.Held = append(in.Held, st.keys...)
			if t == 0 {
				in.Keys = append(in.Keys, st.keys...)
			}
			if st.mouse != nil {
				in.X, in.Y = st.mouse.X, st.mouse.Y
			}
			if st.click {
				in.Left = true
				in.LeftPressed = in.LeftPressed || t == 0
				in.LeftReleased = in.LeftReleased || t == st.ticks-1
			}
			break
		}
		t -= st.ticks
	}
	s.tick++
}
