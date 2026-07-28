// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package game

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/go-mc/server/client"
)

const (
	playerActionStartDestroy = 0
	playerActionAbortDestroy = 1
	playerActionStopDestroy  = 2
)

func (g *Game) handlePlayerAction(p pk.Packet, c *client.Client) error {
	var (
		status    pk.VarInt
		pos       pk.Position
		direction pk.Byte
		sequence  pk.VarInt
	)
	if err := p.Scan(&status, &pos, &direction, &sequence); err != nil {
		return err
	}

	switch status {
	case playerActionStartDestroy:
		// Survival clients send this when the player begins breaking a block.
		// We currently do not track break progress, so we ignore it.
	case playerActionAbortDestroy:
		// Client stopped breaking before the block broke.
	case playerActionStopDestroy:
		// Block has been broken. No items are dropped (creative-style behavior).
		p := c.GetPlayer()
		g.overworld.BreakBlock([3]int32{int32(pos.X), int32(pos.Y), int32(pos.Z)}, p.UUID)
	}
	// Acknowledge the action sequence so the client stays in sync.
	c.SendBlockChangedAck(int32(sequence))
	return nil
}

func (g *Game) registerPlayerActionHandler(c *client.Client) {
	c.AddHandler(packetid.ServerboundPlayerAction, g.handlePlayerAction)
}
