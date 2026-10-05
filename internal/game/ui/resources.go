package ui

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/desc"
	"libre-enroth/internal/assets/exe"
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/assets/txt"
	"libre-enroth/internal/assets/vid"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// Resources is what the screens load from: images, fonts, global.txt and the tables in
// MM8-Rel.exe.
type Resources struct {
	Cache  *gfx.Cache
	Exe    *exe.Image
	fonts  map[string]*text.Font
	global []string
	pft    desc.PFT

	// Party is the party the in-game screen shows. Party creation puts the hero in
	// slot 1; NewResources starts with one member with face 0.
	Party *party.Members
	// Rand is the game's rand(); NewResources seeds it like the MSVC runtime (1).
	Rand *party.Rand
	// Status is the status line above the portraits.
	Status *Status
	ctx    *party.Ctx

	// LoadWorld loads a map for the in-game screen; nil leaves the viewport empty.
	LoadWorld func(name string) (World, error)
	items     *tables.Items
	classes   *tables.Classes
	dialogArt *dialogArt
	vids      vid.Set
	vidErr    error
	noted     map[string]bool
	scrolls   []string
	awards    []tables.Award

	// StartMap is the map a new game begins on.
	//
	// mm8: 0x45f682 (Game_NewGame: "out01.odm")
	StartMap string
}

// NewResources wraps opened game data.
func NewResources(d *assets.Data) *Resources {
	return &Resources{
		Cache: gfx.NewCache(d), Exe: d.Exe, fonts: map[string]*text.Font{}, StartMap: "out01.odm",
		Party:  &party.Members{Players: []party.Player{{Class: ClassForFace(0)}}},
		Rand:   party.NewRand(1),
		Status: &Status{},
	}
}

// vaSpeech is the members' speech table (Player_Speak 0x4949b1).
const vaSpeech = 0x4ff670

// Ctx is what the members' time and condition code needs: the expression table, the
// speech table from MM8-Rel.exe and the current Rand. The item and class tables
// feed the stats, and the hooks are party.StatHooks (HP and spell points).
func (r *Resources) Ctx() (*party.Ctx, error) {
	if r.ctx == nil {
		pft, err := r.PFT()
		if err != nil {
			return nil, err
		}
		l := &loader{r: r}
		raw := l.exeBytes(vaSpeech, 8*party.SpeechEntries)
		if l.err != nil {
			return nil, l.err
		}
		sp := make(party.Speech, party.SpeechEntries)
		for i := range sp {
			copy(sp[i][:], raw[8*i:])
		}
		items, err := r.Items()
		if err != nil {
			return nil, err
		}
		cls, err := r.Classes()
		if err != nil {
			return nil, err
		}
		r.ctx = &party.Ctx{PFT: pft, Speech: sp, Items: items, Classes: cls}
		r.ctx.Hooks = party.StatHooks{M: r.Party, C: r.ctx}
	}
	r.ctx.Rand = r.Rand
	return r.ctx, nil
}

// GlobalText is Global without the error ("" when global.txt cannot be read).
func (r *Resources) GlobalText(i int) string {
	s, _ := r.Global(i)
	return s
}

// PFT is the portrait expression table, dpft.bin.
//
// mm8: 0x464974 (Game_Init -> Pft_LoadBin 0x494d46)
func (r *Resources) PFT() (desc.PFT, error) {
	if r.pft == nil {
		raw, err := r.Cache.Text("dpft.bin")
		if err != nil {
			return nil, err
		}
		if r.pft, err = desc.ParsePFT(raw); err != nil {
			return nil, err
		}
	}
	return r.pft, nil
}

// Font loads a .fnt bound to FONTPAL.
func (r *Resources) Font(name string) (*text.Font, error) {
	if f, ok := r.fonts[name]; ok {
		return f, nil
	}
	f, err := text.Load(r.Cache, name)
	if err != nil {
		return nil, err
	}
	r.fonts[name] = f
	return f, nil
}

// Global returns global.txt string i ("" if out of range).
//
// mm8: 0x601360 (g_globalTxt)
func (r *Resources) Global(i int) (string, error) {
	if r.global == nil {
		raw, err := r.Cache.Text("global.txt")
		if err != nil {
			return "", err
		}
		r.global = ParseGlobal(raw)
	}
	if i < 0 || i >= len(r.global) {
		return "", nil
	}
	return r.global[i], nil
}

// globalTxtSize is the number of g_globalTxt slots (0x601360..0x601f18).
const globalTxtSize = 750

// ParseGlobal splits global.txt into its strings: rows after the two header tokens,
// second column, surrounding quotes removed. Rows are taken in file order (the index
// column is not read).
//
// mm8: 0x45181f (Txt_LoadGlobal), 0x451806 (Txt_StripQuotes)
func ParseGlobal(raw []byte) []string {
	// strtok(buf, "\r"): empty tokens are skipped; each later token starts with the
	// '\n' of the CRLF, which the parser steps over.
	var toks [][]byte
	for _, t := range bytes.Split(raw, []byte{'\r'}) {
		if len(t) > 0 {
			toks = append(toks, t)
		}
	}
	out := make([]string, 0, globalTxtSize)
	for i := 2; i < len(toks) && len(out) < globalTxtSize; i++ {
		row := toks[i][1:]
		cols := strings.SplitN(string(row), "\t", 3)
		s := ""
		if len(cols) > 1 && cols[0] != "" {
			s = txt.StripQuotes(cols[1])
		}
		out = append(out, s)
	}
	return out
}

