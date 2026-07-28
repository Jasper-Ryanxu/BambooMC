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
	"errors"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/go-mc/server/world/internal/bvh"
)

const dataVersion = 3337 // Minecraft 1.19.4

type World struct {
	log           *zap.Logger
	config        Config
	chunkProvider ChunkProvider

	chunks   map[[2]int32]*LoadedChunk
	loaders  map[ChunkViewer]*loader
	tickLock sync.Mutex

	// playerViews is a BVH tree，storing the visual range collision boxes of each player.
	// the data structure is used to determine quickly which players to send notify when entity moves.
	playerViews playerViewTree
	players     map[Client]*Player

	// itemEntities are dropped items in the world.
	itemEntities map[int32]*ItemEntity

	// spawnChunks are kept loaded even when no player is watching them.
	spawnChunks map[[2]int32]struct{}

	// world time in ticks (0-24000)
	time     int64
	timeDay  int64
	doDaylightCycle bool
}

type Config struct {
	ViewDistance  int32
	SpawnAngle    float32
	SpawnPosition [3]int32
	Seed          int64
}

type playerView struct {
	EntityViewer
	*Player
}

type (
	vec3d          = bvh.Vec3[float64]
	aabb3d         = bvh.AABB[float64, vec3d]
	playerViewNode = bvh.Node[float64, aabb3d, playerView]
	playerViewTree = bvh.Tree[float64, aabb3d, playerView]
)

func New(logger *zap.Logger, provider ChunkProvider, config Config) (w *World) {
	w = &World{
		log:             logger,
		config:          config,
		chunks:          make(map[[2]int32]*LoadedChunk),
		loaders:         make(map[ChunkViewer]*loader),
		players:         make(map[Client]*Player),
		itemEntities:    make(map[int32]*ItemEntity),
		spawnChunks:     make(map[[2]int32]struct{}),
		chunkProvider:   provider,
		time:            1000,
		timeDay:         1000,
		doDaylightCycle: true,
	}
	go w.tickLoop()
	return
}

func (w *World) Name() string {
	return "minecraft:overworld"
}

func (w *World) SpawnPositionAndAngle() ([3]int32, float32) {
	return w.config.SpawnPosition, w.config.SpawnAngle
}

// RespawnPlayer teleports a player back to the world spawn point and updates
// their chunk cache. This is used by /kill and the void kill logic.
func (w *World) RespawnPlayer(c Client, p *Player) {
	spawn, angle := w.SpawnPositionAndAngle()
	p.Position = [3]float64{float64(spawn[0]), float64(spawn[1]), float64(spawn[2])}
	p.pos0 = p.Position
	p.Rotation = [2]float32{angle, 0}
	p.rot0 = p.Rotation
	p.ChunkPos = [3]int32{spawn[0] >> 4, spawn[1] >> 4, spawn[2] >> 4}
	c.SendPlayerPosition(p.Position, p.Rotation)
	c.SendSetChunkCacheCenter([2]int32{p.ChunkPos[0], p.ChunkPos[2]})
}

// PreloadSpawnChunks loads and marks the chunks around spawn so they stay loaded.
func (w *World) PreloadSpawnChunks(radius int32) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()

	spawn := w.config.SpawnPosition
	cx, cz := spawn[0]>>4, spawn[2]>>4
	for x := -radius; x <= radius; x++ {
		for z := -radius; z <= radius; z++ {
			pos := [2]int32{cx + x, cz + z}
			w.spawnChunks[pos] = struct{}{}
			if _, ok := w.chunks[pos]; !ok {
				w.loadChunk(pos)
			}
		}
	}
	w.log.Info("出生点区块已预加载",
		zap.Int32("radius", radius),
		zap.Int("count", len(w.spawnChunks)),
	)
}

func (w *World) HashedSeed() [8]byte {
	return [8]byte{}
}

func (w *World) AddPlayer(c Client, p *Player, limiter *rate.Limiter) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.loaders[c] = newLoader(p, limiter)
	w.players[c] = p
	p.view = w.playerViews.Insert(p.getView(), playerView{c, p})
}

func (w *World) GetTime() int64 {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	return w.time
}

func (w *World) SetTime(t int64) {
	w.tickLock.Lock()
	w.time = t % 24000
	if w.time < 0 {
		w.time += 24000
	}
	w.timeDay = w.time
	w.tickLock.Unlock()
	w.broadcastTime()
}

func (w *World) AddTime(t int64) {
	w.tickLock.Lock()
	w.time = (w.time + t) % 24000
	if w.time < 0 {
		w.time += 24000
	}
	w.timeDay = w.time
	w.tickLock.Unlock()
	w.broadcastTime()
}

func (w *World) broadcastTime() {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	for c := range w.players {
		c.SendWorldTime(w.time, w.timeDay)
	}
}

func (w *World) AddItemEntity(e *ItemEntity) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.itemEntities[e.EntityID] = e
}

func (w *World) RemoveItemEntity(id int32) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	delete(w.itemEntities, id)
}

func (w *World) GetBlock(pos [3]int32) block.StateID {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	return w.getBlockNoLock(pos)
}

