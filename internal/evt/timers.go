package evt

import "libre-enroth/internal/game/clock"

// Timer is a registered OnTimer/OnLongTimer trigger (0x20 bytes at 0x5cbf78).
type Timer struct {
	Next      clock.Time // +0x00: when a calendar timer fires next
	ID, Step  int        // +0x08, +0x0a
	Countdown int        // +0x0c: steps of 30 game seconds left (0: a calendar timer)
	Reload    int        // +0x0e
	Yearly    bool       // +0x10
	Monthly   bool       // +0x12
	Weekly    bool       // +0x14
	Hour      int        // +0x16: time of day of a daily timer
	Minute    int        // +0x18
	Second    int        // +0x1a
	Op        Op         // +0x1c
}

// Timers are a map's timers.
type Timers struct {
	List []Timer
}

// ftolThirtieth is ftol(x * (float)(1/30)) on the x87: the float is 0x888889 * 2^-28
// and the product exact.
func ftolThirtieth(x int64) int64 {
	p := x * 0x888889
	if p < 0 {
		return -((-p) >> 28)
	}
	return p >> 28
}

// Period lengths in seconds.
const (
	secMinute = 60
	secHour   = 0xe10
	secDay    = 0x15180
	secWeek   = 0x93a80
	secMonth  = 0x24ea00
	secYear   = 0x1baf800
)

// InitTimers registers the OnTimer (0x1f) and OnLongTimer (0x26) triggers of a map. A
// countdown timer counts its interval in 30-second steps; a calendar one fires at the
// next boundary of its period: next year, month or week at the same date and time, or
// today at its time of day (which may have passed: it then fires on the first scan).
// An OnLongTimer without an interval fires at once when a period has passed since the
// last visit (lastVisit, 0 = never), else waits for the boundary.
//
// mm8: 0x441caf (Evt_InitTimers)
func InitTimers(s *Script, now, lastVisit clock.Time) *Timers {
	t := &Timers{}
	if s == nil {
		return t
	}
	for _, rec := range s.Records {
		op := rec.Op()
		if op != OpOnTimer && op != OpOnLongTimer {
			continue
		}
		interval := rec.U16(0xb)
		tm := Timer{
			ID: rec.ID(), Step: rec.Step(), Countdown: interval, Reload: interval,
			Yearly: rec.U8(5) != 0, Monthly: rec.U8(6) != 0, Weekly: rec.U8(7) != 0,
			Hour: rec.U8(8), Minute: rec.U8(9), Second: rec.U8(0xa), Op: op,
		}
		boundary := true
		if op == OpOnLongTimer && interval == 0 {
			var elapsed clock.Time
			if lastVisit != 0 {
				elapsed = now - lastVisit
			}
			days := elapsed.Seconds() / 60 / 60 / 24
			weeks := days / 7
			months := weeks / 4
			years := months / 12
			if !((years == 0 || !tm.Yearly) && (months == 0 || !tm.Monthly) && (weeks == 0 || !tm.Weekly) &&
				days == 0 && lastVisit != 0) {
				boundary = false
				tm.Next = 0
			}
		}
		if boundary {
			tm.Next = tm.boundary(now)
		}
		t.List = append(t.List, tm)
	}
	return t
}

// boundary is the next fire time of a calendar timer from now.
func (tm *Timer) boundary(now clock.Time) clock.Time {
	s := now.Seconds()
	sec, min := s%60, s/60%60
	hours := s / 3600
	days := hours / 24
	weeks := days / 7
	months := weeks / 4
	years := months / 12
	h, dow, w, mo := hours%24, days%7, weeks%4, months%12
	switch {
	case tm.Yearly:
		years++
	case tm.Monthly:
		mo++
	case tm.Weekly:
		w++
	default:
		h, min, sec = int64(tm.Hour), int64(tm.Minute), int64(tm.Second)
	}
	total := (years*12+mo)*secMonth + dow*secDay + w*secWeek + h*secHour + min*secMinute + sec
	return clock.Time(ftolThirtieth(total * 0x80))
}

// period is a calendar timer's period in seconds.
func (tm *Timer) period() int64 {
	switch {
	case tm.Yearly:
		return secYear
	case tm.Monthly:
		return secMonth
	case tm.Weekly:
		return secWeek
	}
	return secDay
}

// Scan fires the timers that are due, once per 30 game seconds (128 ticks) that passed
// since the last scan, *last: a countdown timer counts down by the steps that passed
// and fires when it reaches 0 (then starts over), a calendar timer fires when its time
// has come and moves on by its period (to now, if that is still behind). Game time
// does not move while the timer is paused or stopped, so neither do timers.
//
// The last scan's time is one global (0x587db0) that only the scan writes: it is 0
// when the program starts, so the first scan of a game counts down every countdown
// timer by the whole game time (they all fire), and it carries over map changes.
//
// mm8: 0x446fb5 (Evt_TimerScan; the date timers of g_game+0xe74 are M10's, none ship)
func (t *Timers) Scan(now clock.Time, last *clock.Time, fire func(id, step int)) {
	q := int64(now-*last) >> 7
	if q == 0 {
		return
	}
	*last = now
	for i := range t.List {
		tm := &t.List[i]
		if tm.Countdown == 0 {
			if tm.Next > now {
				continue
			}
			tm.Next += clock.Time(ftolThirtieth(tm.period() * 0x80))
			if tm.Next < now {
				tm.Next = now
			}
		} else if cd := int16(tm.Countdown); uint32(q) < uint32(int32(cd)) {
			tm.Countdown = int(cd - int16(q))
			continue
		} else {
			tm.Countdown = tm.Reload
		}
		fire(tm.ID, tm.Step)
	}
}
