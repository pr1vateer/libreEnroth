package evt

import "fmt"

// The opcodes (re/notes/evt.md has the record layouts).
const (
	OpExit              Op = 0x01
	OpSpeakInHouse      Op = 0x02
	OpPlaySound         Op = 0x03
	OpHint              Op = 0x04
	OpLocationName      Op = 0x05
	OpMoveToMap         Op = 0x06
	OpOpenChest         Op = 0x07
	OpShowFace          Op = 0x08
	OpReceiveDamage     Op = 0x09
	OpSetSnow           Op = 0x0a
	OpSetTexture        Op = 0x0b
	OpShowMovie         Op = 0x0c
	OpSetSprite         Op = 0x0d
	OpCompare           Op = 0x0e
	OpChangeDoorState   Op = 0x0f
	OpAdd               Op = 0x10
	OpSubtract          Op = 0x11
	OpSet               Op = 0x12
	OpSummonMonsters    Op = 0x13
	OpCastSpell         Op = 0x15
	OpSpeakNPC          Op = 0x16
	OpSetFacesBit       Op = 0x17
	OpToggleActorFlag   Op = 0x18
	OpRandomGoTo        Op = 0x19
	OpInputString       Op = 0x1a
	OpStatusText        Op = 0x1d
	OpShowMessage       Op = 0x1e
	OpOnTimer           Op = 0x1f
	OpToggleIndoorLight Op = 0x20
	OpPressAnyKey       Op = 0x21
	OpSummonItem        Op = 0x22
	OpForPartyMember    Op = 0x23
	OpJmp               Op = 0x24
	OpOnMapReload       Op = 0x25
	OpOnLongTimer       Op = 0x26
	OpSetNPCTopic       Op = 0x27
	OpMoveNPC           Op = 0x28
	OpGiveItem          Op = 0x29
	OpChangeEvent       Op = 0x2a
	OpCheckSkill        Op = 0x2b
	OpOnCanShowDialog   Op = 0x2c // topic visibility: compare over the party
	OpEndCanShowDialog  Op = 0x2d
	OpSetCanShowDialog  Op = 0x2e
	OpSetNPCGroupNews   Op = 0x2f
	OpSetActorGroup     Op = 0x30
	OpNPCSetItem        Op = 0x31
	OpSetNPCGreeting    Op = 0x32
	OpIsActorKilled     Op = 0x33
	OpCanShowTopicKill  Op = 0x34 // topic visibility: IsActorKilled
	OpOnMapLeave        Op = 0x35
	OpChangeGroup       Op = 0x36
	OpChangeGroupAlly   Op = 0x37
	OpCheckSeason       Op = 0x38
	OpToggleGroupFlag   Op = 0x39
	OpToggleChestFlag   Op = 0x3a
	OpCharacterAnim     Op = 0x3b
	OpSetActorItem      Op = 0x3c
	OpOnDateTimer       Op = 0x3d
	OpEnableDateTimer   Op = 0x3e
	OpStopDoor          Op = 0x3f
	OpCheckItemsCount   Op = 0x40
	OpRemoveItems       Op = 0x41
	OpSpecialJump       Op = 0x42
	OpIsTotalBounty     Op = 0x43
	OpIsPlayerInParty   Op = 0x44
)

// opInfo names an opcode and, for the ones libre-enroth does not do yet, the
// milestone that will.
type opInfo struct {
	name      string
	milestone string // "" = implemented (or a trigger handled outside the VM)
}

