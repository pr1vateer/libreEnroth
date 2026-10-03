package delta

import "testing"

func TestDoorSettle(t *testing.T) {
	for _, c := range []struct {
		d    Door
		want int32
	}{
		{Door{State: DoorOpen, MoveLength: 300, OpenSpeed: 50, CloseSpeed: 50}, 0},
		{Door{State: DoorClosed, MoveLength: 300, OpenSpeed: 50, CloseSpeed: 50}, 300},
		{Door{State: DoorOpen, Attr: DoorStartClosed, MoveLength: 300, CloseSpeed: 50}, 300},
		// A slow door does not finish within the 0x3c00 ticks Level_Load grants it.
		{Door{State: DoorClosed, MoveLength: 3000, CloseSpeed: 1}, 0x3c00 / 128},
		{Door{State: DoorOpen, MoveLength: 3000, OpenSpeed: 1}, 3000 - 0x3c00/128},
		// Doors saved mid-move keep their own time.
		{Door{State: DoorClosing, Time: 256, MoveLength: 300, CloseSpeed: 10}, 20},
		{Door{State: DoorOpening, Time: 256, MoveLength: 300, OpenSpeed: 10}, 280},
	} {
		d := c.d
		d.Settle()
		if got, _ := d.Distance(); got != c.want {
			t.Errorf("%+v: settled distance %d, want %d", c.d, got, c.want)
		}
	}
}
