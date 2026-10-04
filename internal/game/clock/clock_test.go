package clock

import "testing"

// TestNewGame: 0x21c00 is 9:00 AM on the first day of the first month of 1172.
//
// mm8: 0x49228d (Party_InitNewGame), 0x494007 (Party_UpdateTime)
func TestNewGame(t *testing.T) {
	c := NewGame.Calendar()
	want := Calendar{Year: 1172, Hour: 9}
	if c != want || NewGame != 0x21c00 {
		t.Errorf("NewGame %#x = %+v, want %+v", int64(NewGame), c, want)
	}
	if c.Hour12() != 9 || c.PM() || c.Weekday() != 0 || c.DayOfYear() != 1 {
		t.Errorf("hour12 %d pm %v weekday %d day %d", c.Hour12(), c.PM(), c.Weekday(), c.DayOfYear())
	}
}

// TestUnits: 256 ticks a minute, and the calendar rolls over at every boundary.
func TestUnits(t *testing.T) {
	if Minute != 0x100 || Hour != 0x3c00 || Day != 0x5a000 || Week != 0x276000 {
		t.Fatalf("units %#x %#x %#x %#x", Minute, Hour, Day, Week)
	}
	for _, c := range []struct {
		t    Time
		want Calendar
	}{
		{0, Calendar{Year: 1172}},
		{Minute - 1, Calendar{Year: 1172, Second: 59}}, // 255 * 15 / 64 = 59.77
		{Minute, Calendar{Year: 1172, Minute: 1}},
		{Hour - 1, Calendar{Year: 1172, Minute: 59, Second: 59}},
		{Hour, Calendar{Year: 1172, Hour: 1}},
		{Day - 1, Calendar{Year: 1172, Hour: 23, Minute: 59, Second: 59}},
		{Day, Calendar{Year: 1172, Day: 1}},
		{Week, Calendar{Year: 1172, Week: 1, Day: 7}},
		{Month - 1, Calendar{Year: 1172, Week: 3, Day: 27, Hour: 23, Minute: 59, Second: 59}},
		{Month, Calendar{Year: 1172, Month: 1}},
		{Year - 1, Calendar{Year: 1172, Month: 11, Week: 3, Day: 27, Hour: 23, Minute: 59, Second: 59}},
		{Year + 3*Day + 13*Hour + 5*Minute, Calendar{Year: 1173, Day: 3, Hour: 13, Minute: 5}},
	} {
		if got := c.t.Calendar(); got != c.want {
			t.Errorf("%#x: %+v, want %+v", int64(c.t), got, c.want)
		}
	}
}

func TestHour12(t *testing.T) {
	for h, want := range []int{12, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11} {
		c := Calendar{Hour: h}
		if c.Hour12() != want || c.PM() != (h >= 12) {
			t.Errorf("hour %d: %d pm %v", h, c.Hour12(), c.PM())
		}
	}
}

// TestHoursTo5AM: mm8 0x4948f7.
func TestHoursTo5AM(t *testing.T) {
	want := []int{5, 4, 3, 2, 1, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6}
	for h, w := range want {
		if got := HoursTo5AM(h); got != w {
			t.Errorf("HoursTo5AM(%d) = %d, want %d", h, got, w)
		}
	}
}

// TestSeasons: CheckSeason (0x4443f5) splits at day 21 of months 3, 6, 9 and 12.
func TestSeasons(t *testing.T) {
	season := func(month, day int) int {
		c := Calendar{Month: month - 1, Day: day - 1}
		n, s := 0, -1
		for i := Spring; i <= Winter; i++ {
			if c.InSeason(i) {
				n, s = n+1, i
			}
		}
		if n != 1 {
			t.Errorf("%d/%d is in %d seasons", day, month, n)
		}
		return s
	}
	for _, c := range []struct{ month, day, want int }{
		{1, 1, Winter}, {3, 20, Winter}, {3, 21, Spring}, {6, 20, Spring}, {6, 21, Summer},
		{9, 20, Summer}, {9, 21, Autumn}, {12, 20, Autumn}, {12, 21, Winter}, {12, 28, Winter},
		{4, 1, Spring}, {7, 28, Summer}, {11, 5, Autumn}, {2, 14, Winter},
	} {
		if got := season(c.month, c.day); got != c.want {
			t.Errorf("%d/%d: season %d, want %d", c.day, c.month, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	n := LoadNames(func(i int) string {
		switch i {
		case 0x1d8:
			return "am"
		case 0x1d9:
			return "pm"
		case 0x91:
			return "Monday"
		case 0xe6:
			return "Tuesday"
		case 0x19f:
			return "January"
		case 0x1a0:
			return "February"
		}
		return "?"
	})
	if got := n.Format(NewGame.Calendar()); got != "9:00am Monday 1 January 1172" {
		t.Errorf("Format = %q", got)
	}
	c := (Month + Week + Day + 15*Hour + 7*Minute).Calendar()
	if got := n.Format(c); got != "3:07pm Tuesday 9 February 1172" {
		t.Errorf("Format = %q", got)
	}
}
