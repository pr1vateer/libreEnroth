package ui

import (
	"fmt"
	"math"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// RestSite is a world the party can rest in (internal/game/world implements it).
type RestSite interface {
	// RestRefusal is the global.txt message that stops a rest here, 0 when none.
	RestRefusal() int
	// RestFood is the food a night's rest costs here.
	RestFood() int
	// RestEncounter rolls for monsters interrupting a night's rest.
	RestEncounter() bool
	// RestDone runs when a night's rest ends.
	RestDone()
}

// Rest screen messages (Rest_Build 0x41f370) and the in-game one that opens it.
const (
	msgRest      = 0x68 // butn2 / R: open the rest screen
	msgRestExit  = 0xa7
	msgRestHeal  = 0x61
	msgRestDawn  = 0x6d
	msgRestHour  = 0x60
	msgRestFive  = 0x5f
	txtRestBusy  = 0x1dd // "You are already resting!"
	txtNoFood    = 0x1e2 // "You don't have enough food to rest"
	txtAmbushed  = 0x1e1 // "Encounter!"
	restMinutes  = 480   // a night's rest
	restStepMins = 6     // per frame
)

// Rest modes (0x5184c0).
const (
	restIdle = iota
	restWaiting
	restResting
)

// Rest screen text colours and positions (Rest_Draw 0x41f71e).
var (
	restInk    = gfx.RGB16(0xe6, 0xd6, 0xc1)
	restShadow = gfx.RGB16(10, 0, 0)
)

// restScreen is the campfire screen over the game view: the sky at the current time,
// an hourglass, the food it costs and the date, and buttons to rest 8 hours, wait until
// dawn, an hour or five minutes. The game timer is paused while it is open; time moves
// 6 minutes a frame while resting or waiting.
type restScreen struct {
	ct         Container
	r          *Resources
	site       RestSite
	ctx        *party.Ctx
	bg         *gfx.Sprite
	font       *text.Font
	cost       int
	mode       int
	minutes    int // left to rest or wait (0x5184c4)
	hourglas   int // 0x519378: timer ticks into the hourglass turn
	subTicks   int
	sky, glass *gfx.Sprite
	closed     bool
}

// newRestScreen builds the rest screen (free: a rest that costs no food).
//
// mm8: 0x4d10e9 (Rest_BuildScreen), 0x41f370 (Rest_Build)
func newRestScreen(r *Resources, site RestSite, free bool) (*restScreen, error) {
	l := &loader{r: r}
	ctx, err := r.Ctx()
	if err != nil {
		return nil, err
	}
	s := &restScreen{r: r, site: site, ctx: ctx, bg: l.icon("restmain", true), font: l.font("create.fnt")}
	s.cost = site.RestFood()
	if free {
		s.cost = 0
	}
	for _, b := range []struct {
		x, y int
		msg  int
		icon string
		key  Key
	}{
		{0x16a, 300, msgRestExit, "Rexit", KeyEscape},
		{0x3c, 0x9c, msgRestHeal, "R8h", 'R'},
		{99, 0xf0, msgRestDawn, "Rdawn", 'D'},
		{99, 0x110, msgRestHour, "R1h", 'H'},
		{99, 0x130, msgRestFive, "R5m", 'M'},
	} {
		btn := l.button(b.x, b.y, Msg{ID: b.msg}, b.icon+"U", b.icon+"D", b.icon+"H", true)
		btn.Hotkey = b.key
		s.ct.Add(btn)
	}
	r.Party.Resting = true
	s.loadSky(l)
	return s, l.err
}

// loadSky picks the sky picture for the time of day: TERRA0000..0243, one per 6 minutes.
//
// mm8: 0x41f1f9 (Rest_LoadSky)
func (s *restScreen) loadSky(l *loader) {
	c := s.r.Party.Calendar
	if i := c.Minute/6 + c.Hour*10; i >= 0 && i < 0xf4 {
		s.sky = l.icon(fmt.Sprintf("TERRA%04d", i), false)
	}
}

// update runs one tick: the buttons, then a rest step. It reports the screen closing.
//
// mm8: 0x42f877 (msgs 0x5f 0x60 0x61 0x6d 0xa7), 0x41f297 (Rest_Step from Rest_Draw)
func (s *restScreen) update(in *Input) (closed bool) {
	s.subTicks += 128
	ticks := s.subTicks / 60
	s.subTicks %= 60
	s.ct.Update(in)
	m := s.r.Party
	if !anyAlive(m) {
		s.close(ticks)
		return true
	}
	for _, msg := range s.ct.Queue.Drain() {
		switch msg.ID {
		case msgRestExit:
			s.close(ticks)
		case msgRestFive, msgRestHour, msgRestDawn:
			if s.mode == restResting {
				s.r.Status.Show(s.r.GlobalText(txtRestBusy), 2)
				continue
			}
			switch msg.ID {
			case msgRestFive:
				s.minutes = 5
			case msgRestHour:
				s.minutes = 60
			default:
				s.minutes = clock.HoursTo5AM(m.Calendar.Hour)*60 - m.Calendar.Minute
			}
			s.mode = restWaiting
		case msgRestHeal:
			s.restHeal(ticks)
		}
	}
	if s.closed {
		return true
	}
	s.hourglas += ticks
	if s.hourglas > 0x1ff {
		s.hourglas = 0
	}
	if s.mode != restIdle {
		s.step(ticks)
	}
	return s.closed
}

// restHeal starts a night's rest: everyone falls asleep, the map may send monsters
// (then the rest ends an hour or so in and one member wakes), else the food is eaten
// and 8 hours of healing sleep begin.
//
// mm8: 0x42f877 (msg 0x61)
func (s *restScreen) restHeal(ticks int) {
	m := s.r.Party
	if s.mode != restIdle {
		s.r.Status.Show(s.r.GlobalText(txtRestBusy), 2)
		return
	}
	if int(m.Food) < s.cost {
		s.r.Status.Show(s.r.GlobalText(txtNoFood), 2)
		return
	}
	sleep := func() {
		for i := range m.Players {
			m.Players[i].Conditions[party.CondAsleep] = int64(m.Time)
		}
	}
	sleep()
	if s.site.RestEncounter() {
		rng := s.ctx.Rand
		m.Players[rng.Int()%len(m.Players)].Conditions[party.CondAsleep] = 0
		m.AdvanceMinutes(rng.Int()%6+60, ticks, s.ctx)
		s.mode, s.minutes = restIdle, 0
		s.close(ticks)
		s.r.Status.Show(s.r.GlobalText(txtAmbushed), 2)
		return
	}
	m.EatFood(int32(s.cost))
	s.mode, s.minutes = restResting, restMinutes
	m.RestHeal(ticks, s.ctx)
	sleep()
}

// restAtInn starts the inn's night: everyone heals and sleeps until an hour past the
// next 5 AM; no food is eaten and no monsters come. (A night longer than 4 hours also
// resets the map's monsters, 0x40928c: M8.)
//
// mm8: 0x42f877 (msg 0x199)
func (s *restScreen) restAtInn(ticks int) {
	m := s.r.Party
	c := m.Calendar
	s.mode = restResting
	s.minutes = (clock.HoursTo5AM(c.Hour)+1)*60 - c.Minute
	m.RestHeal(ticks, s.ctx)
	for i := range m.Players {
		m.Players[i].Conditions[party.CondAsleep] = int64(m.Time)
	}
}

// step moves time on by 6 minutes, or what is left; at the end everyone wakes and a
// night's rest closes the screen.
//
// mm8: 0x41f297 (Rest_Step)
func (s *restScreen) step(ticks int) {
	m := s.r.Party
	l := &loader{r: s.r}
	if s.minutes >= restStepMins {
		m.AdvanceMinutes(restStepMins, ticks, s.ctx)
		s.minutes -= restStepMins
		s.loadSky(l)
		return
	}
	for i := range m.Players {
		m.Players[i].Conditions[party.CondAsleep] = 0
	}
	if s.minutes != 0 {
		m.AdvanceMinutes(s.minutes, ticks, s.ctx)
		s.minutes = 0
		s.loadSky(l)
	}
	if s.mode == restResting {
		s.ct.Queue.Post(Msg{ID: msgRestExit})
		s.site.RestDone()
	}
	s.mode = restIdle
}

// close leaves the screen: an unfinished rest or wait passes at once and everyone
// wakes; the game timer runs again.
//
// mm8: 0x4d112c (Rest_Close)
func (s *restScreen) close(ticks int) {
	m := s.r.Party
	if s.mode != restIdle {
		m.AdvanceMinutes(s.minutes, ticks, s.ctx)
		for i := range m.Players {
			m.Players[i].Conditions[party.CondAsleep] = 0
		}
	}
	s.mode, s.minutes = restIdle, 0
	m.Resting = false
	s.closed = true
}

// anyAlive reports a member who is neither dead nor eradicated and has HP above 0.
func anyAlive(m *party.Members) bool {
	for i := range m.Players {
		p := &m.Players[i]
		if p.Conditions[party.CondDead] == 0 && p.Conditions[party.CondEradicated] == 0 && p.HP > 0 {
			return true
		}
	}
	return false
}

// draw draws the background, the buttons, then the sky, the hourglass and the texts.
//
// mm8: 0x4d108c (Rest_Draw: background, children, foreground), 0x41f71e
func (s *restScreen) draw(c *gfx.Canvas) {
	c.Blit(s.bg, 0, 0)
	s.ct.Draw(c)
	m := s.r.Party
	if !anyAlive(m) {
		return
	}
	cal := m.Calendar
	if s.sky != nil {
		c.Blit(s.sky, 0x60, 0x19)
	}
	frame := int(math.RoundToEven(float64(float32(float32(s.hourglas)*0.001953125*120)))) + 1
	if frame > 0x77 {
		frame = 1
	}
	l := &loader{r: s.r}
	if g := l.icon(fmt.Sprintf("hglas%03d", frame), false); g != nil {
		c.Blit(g, 0x180, 0xa4)
	}
	full := text.Rect{W: gfx.ScreenW, H: gfx.ScreenH}
	draw := func(x, y int, msg string) { s.font.Draw(c, full, x, y, restInk, restShadow, msg, 0) }
	names := clock.LoadNames(s.r.GlobalText)
	draw(0, 0xa8, fmt.Sprintf("\r315%d", s.cost))
	draw(0x1e6, 0xa8, fmt.Sprintf("%d:%02d %s", cal.Hour12(), cal.Minute, names.Meridiem(cal)))
	draw(0x1d0, 0xbe, fmt.Sprintf("%s\r078%d", s.r.GlobalText(0x38), cal.Day+1))
	draw(0x1d0, 0xde, fmt.Sprintf("%s\r078%d", s.r.GlobalText(0x92), cal.Month+1))
	draw(0x1d0, 0xfe, fmt.Sprintf("%s\r078%d", s.r.GlobalText(0xf5), cal.Year))
}
