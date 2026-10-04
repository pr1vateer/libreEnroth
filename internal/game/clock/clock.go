// Package clock is the game calendar: game time in timer ticks (Party +0x2c) and the
// calendar fields Party_UpdateTime derives from it (re/notes/time.md).
//
// A tick is 30/128 of a game second: the 128 Hz timer runs game time 30 times faster
// than real time. A month is 4 weeks of 7 days and a year 12 months, so every month
// has 28 days and starts on the first weekday.
package clock

import "fmt"

// Time is game time in timer ticks.
type Time int64

// Units of game time.
const (
	Minute Time = 0x100
	Hour   Time = 60 * Minute // 0x3c00
	Day    Time = 24 * Hour   // 0x5a000
	Week   Time = 7 * Day     // 0x276000
	Month  Time = 4 * Week
	Year   Time = 12 * Month
)

// NewGame is the time a new game starts at: 9:00 AM on day 1 of month 1, 1172.
//
// mm8: 0x49228d (Party_InitNewGame: gameTime = lastRegen = 0x21c00)
const NewGame Time = 9 * Hour

// BaseYear is the year of game time 0.
const BaseYear = 1172

// Seconds is the game time in whole seconds: ftol(t * 0.234375), truncated towards
// zero (0.234375 = 30/128 is exact in binary, so the integer form is identical).
func (t Time) Seconds() int64 { return int64(t) * 15 / 64 }

// Minutes is the game time in whole minutes, from the 32-bit seconds the regeneration
// cadence uses.
//
// mm8: 0x493a34 (Party_Regen: (int)ftol(t * 0.234375) / 60)
func (t Time) Minutes() int32 { return int32(t.Seconds()) / 60 }

// Calendar is the date and time of day. Every field counts from 0 except Year; the
// game shows Month+1 and Day+1.
type Calendar struct {
	Year   int // +0x71c
	Month  int // +0x720 0..11
	Week   int // +0x724 week of the month 0..3
	Day    int // +0x728 day of the month 0..27
	Hour   int // +0x72c 0..23
	Minute int // +0x730
	Second int // +0x734
}

// Calendar splits t into the calendar fields with the original's divide chain; the
// day count is taken from the low 32 bits of the hours.
//
// mm8: 0x494007 (Party_UpdateTime)
func (t Time) Calendar() Calendar {
	s := t.Seconds()
	m := s / 60
	h := m / 60
	d := uint32(h) / 24
	w := d / 7
	mo := w >> 2
	return Calendar{
		Second: int(s % 60),
		Minute: int(m % 60),
		Hour:   int(h % 24),
		Day:    int(d % 28),
		Week:   int(w & 3),
		Month:  int(mo % 12),
		Year:   int(mo/12) + BaseYear,
	}
}

// Weekday is the day of the week, 0..6: every month starts on day 0.
func (c Calendar) Weekday() int { return c.Day % 7 }

// DayOfYear is the day of the year, 1..336.
//
// mm8: 0x4483d3 (Evt_Compare var 0x13)
func (c Calendar) DayOfYear() int { return c.Month*28 + c.Day + 1 }

// Hour12 is the hour on a 12-hour clock: 0 shows as 12.
//
// mm8: 0x42f877 (msg 0x5c), 0x41f71e (Rest_Draw)
func (c Calendar) Hour12() int {
	switch {
	case c.Hour > 12:
		return c.Hour - 12
	case c.Hour == 0:
		return 12
	}
	return c.Hour
}

// PM reports the afternoon (12:00 to 23:59).
func (c Calendar) PM() bool { return c.Hour >= 12 && c.Hour <= 23 }

// Names are the calendar strings from global.txt.
type Names struct {
	AM, PM   string
	Weekdays [7]string
	Months   [12]string
}

// Global.txt indices of the calendar strings.
//
// mm8: 0x45181f (Txt_LoadGlobal: AM/PM 0x5db840, weekdays 0x5191f0, months 0x5191c0)
var (
	globalAMPM     = [2]int{0x1d8, 0x1d9}
	globalWeekdays = [7]int{0x91, 0xe6, 0xf3, 0xe3, 0x5b, 0xbc, 0xde}
	globalMonth0   = 0x19f
)

// LoadNames fetches the calendar strings with global (global.txt by index).
func LoadNames(global func(int) string) Names {
	var n Names
	n.AM, n.PM = global(globalAMPM[0]), global(globalAMPM[1])
	for i, g := range globalWeekdays {
		n.Weekdays[i] = global(g)
	}
	for i := range n.Months {
		n.Months[i] = global(globalMonth0 + i)
	}
	return n
}

// Meridiem is the AM or PM suffix of the hour.
func (n *Names) Meridiem(c Calendar) string {
	if c.PM() {
		return n.PM
	}
	return n.AM
}

// Format is the clock text the status line shows over the minimap (msg 0x5c):
// "9:00AM Monday 1 January 1172".
//
// mm8: 0x42f877 (msg 0x5c: "%d:%02d%s %s %d %s %d")
func (n *Names) Format(c Calendar) string {
	return fmt.Sprintf("%d:%02d%s %s %d %s %d", c.Hour12(), c.Minute, n.Meridiem(c),
		n.Weekdays[c.Weekday()], c.Week*7+1+c.Weekday(), n.Months[c.Month], c.Year)
}

// HoursTo5AM is the number of hours from hour h until the next 5 AM (5 at midnight, 24
// at 5 AM itself).
//
// mm8: 0x4948f7
func HoursTo5AM(h int) int {
	h %= 24
	if h <= 4 {
		return 5 - h
	}
	return 29 - h
}

// Seasons for CheckSeason.
const (
	Spring = iota
	Summer
	Autumn
	Winter
)

// InSeason reports whether the date falls in season s: spring runs from day 21 of
// month 3 to day 20 of month 6, summer to day 20 of month 9, autumn to day 20 of month
// 12 and winter to day 20 of month 3 (1-based months and days).
//
// mm8: 0x4443f5 (CheckSeason)
func (c Calendar) InSeason(s int) bool {
	m, d := c.Month+1, c.Day+1
	if s < Spring || s > Winter {
		return false
	}
	first := 3 * (s + 1) // the month the season starts in (12 for winter)
	last := first + 3
	switch {
	case m == first:
		return d > 20
	case s == Winter && m > 0 && m < 3, s != Winter && m > first && m < last:
		return true
	case s == Winter && m == 3, s != Winter && m == last:
		return d < 21
	}
	return false
}