func (w *World) getBlockNoLock(pos [3]int32) block.StateID {
	cpos := [2]int32{pos[0] >> 4, pos[2] >> 4}
	lc, ok := w.chunks[cpos]
	if !ok {
		return 0
	}
	lx := int(pos[0] & 15)
	lz := int(pos[2] & 15)
	y := int(pos[1])
	if y < worldMinY || y > worldMaxY {
		return 0
	}
	secIdx := (y - worldMinY) / 16
	localY := y - worldMinY - secIdx*16
	idx := ((localY * 16) + lz) * 16 + lx
	return block.StateID(lc.Sections[secIdx].GetBlock(idx))
}

func (w *World) SetBlock(pos [3]int32, stateID block.StateID) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.setBlockNoLock(pos, stateID)
}

func (w *World) setBlockNoLock(pos [3]int32, stateID block.StateID) {
	cpos := [2]int32{pos[0] >> 4, pos[2] >> 4}
	lc, ok := w.chunks[cpos]
	if !ok {
		return
	}
	lx := int(pos[0] & 15)
	lz := int(pos[2] & 15)
	y := int(pos[1])
	if y < worldMinY || y > worldMaxY {
		return
	}
	secIdx := (y - worldMinY) / 16
	localY := y - worldMinY - secIdx*16
	idx := ((localY * 16) + lz) * 16 + lx
	lc.Sections[secIdx].SetBlock(idx, stateID)
	pkPos := pk.Position{X: int(pos[0]), Y: int(pos[1]), Z: int(pos[2])}
	for _, c := range lc.viewers {
		c.ViewBlockUpdate(pkPos, int32(stateID))
	}
}

// BreakBlock removes the block at pos.
// Items are not dropped, matching creative-mode behavior in survival mode.
func (w *World) BreakBlock(pos [3]int32, owner uuid.UUID) *ItemEntity {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	if w.getBlockNoLock(pos) == 0 {
		return nil
	}
	w.setBlockNoLock(pos, 0)
	return nil
}

func (w *World) RemovePlayer(c Client, p *Player) {
	w.tickLock.Lock()
	defer w.tickLock.Unlock()
	w.log.Debug("Remove Player",
		zap.Int("loader count", len(w.loaders[c].loaded)),
		zap.Int("world count", len(w.chunks)),
	)
	// delete the player from all chunks which load the player.
	for pos := range w.loaders[c].loaded {
		if !w.chunks[pos].RemoveViewer(c) {
			w.log.Panic("viewer is not found in the loaded chunk")
		}
	}
	delete(w.loaders, c)
	delete(w.players, c)
	// delete the player from entity system.
	w.playerViews.Delete(p.view)
	w.playerViews.Find(
		bvh.TouchPoint[vec3d, aabb3d](bvh.Vec3[float64](p.Position)),
		func(n *playerViewNode) bool {
			n.Value.ViewRemoveEntities([]int32{p.EntityID})
			delete(n.Value.EntitiesInView, p.EntityID)
			return true
		},
	)
}

func (w *World) loadChunk(pos [2]int32) bool {
	logger := w.log.With(zap.Int32("x", pos[0]), zap.Int32("z", pos[1]))
	logger.Debug("Loading chunk")
	c, err := w.chunkProvider.GetChunk(pos)
	if err != nil {
		if errors.Is(err, errChunkNotExist) {
			logger.Debug("Generate chunk")
			c = level.EmptyChunk(sectionCount)
			generateTerrain(c, pos, w.config.Seed)
		} else if !errors.Is(err, ErrReachRateLimit) {
			logger.Error("GetChunk error", zap.Error(err))
			return false
		}
	}
	if c != nil {
		initChunkLight(c)
	}
	w.chunks[pos] = &LoadedChunk{Chunk: c}
	return true
}

func (w *World) unloadChunk(pos [2]int32) {
	logger := w.log.With(zap.Int32("x", pos[0]), zap.Int32("z", pos[1]))
	logger.Debug("Unloading chunk")
	c, ok := w.chunks[pos]
	if !ok {
		logger.Panic("Unloading an non-exist chunk")
	}
	// notify all viewers who are watching the chunk to unload the chunk
	for _, viewer := range c.viewers {
		viewer.ViewChunkUnload(pos)
	}
	// move the chunk to provider and save
	err := w.chunkProvider.PutChunk(pos, c.Chunk)
	if err != nil {
		logger.Error("Store chunk data error", zap.Error(err))
	}
	delete(w.chunks, pos)
}

type LoadedChunk struct {
	sync.Mutex
	viewers []ChunkViewer
	*level.Chunk
}

func (lc *LoadedChunk) AddViewer(v ChunkViewer) {
	lc.Lock()
	defer lc.Unlock()
	for _, v2 := range lc.viewers {
		if v2 == v {
			panic("append an exist viewer")
		}
	}
	lc.viewers = append(lc.viewers, v)
}

func (lc *LoadedChunk) RemoveViewer(v ChunkViewer) bool {
	lc.Lock()
	defer lc.Unlock()
	for i, v2 := range lc.viewers {
		if v2 == v {
			last := len(lc.viewers) - 1
			lc.viewers[i] = lc.viewers[last]
			lc.viewers = lc.viewers[:last]
			return true
		}
	}
	return false
}
