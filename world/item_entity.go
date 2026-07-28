// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package world

import (
	"encoding/binary"

	"github.com/google/uuid"
)

// ItemEntityTypeID is the Minecraft 1.19.4 protocol entity type ID for item drops.
const ItemEntityTypeID = 54

// ItemEntity is a dropped item on the ground.
type ItemEntity struct {
	Entity
	Item Slot
	// Owner is the UUID of the player who dropped the item.
	// Only this player can pick it up for the first few ticks.
	Owner uuid.UUID
	// PickupDelay is the number of ticks before the item can be picked up.
	PickupDelay int
	// Age is how many ticks the item has existed.
	Age int
}

// NewItemEntity creates a new dropped item entity.
func NewItemEntity(pos Position, item Slot, owner uuid.UUID) *ItemEntity {
	e := &ItemEntity{
		Entity: Entity{
			EntityID: NewEntityID(),
			Position: pos,
			Rotation: [2]float32{0, 0},
		},
		Item:        item,
		Owner:       owner,
		PickupDelay: 10, // 0.5 seconds
	}
	e.pos0 = pos
	return e
}

// EntityIDAsUUID returns a deterministic UUID derived from the entity ID.
// The AddEntity packet requires a UUID for every entity.
func (e *ItemEntity) EntityIDAsUUID() uuid.UUID {
	var u uuid.UUID
	binary.BigEndian.PutUint64(u[0:8], 0)
	binary.BigEndian.PutUint64(u[8:16], uint64(e.EntityID))
	return u
}
