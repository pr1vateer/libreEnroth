package ui

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// ChestView is a chest an event opened: the record, its grid and picture.
type ChestView struct {
	Chest   *items.Chest
	Grid    tables.ChestGrid
	Picture int // "chest%02d"
}

// ChestOpener is a world whose events open chests (evt OpenChest).
type ChestOpener interface {
	// OpenedChest is the chest an event opened, nil for none.
	OpenedChest() *ChestView
	// CloseChest closes it (the chest screen's Close).
	CloseChest()
}
