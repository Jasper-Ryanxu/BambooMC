// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package world

import (
	"io"

	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Slot represents an item stack in the Minecraft protocol.
type Slot struct {
	ID    pk.VarInt
	Count pk.Byte
	NBT   nbt.RawMessage
}

func (s *Slot) WriteTo(w io.Writer) (n int64, err error) {
	var present pk.Boolean = s != nil && s.ID != -1
	return pk.Tuple{
		present, pk.Opt{
			Has: present,
			Field: pk.Tuple{
				&s.ID, &s.Count, pk.NBT(&s.NBT),
			},
		},
	}.WriteTo(w)
}

func (s *Slot) ReadFrom(r io.Reader) (n int64, err error) {
	var present pk.Boolean
	n, err = present.ReadFrom(r)
	if err != nil {
		return
	}
	if !present {
		s.ID = -1
		s.Count = 0
		s.NBT = nbt.RawMessage{Type: nbt.TagEnd}
		return
	}
	nn, err := pk.Tuple{&s.ID, &s.Count, pk.NBT(&s.NBT)}.ReadFrom(r)
	return n + nn, err
}

// EmptySlot returns an empty slot.
func EmptySlot() Slot {
	return Slot{ID: -1, Count: 0, NBT: nbt.RawMessage{Type: nbt.TagEnd}}
}
