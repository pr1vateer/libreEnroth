package dialog

import "libre-enroth/internal/game/party"

// ServiceMenu is a proprietor's menu for house type typ: the service codes of its
// buttons (msg 0x195), at x 0x1e0, y 0x92 + 0x1e*i. Shops: buy standard 2, buy special
// 0x5f, display/sell 0x5e, learn skills 0x60; guilds 0xc/0xd/0xe/0xf; tavern 0xf 0x10
// 0x60 0x65; bank 7 8; temple 10 0xb 0x60; stables and boats 0x69..0x6c; training 0x11
// 0x60; the town hall 99 (and 100 while a bounty hunt is on, 0xbb2df4). What each does
// is M6c's (and M7's for the item services).
//
// mm8: 0x4b496e (with 0x4b4397 adding each button)
func ServiceMenu(typ int, m *party.Members) []Button {
	var codes []int
	switch {
	case typ == 0x11:
		codes = []int{99}
		// 0xbb2df4 (a bounty hunt is running) adds 100: M6c.
	case typ >= 1 && typ <= 4:
		codes = []int{2, 0x5f, 0x5e, 0x60}
	case typ == 0xc:
		codes = []int{0x75, 0x60}
	case typ == 0xd:
		codes = []int{0x76, 0x60}
	case typ == 0xe:
		codes = []int{0x6e, 0x6f, 0x71, 0x70, 0x60}
	case typ == 0xf:
		codes = []int{0x74, 0x73, 0x72, 0x60}
	case typ == 0x15:
		codes = []int{0xf, 0x10, 0x60, 0x65}
	case typ == 0x16:
		codes = []int{7, 8}
	case typ == 0x17:
		codes = []int{10, 0xb, 0x60}
	case typ == 0x1b, typ == 0x1c:
		codes = []int{0x69, 0x6a, 0x6b, 0x6c}
	case typ == 0x1e:
		codes = []int{0x11, 0x60}
	}
	out := make([]Button, len(codes))
	for i, c := range codes {
		out[i] = Button{MsgService, c}
	}
	return out
}