// loader collects the first error while a screen loads many assets.
type loader struct {
	r   *Resources
	err error
}

func (l *loader) fail(err error) {
	if l.err == nil && err != nil {
		l.err = err
	}
}

func (l *loader) icon(name string, fromLangD bool) *gfx.Sprite {
	s, err := l.r.Cache.Icon(name, fromLangD)
	l.fail(err)
	return s
}

func (l *loader) pcx(name string) *gfx.Sprite {
	s, err := l.r.Cache.Pcx(name, true)
	l.fail(err)
	return s
}

func (l *loader) font(name string) *text.Font {
	f, err := l.r.Font(name)
	l.fail(err)
	return f
}

func (l *loader) global(i int) string {
	s, err := l.r.Global(i)
	l.fail(err)
	return s
}

func (l *loader) exeBytes(va uint32, n int) []byte {
	if l.r.Exe == nil {
		l.fail(fmt.Errorf("MM8-Rel.exe not opened"))
		return make([]byte, n)
	}
	b, err := l.r.Exe.Read(va, n)
	l.fail(err)
	if b == nil {
		b = make([]byte, n)
	}
	return b
}

func (l *loader) exeString(va uint32) string {
	if l.r.Exe == nil {
		l.fail(fmt.Errorf("MM8-Rel.exe not opened"))
		return ""
	}
	v, err := l.r.Exe.CString(va)
	l.fail(err)
	return v
}

func (l *loader) exeInts(va uint32, n int) []int32 {
	if l.r.Exe == nil {
		l.fail(fmt.Errorf("MM8-Rel.exe not opened"))
		return make([]int32, n)
	}
	v, err := l.r.Exe.Int32s(va, n)
	l.fail(err)
	if v == nil {
		v = make([]int32, n)
	}
	return v
}

func (l *loader) exeStrings(va uint32, n int) []string {
	if l.r.Exe == nil {
		l.fail(fmt.Errorf("MM8-Rel.exe not opened"))
		return make([]string, n)
	}
	v, err := l.r.Exe.CStringTable(va, n)
	l.fail(err)
	if v == nil {
		v = make([]string, n)
	}
	return v
}

// button builds an icon button the way the Build functions do: SetIcons(xy, keyed, up,
// dn, ht, 0, fromLangD).
//
// mm8: 0x4c4cdc (GuiButton_SetIcons)
func (l *loader) button(x, y int, msg Msg, up, dn, ht string, fromLangD bool) *Button {
	return NewButton(x, y, msg, l.icon(up, fromLangD), l.icon(dn, fromLangD), l.icon(ht, fromLangD))
}

// Items is the items table (items.txt and the bonus tables, with the item sizes).
//
// mm8: 0x455a6e (Txt_LoadItemsClassesSkills)
func (r *Resources) Items() (*tables.Items, error) {
	if r.items == nil {
		d := r.Cache.Data()
		if d == nil {
			return nil, fmt.Errorf("items: no game data")
		}
		t, err := tables.LoadItems(d)
		if err != nil {
			return nil, err
		}
		r.items = t
	}
	return r.items, nil
}

// Classes are the class and stat tables of MM8-Rel.exe.
func (r *Resources) Classes() (*tables.Classes, error) {
	if r.classes == nil {
		if r.Exe == nil {
			return nil, fmt.Errorf("classes: MM8-Rel.exe not opened")
		}
		c, err := tables.ReadClasses(r.Exe)
		if err != nil {
			return nil, err
		}
		r.classes = c
	}
	return r.classes, nil
}

func (l *loader) classes() *tables.Classes {
	c, err := l.r.Classes()
	l.fail(err)
	if c == nil {
		c = &tables.Classes{}
	}
	return c
}

// dialogIcon loads an icons.lod picture, PENDING when it is not there (as
// TexLod_LoadTexture falls back), nil when even that is missing.
//
// mm8: 0x411278 (TexLod_LoadTexture)
func (r *Resources) dialogIcon(name string) *gfx.Sprite {
	if s, err := r.Cache.Icon(name, false); err == nil {
		return s
	}
	s, _ := r.Cache.Icon("PENDING", false)
	return s
}

// note logs what a later milestone does, once per key.
func (r *Resources) note(key, what string) {
	if r.noted == nil {
		r.noted = map[string]bool{}
	}
	if !r.noted[key] {
		r.noted[key] = true
		log.Printf("%s", what)
	}
}

// ScrollText is message scroll n's text (scroll.txt; items 700 + n), "" when unknown.
//
// mm8: 0x761400 (Txt_LoadScroll 0x476fee)
func (r *Resources) ScrollText(n int) string {
	if r.scrolls == nil {
		raw, err := r.Cache.Text("scroll.txt")
		if err != nil {
			return ""
		}
		r.scrolls = tables.ParseScrolls(raw)
	}
	if n < 0 || n >= len(r.scrolls) {
		return ""
	}
	return r.scrolls[n]
}

// Awards is awards.txt (the character screen's awards page).
//
// mm8: 0x476f0a (Txt_LoadAwards, g_awards 0x761548)
func (r *Resources) Awards() []tables.Award {
	if r.awards == nil {
		raw, err := r.Cache.Text("awards.txt")
		if err != nil {
			return nil
		}
		r.awards = tables.ParseAwards(raw)
	}
	return r.awards
}
