package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
)

// Shops are the shops' and guilds' stock and when they restock (internal/game/dialog
// fills and sells them).
type Shops struct {
	// Standard and Special are the shops' two shelves (g_shopStandard 0xb7c98c and
	// g_shopSpecial 0xb822fc, [house * 12 + slot]).
	Standard, Special [tables.NumShopHouses][tables.ShopSlots]items.Item
	// Restock is when a shop next restocks (Party +0x8c + house * 8).
	Restock [tables.NumShopHouses]clock.Time
	// Spells are the guild shelves (g_guildSpells 0xb87c6c, [house - 0x8b][school][slot]).
	Spells [tables.NumGuilds][tables.NumSchools][tables.ShopSlots]items.Item
	// SpellRestock is when a guild's shelves next restock (0xb20b6c + house * 8).
	SpellRestock [tables.FirstGuild + tables.NumGuilds]clock.Time
}

// ShopState is the shops' state, made on first use.
func (m *Members) ShopState() *Shops {
	if m.Shops == nil {
		m.Shops = &Shops{}
	}
	return m.Shops
}
