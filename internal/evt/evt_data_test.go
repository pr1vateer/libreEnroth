package evt

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"libre-enroth/internal/assets"
	"libre-enroth/internal/assets/assettest"
)

// allScripts loads every .evt and .str of EnglishT.lod.
func allScripts(t *testing.T) (scripts map[string]*Script, strs map[string][]string) {
	t.Helper()
	d, err := assets.OpenAll(assettest.Dir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	scripts, strs = map[string]*Script{}, map[string][]string{}
	for _, e := range d.LangT.Entries {
		name := strings.ToLower(e.Name)
		switch {
		case strings.HasSuffix(name, ".evt"):
			_, b, err := d.LangFile(name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			max := MapMaxBytes
			if name == "global.evt" {
				max = GlobalMaxBytes
			}
			s, err := Parse(b, max)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			scripts[name] = s
		case strings.HasSuffix(name, ".str"):
			_, b, err := d.LangFile(name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			ss, err := ParseStrings(b)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			strs[name] = ss
		}
	}
	return scripts, strs
}

// TestParseAll: every .evt and .str of the game parses within the original's limits:
// 62 .evt files with 25395 records (global.evt 4983 of them) and 63 .str files.
//
// mm8: 0x441a6f, 0x442119, 0x441aff
func TestParseAll(t *testing.T) {
	scripts, strs := allScripts(t)
	total := 0
	ops := map[Op]int{}
	for _, s := range scripts {
		total += len(s.Records)
		for _, r := range s.Records {
			ops[r.Op()]++
		}
	}
	if len(scripts) != 62 || len(strs) != 63 {
		t.Errorf("%d .evt and %d .str files, want 62 and 63", len(scripts), len(strs))
	}
	if total != 25395 || len(scripts["global.evt"].Records) != 4983 {
		t.Errorf("%d records, global %d; want 25395, 4983", total, len(scripts["global.evt"].Records))
	}
	if s := strs["out01.str"]; len(s) < 3 || s[1] != "Dagger Wound Islands" {
		t.Errorf("out01.str starts %q", s[:min(3, len(s))])
	}
	var keys []int
	for op := range ops {
		keys = append(keys, int(op))
	}
	sort.Ints(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, " %#x:%d", k, ops[Op(k)])
	}
	t.Logf("opcodes:%s", b.String())
}

// The item opcodes' arguments in the shipped scripts: GiveItem levels are 1..6 (the
// level clamp of giveItem never changes one), SummonItem levels (id / 1000) too.
func TestItemOpcodeArgs(t *testing.T) {
	scripts, _ := allScripts(t)
	kinds := map[int]int{}
	n, summons := 0, 0
	for name, s := range scripts {
		for _, r := range s.Records {
			switch r.Op() {
			case OpGiveItem:
				n++
				kinds[r.U8(6)]++
				if l := r.U8(5); l < 1 || l > 6 {
					t.Errorf("%s event %d: GiveItem level %d", name, r.ID(), l)
				}
			case OpSummonItem:
				summons++
				if v := r.U32(5); v > 1000 && (v/1000 < 1 || v/1000 > 6) {
					t.Errorf("%s event %d: SummonItem %d", name, r.ID(), v)
				}
			}
		}
	}
	t.Logf("%d GiveItem (kinds %v), %d SummonItem", n, kinds, summons)
}
