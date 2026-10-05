package tables

import (
	"encoding/binary"

	"libre-enroth/internal/assets/exe"
)

// The player tables compiled into MM8-Rel.exe: per class HP and spell points, the
// creation stats and skills, the condition and age percentages of the stats, the stat
// bonus thresholds and the equip slots (re/notes/items.md#tables).

// StatRange is a class's g_classStatTable entry for one stat.
type StatRange struct {
	Base, Max uint8
	// Above the base a click on + costs UpCost points and adds UpStep; below it the
	// two swap (PartyCreate_PointsLeft weighs (base - value) * UpCost / UpStep).
	UpCost, UpStep uint8
}

// Classes are the class and stat tables.
type Classes struct {
	HPBase, SPBase         [NumClasses]uint8 // 0x4ff9ec, 0x4ff9fc
	HPPerLevel, SPPerLevel [NumClasses]uint8 // 0x4ffa0c, 0x4ffa1c
	// Stats are the creation stats per class, in stat order Might, Intellect,
	// Personality, Endurance, Accuracy, Speed, Luck (0x4ffa2c).
	Stats [NumClasses][7]StatRange
	// Skills are the class skills per class pair (class / 2): 2 the two a new
	// character has, 1 choosable at creation, 0 not (0x4ffbec; the houses that teach
	// read it too).
	Skills [NumClasses / 2][NumSkills]uint8
	// CondPct is the % of each stat a condition leaves ([stat][condition], 0x500000).
	CondPct [7][19]uint8
	// AgeLimits and AgePct: below AgeLimits[i] years a stat counts AgePct[stat][i] %
	// (0x500088, 0x500098); from the last limit on, 100 %.
	AgeLimits [4]int32
	AgePct    [7][4]uint8
	// BonusLimits and Bonus are StatToBonus's table: the first limit a value is not
	// below gives Bonus at its index (0x5000b4, 0x5000f0). BonusLimits ends with its 0.
	BonusLimits []int16
	Bonus       []int8
	// EquipSlot is the slot an equip type goes to (0x4f9ee0; rings take the first free
	// of 10..15).
	EquipSlot [13]uint8
}

// Exe addresses of the class tables.
const (
	vaClassHPBase   = 0x4ff9ec
	vaClassSPBase   = 0x4ff9fc
	vaClassHPLevel  = 0x4ffa0c
	vaClassSPLevel  = 0x4ffa1c
	vaClassStats    = 0x4ffa2c
	vaClassSkills   = 0x4ffbec
	vaCondPct       = 0x500000
	vaAgeLimits     = 0x500088
	vaAgePct        = 0x500098
	vaBonusLimits   = 0x5000b4
	vaBonus         = 0x5000f0
	vaEquipSlot     = 0x4f9ee0
	numBonusLimits  = (vaBonus - vaBonusLimits) / 2
	numCondPctConds = 19
)

// ReadClasses reads the class tables from the executable.
//
// mm8: 0x48f5b9 (MaxHP), 0x48f61d (MaxSP), 0x48fd62 (condition %), 0x48fd79 (age %),
// 0x48fda7 (StatToBonus), 0x492094 (creation), 0x492d41 (equip slots)
func ReadClasses(im *exe.Image) (*Classes, error) {
	c := &Classes{}
	read := func(va uint32, dst []byte) error {
		b, err := im.Read(va, len(dst))
		if err == nil {
			copy(dst, b)
		}
		return err
	}
	for _, r := range []struct {
		va  uint32
		dst []byte
	}{
		{vaClassHPBase, c.HPBase[:]}, {vaClassSPBase, c.SPBase[:]},
		{vaClassHPLevel, c.HPPerLevel[:]}, {vaClassSPLevel, c.SPPerLevel[:]},
		{vaEquipSlot, c.EquipSlot[:]},
	} {
		if err := read(r.va, r.dst); err != nil {
			return nil, err
		}
	}
	b, err := im.Read(vaClassStats, NumClasses*7*4)
	if err != nil {
		return nil, err
	}
	for cl := range c.Stats {
		for s := range c.Stats[cl] {
			e := b[(cl*7+s)*4:]
			c.Stats[cl][s] = StatRange{Base: e[0], Max: e[1], UpCost: e[2], UpStep: e[3]}
		}
	}
	if b, err = im.Read(vaClassSkills, NumClasses/2*NumSkills); err != nil {
		return nil, err
	}
	for i := range c.Skills {
		copy(c.Skills[i][:], b[i*NumSkills:])
	}
	if b, err = im.Read(vaCondPct, 7*numCondPctConds); err != nil {
		return nil, err
	}
	for s := range c.CondPct {
		copy(c.CondPct[s][:], b[s*numCondPctConds:])
	}
	ages, err := im.Int32s(vaAgeLimits, 4)
	if err != nil {
		return nil, err
	}
	copy(c.AgeLimits[:], ages)
	if b, err = im.Read(vaAgePct, 7*4); err != nil {
		return nil, err
	}
	for s := range c.AgePct {
		copy(c.AgePct[s][:], b[s*4:])
	}
	if b, err = im.Read(vaBonusLimits, numBonusLimits*2); err != nil {
		return nil, err
	}
	for i := 0; i < numBonusLimits; i++ {
		v := int16(binary.LittleEndian.Uint16(b[2*i:]))
		c.BonusLimits = append(c.BonusLimits, v)
		if v == 0 {
			break
		}
	}
	if b, err = im.Read(vaBonus, len(c.BonusLimits)); err != nil {
		return nil, err
	}
	for _, v := range b {
		c.Bonus = append(c.Bonus, int8(v))
	}
	return c, nil
}

// StatToBonus is the bonus a stat value gives (500 and more: 30, below 3: -6).
//
// mm8: 0x48fda7
func (c *Classes) StatToBonus(v int) int {
	i := 0
	if v < int(c.BonusLimits[0]) {
		for {
			if c.BonusLimits[i] == 0 {
				break
			}
			i++
			if v >= int(c.BonusLimits[i]) {
				break
			}
		}
	}
	return int(c.Bonus[i])
}

// AgePercent is the % of a stat a character of age years keeps.
//
// mm8: 0x48fd79
func (c *Classes) AgePercent(age, stat int) int {
	for i, lim := range c.AgeLimits {
		if age < int(lim) {
			return int(c.AgePct[stat][i])
		}
	}
	return 100
}

// ClassSkill is the class table value of skill for class: 2 a starting skill, 1
// choosable at creation (and learnable), 0 not for this class.
//
// mm8: 0x4ffbec (g_classSkillTable[class / 2][skill])
func (c *Classes) ClassSkill(class, skill int) uint8 {
	if class < 0 || class >= NumClasses || skill < 0 || skill >= NumSkills {
		return 0
	}
	return c.Skills[class/2][skill]
}