var opInfos = map[Op]opInfo{
	OpExit: {"Exit", ""}, OpSpeakInHouse: {"SpeakInHouse", "M6"}, OpPlaySound: {"PlaySound", "M11"},
	OpHint: {"Hint", ""}, OpLocationName: {"LocationName", ""}, OpMoveToMap: {"MoveToMap", ""},
	OpOpenChest: {"OpenChest", "M7"}, OpShowFace: {"ShowFace", ""}, OpReceiveDamage: {"ReceiveDamage", "M9"},
	OpSetSnow: {"SetSnow", ""}, OpSetTexture: {"SetTexture", ""}, OpShowMovie: {"ShowMovie", "M11"},
	OpSetSprite: {"SetSprite", ""}, OpCompare: {"Compare", ""}, OpChangeDoorState: {"ChangeDoorState", ""},
	OpAdd: {"Add", ""}, OpSubtract: {"Subtract", ""}, OpSet: {"Set", ""},
	OpSummonMonsters: {"SummonMonsters", "M8"}, OpCastSpell: {"CastSpell", "M9"},
	OpSpeakNPC: {"SpeakNPC", "M6"}, OpSetFacesBit: {"SetFacesBit", ""},
	OpToggleActorFlag: {"ToggleActorFlag", "M8"}, OpRandomGoTo: {"RandomGoTo", ""},
	OpInputString: {"InputString", "M6"}, OpStatusText: {"StatusText", ""},
	OpShowMessage: {"ShowMessage", ""}, OpOnTimer: {"OnTimer", ""},
	OpToggleIndoorLight: {"ToggleIndoorLight", ""}, OpPressAnyKey: {"PressAnyKey", "M6"},
	OpSummonItem: {"SummonItem", "M7"}, OpForPartyMember: {"ForPartyMember", ""}, OpJmp: {"Jmp", ""},
	OpOnMapReload: {"OnMapReload", ""}, OpOnLongTimer: {"OnLongTimer", ""},
	OpSetNPCTopic: {"SetNPCTopic", "M6"}, OpMoveNPC: {"MoveNPC", "M6"}, OpGiveItem: {"GiveItem", "M7"},
	OpChangeEvent: {"ChangeEvent", ""}, OpCheckSkill: {"CheckSkill", "M7"},
	OpOnCanShowDialog: {"OnCanShowDialogItemCmp", "M6"}, OpEndCanShowDialog: {"EndCanShowDialogItem", "M6"},
	OpSetCanShowDialog: {"SetCanShowDialogItem", "M6"}, OpSetNPCGroupNews: {"SetNPCGroupNews", "M6"},
	OpSetActorGroup: {"SetActorGroup", "M8"}, OpNPCSetItem: {"NPCSetItem", "M8"},
	OpSetNPCGreeting: {"SetNPCGreeting", "M6"}, OpIsActorKilled: {"IsActorKilled", "M8"},
	OpCanShowTopicKill: {"CanShowTopicIsActorKilled", "M6"}, OpOnMapLeave: {"OnMapLeave", ""},
	OpChangeGroup: {"ChangeGroup", "M8"}, OpChangeGroupAlly: {"ChangeGroupAlly", "M8"},
	OpCheckSeason: {"CheckSeason", ""}, OpToggleGroupFlag: {"ToggleActorGroupFlag", "M8"},
	OpToggleChestFlag: {"ToggleChestFlag", ""}, OpCharacterAnim: {"CharacterAnimation", ""},
	OpSetActorItem: {"SetActorItem", "M8"}, OpOnDateTimer: {"OnDateTimer", "M10"},
	OpEnableDateTimer: {"EnableDateTimer", "M10"}, OpStopDoor: {"StopDoor", ""},
	OpCheckItemsCount: {"CheckItemsCount", "M7"}, OpRemoveItems: {"RemoveItems", "M7"},
	OpSpecialJump: {"SpecialJump", "M9"}, OpIsTotalBounty: {"IsTotalBountyInRange", ""},
	OpIsPlayerInParty: {"IsPlayerInParty", "M6"},
}

func (o Op) String() string {
	if i, ok := opInfos[o]; ok {
		return i.name
	}
	return fmt.Sprintf("Op(%#x)", uint8(o))
}

// Milestone is the milestone that implements o ("" when the VM does, "?" for an opcode
// the game has no case for).
func (o Op) Milestone() string {
	if i, ok := opInfos[o]; ok {
		return i.milestone
	}
	return "?"
}

// Known reports an opcode the game's interpreter or trigger code handles.
func (o Op) Known() bool {
	_, ok := opInfos[o]
	return ok
}
