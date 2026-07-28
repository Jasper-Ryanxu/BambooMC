// This file is part of go-mc/server project.
// Copyright (C) 2023.  Tnze
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package world

import (
	"math"
	"time"

	"go.uber.org/zap"

	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/go-mc/server/world/internal/bvh"
)

func (w *World) tickLoop() {
	var n uint
	for range time.Tick(time.Microsecond * 20) {
		w.tick(n)
		n++
	}
}

const ticksPerGameTick = 2500

func (w *World) tick(n uint) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()

	if n%8 == 0 {
		w.subtickChunkLoad()
	}
	w.subtickUpdatePlayers()
	w.subtickUpdateEntities()
	w.subtickUpdateItemEntities()
	w.subtickUpdateTime(n)
}

func (w *World) subtickUpdateTime(n uint) {
	if n%ticksPerGameTick != 0 {
		return
	}
	if !w.doDaylightCycle {
		return
	}
	w.time = (w.time + 1) % 24000
	w.timeDay = w.time
	for c := range w.players {
		c.SendWorldTime(w.time, w.timeDay)
	}
}

func (w *World) subtickChunkLoad() {
	for c, p := range w.players {
		x := int32(p.Position[0]) >> 4
		y := int32(p.Position[1]) >> 4
		z := int32(p.Position[2]) >> 4
		if newChunkPos := [3]int32{x, y, z}; newChunkPos != p.ChunkPos {
			p.ChunkPos = newChunkPos
			c.SendSetChunkCacheCenter([2]int32{x, z})
		}
	}
	// because of the random traversal order of w.loaders, every loader has the same opportunity, so it's relatively fair.
LoadChunk:
	for viewer, loader := range w.loaders {
		loader.calcLoadingQueue()
		for _, pos := range loader.loadQueue {
			if !loader.limiter.Allow() { // We reach the player limit. Skip
				break
			}
			if _, ok := w.chunks[pos]; !ok {
				if !w.loadChunk(pos) {
					break LoadChunk // We reach the global limit. skip
				}
			}
			loader.loaded[pos] = struct{}{}
			lc := w.chunks[pos]
			lc.AddViewer(viewer)
			lc.Lock()
			viewer.ViewChunkLoad(pos, lc.Chunk)
			lc.Unlock()
		}
	}
	for viewer, loader := range w.loaders {
		loader.calcUnusedChunks()
		for _, pos := range loader.unloadQueue {
			delete(loader.loaded, pos)
			if !w.chunks[pos].RemoveViewer(viewer) {
				w.log.Panic("viewer is not found in the loaded chunk")
			}
			viewer.ViewChunkUnload(pos)
		}
	}
	var unloadQueue [][2]int32
	for pos, chunk := range w.chunks {
		if _, isSpawn := w.spawnChunks[pos]; isSpawn {
			continue
		}
		if len(chunk.viewers) == 0 {
			unloadQueue = append(unloadQueue, pos)
		}
	}
	for i := range unloadQueue {
		w.unloadChunk(unloadQueue[i])
	}
}

func (w *World) subtickUpdatePlayers() {
	for c, p := range w.players {
		if !p.Inputs.TryLock() {
			continue
		}
		inputs := &p.Inputs
		// update the range of visual.
		if p.ViewDistance != int32(inputs.ViewDistance) {
			p.ViewDistance = int32(inputs.ViewDistance)
			p.view = w.playerViews.Insert(p.getView(), w.playerViews.Delete(p.view))
		}
		// delete entities that not in range from entities lists of each player.
		for id, e := range p.EntitiesInView {
			if !p.view.Box.WithIn(vec3d(e.Position)) {
				delete(p.EntitiesInView, id) // it should be safe to delete element from a map being traversed.
				p.view.Value.ViewRemoveEntities([]int32{id})
			}
		}
		if p.teleport != nil {
			if inputs.TeleportID == p.teleport.ID {
				p.pos0 = p.teleport.Position
				p.rot0 = p.teleport.Rotation
				p.teleport = nil
			}
		} else {
			delta := [3]float64{
				inputs.Position[0] - p.Position[0],
				inputs.Position[1] - p.Position[1],
				inputs.Position[2] - p.Position[2],
			}
			distance := math.Sqrt(delta[0]*delta[0] + delta[1]*delta[1] + delta[2]*delta[2])
			if distance > 100 {
				// You moved too quickly :( (Hacking?)
				teleportID := c.SendPlayerPosition(p.Position, p.Rotation)
				p.teleport = &TeleportRequest{
					ID:       teleportID,
					Position: p.Position,
					Rotation: p.Rotation,
				}
			} else if inputs.Position.IsValid() {
				p.pos0 = inputs.Position
				p.rot0 = inputs.Rotation
				p.OnGround = inputs.OnGround
			} else {
				w.log.Info("Player move invalid",
					zap.Float64("x", inputs.Position[0]),
					zap.Float64("y", inputs.Position[1]),
					zap.Float64("z", inputs.Position[2]),
				)
				c.SendDisconnect(chat.TranslateMsg("multiplayer.disconnect.invalid_player_movement"))
			}
		}

		// Void kill: teleport players who fall below Y=-50 back to spawn.
		if p.Position[1] < -50 {
			w.RespawnPlayer(c, p)
		}

		p.Inputs.Unlock()
	}
}

