package world

import (
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/ui"
)

// Status receives the timed status line texts (Status_SetTimed 0x44a847); the UI
// implements it. Timed is the message up now ("" for none).
type Status interface {
	Show(text string, seconds int)
	Timed() string
}

// Session is the game state that outlives a map: the party and its clock, the random
// generator and the tables the members' code needs, and where messages go. One session
// carries the party from map to map.
type Session struct {
	Party  *party.Members
	Ctx    *party.Ctx
	Status Status
	// Global returns global.txt string i ("" when unknown).
	Global func(i int) string
	// Log receives the notes about what later milestones do (default log.Printf).
	Log func(format string, args ...any)

	// worlds are the maps visited this session, kept as they were left (their map
	// variables, doors, decorations and timers' last visit); saves are M10's.
	worlds  map[string]*World
	arrival *Arrival     // where a MoveToMap puts the party on the next map
	carry   *party.Party // the party leaving the last map (its buffs carry over)
	stubbed map[[2]int]bool
	// timerScan is the game time of the last timer scan (0x587db0, Timers.Scan).
	timerScan clock.Time
}

type noStatus struct{}

func (noStatus) Show(string, int) {}
func (noStatus) Timed() string    { return "" }

// NewSession is a session for a fresh one-member party (tests and tools).
func NewSession() *Session {
	s := &Session{
		Party: &party.Members{Players: []party.Player{{}}},
		Ctx:   &party.Ctx{Rand: party.NewRand(1)},
	}
	s.Party.NewGame(s.Ctx.Rand)
	return s
}

func (s *Session) status() Status {
	if s.Status == nil {
		return noStatus{}
	}
	return s.Status
}

func (s *Session) global(i int) string {
	if s.Global == nil {
		return ""
	}
	return s.Global(i)
}

// SetTimeOfDay puts the clock at hour:minute of the current day (the -time flag).
func (s *Session) SetTimeOfDay(hour, minute int) {
	m := s.Party
	day := m.Time / clock.Day * clock.Day
	m.Time = day + clock.Time(hour)*clock.Hour + clock.Time(minute)*clock.Minute
	m.LastRegen = m.Time
	m.Calendar = m.Time.Calendar()
}

// UISession is the session over the UI's party, RNG, status line and global.txt.
func UISession(r *ui.Resources) (*Session, error) {
	ctx, err := r.Ctx()
	if err != nil {
		return nil, err
	}
	return &Session{Party: r.Party, Ctx: ctx, Status: r.Status, Global: r.GlobalText}, nil
}
