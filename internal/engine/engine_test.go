package engine

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"libre-enroth/internal/game/ui"
)

func TestMapKey(t *testing.T) {
	for k, want := range map[ebiten.Key]ui.Key{
		ebiten.KeyA: 'A', ebiten.KeyQ: 'Q', ebiten.KeyDigit5: '5',
		ebiten.KeyEscape: ui.KeyEscape, ebiten.KeyNumpadEnter: ui.KeyEnter, ebiten.KeyBackspace: ui.KeyBackspace,
	} {
		if got, ok := mapKey(k); !ok || got != want {
			t.Errorf("mapKey(%v) = %v, %v; want %v", k, got, ok, want)
		}
	}
	if _, ok := mapKey(ebiten.KeyF1); ok {
		t.Error("F1 mapped")
	}
}

func TestRepeatTick(t *testing.T) {
	var fired []int
	for d := 0; d <= 40; d++ {
		if repeatTick(d) {
			fired = append(fired, d)
		}
	}
	want := []int{1, 30, 33, 36, 39}
	if len(fired) != len(want) {
		t.Fatalf("fired at %v, want %v", fired, want)
	}
	for i := range want {
		if fired[i] != want[i] {
			t.Fatalf("fired at %v, want %v", fired, want)
		}
	}
}

func TestParseFilter(t *testing.T) {
	for s, want := range map[string]Filter{"sharp": FilterSharp, "nearest": FilterNearest, "linear": FilterLinear} {
		if got, err := ParseFilter(s); err != nil || got != want {
			t.Errorf("ParseFilter(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseFilter("bicubic"); err == nil {
		t.Error("bicubic accepted")
	}
}