func (w *World) subtickUpdateEntities() {
	// TODO: entity list should be traversed here, but players are the only entities now.
	for _, e := range w.players {
		// sending Update Entity Position pack to every player who can see it, when it moves.
		var delta [3]int16
		var rot [2]int8
		if e.Position != e.pos0 { // TODO: send Teleport Entity pack instead when moving distance is greater than 8.
			delta = [3]int16{
				int16((e.pos0[0] - e.Position[0]) * 32 * 128),
				int16((e.pos0[1] - e.Position[1]) * 32 * 128),
				int16((e.pos0[2] - e.Position[2]) * 32 * 128),
			}
		}
		if e.Rotation != e.rot0 {
			rot = [2]int8{
				int8(e.rot0[0] * 256 / 360),
				int8(e.rot0[1] * 256 / 360),
			}
		}
		cond := bvh.TouchPoint[vec3d, aabb3d](vec3d(e.Position))
		w.playerViews.Find(cond,
			func(n *playerViewNode) bool {
				if n.Value.Player == e {
					return true // don't send the player self to the player
				}
				// check if the current entity is in range of player visual. if so, moving data will be forwarded.
				if _, ok := n.Value.EntitiesInView[e.EntityID]; !ok {
					// add the entity to the entity list of the player
					n.Value.ViewAddPlayer(e)
					n.Value.EntitiesInView[e.EntityID] = &e.Entity
				}
				return true
			},
		)
		var sendMove func(v EntityViewer)
		switch {
		case e.Position != e.pos0 && e.Rotation != e.rot0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityPosAndRot(e.EntityID, delta, rot, bool(e.OnGround))
				v.ViewRotateHead(e.EntityID, rot[0])
			}
		case e.Position != e.pos0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityPos(e.EntityID, delta, bool(e.OnGround))
			}
		case e.Rotation != e.rot0:
			sendMove = func(v EntityViewer) {
				v.ViewMoveEntityRot(e.EntityID, rot, bool(e.OnGround))
				v.ViewRotateHead(e.EntityID, rot[0])
			}
		default:
			continue
		}
		e.Position = e.pos0
		e.Rotation = e.rot0
		w.playerViews.Find(cond,
			func(n *playerViewNode) bool {
				if n.Value.Player == e {
					return true // not sending self movements to player self.
				}
				// check if the current entity is in the player visual entities list. if so, moving data will be forwarded.
				if _, ok := n.Value.EntitiesInView[e.EntityID]; ok {
					sendMove(n.Value.EntityViewer)
				} else {
					// or the entity will be add to the entities list of the player
					// TODO: deal with the situation that the entity is not a player
					n.Value.ViewAddPlayer(e)
					n.Value.EntitiesInView[e.EntityID] = &e.Entity
				}
				return true
			},
		)
	}
}

func (w *World) subtickUpdateItemEntities() {
	for id, e := range w.itemEntities {
		e.Age++
		if e.Age > 6000 { // 5 minutes
			w.removeItemEntityForAll(id)
			delete(w.itemEntities, id)
			continue
		}
		if e.PickupDelay > 0 {
			e.PickupDelay--
		}

		// find nearby players for visibility and pickup
		for c, p := range w.players {
			dx := p.Position[0] - e.Position[0]
			dy := p.Position[1] - e.Position[1]
			dz := p.Position[2] - e.Position[2]
			distSq := dx*dx + dy*dy + dz*dz
			viewDistance := float64(p.ViewDistance) * 16

			inView := distSq <= viewDistance*viewDistance
			_, wasInView := p.EntitiesInView[id]

			if inView {
				if !wasInView {
					c.ViewAddItemEntity(e)
					c.ViewSetEntityData(id, itemEntityData(e.Item))
					p.EntitiesInView[id] = &e.Entity
				}
				if e.Position != e.pos0 {
					delta := [3]int16{
						int16((e.Position[0] - e.pos0[0]) * 32 * 128),
						int16((e.Position[1] - e.pos0[1]) * 32 * 128),
						int16((e.Position[2] - e.pos0[2]) * 32 * 128),
					}
					c.ViewMoveEntityPos(id, delta, bool(e.OnGround))
				}
			} else if wasInView {
				c.ViewRemoveEntities([]int32{id})
				delete(p.EntitiesInView, id)
			}

			// pickup
			if e.PickupDelay == 0 && distSq < 1.5*1.5 {
				c.ViewTakeItemEntity(id, p.EntityID, pk.VarInt(e.Item.Count))
				w.removeItemEntityForAll(id)
				delete(w.itemEntities, id)
				break
			}
		}
		e.pos0 = e.Position
	}
}

func (w *World) removeItemEntityForAll(id int32) {
	for c, p := range w.players {
		if _, ok := p.EntitiesInView[id]; ok {
			c.ViewRemoveEntities([]int32{id})
			delete(p.EntitiesInView, id)
		}
	}
}

func itemEntityData(item Slot) []EntityDataItem {
	return []EntityDataItem{
		{
			Index: 8,
			Type:  6, // Slot
			Value: &item,
		},
	}
}
