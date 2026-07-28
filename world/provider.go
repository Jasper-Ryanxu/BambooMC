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
	"compress/gzip"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/save"
	"github.com/Tnze/go-mc/save/region"
	"github.com/Tnze/go-mc/yggdrasil/user"
)

// ChunkProvider implements chunk storage
type ChunkProvider struct {
	dir     string
	limiter *rate.Limiter
}

func NewProvider(dir string, limiter *rate.Limiter) ChunkProvider {
	return ChunkProvider{dir: dir, limiter: limiter}
}

var ErrReachRateLimit = errors.New("reach rate limit")

func (p *ChunkProvider) GetChunk(pos [2]int32) (c *level.Chunk, errRet error) {
	if !p.limiter.Allow() {
		return nil, ErrReachRateLimit
	}
	r, err := p.getRegion(region.At(int(pos[0]), int(pos[1])))
	if err != nil {
		return nil, fmt.Errorf("open region fail: %w", err)
	}
	defer func(r *region.Region) {
		err2 := r.Close()
		if errRet == nil && err2 != nil {
			errRet = fmt.Errorf("close region fail: %w", err2)
		}
	}(r)

	x, z := region.In(int(pos[0]), int(pos[1]))
	if !r.ExistSector(x, z) {
		return nil, errChunkNotExist
	}

	data, err := r.ReadSector(x, z)
	if err != nil {
		return nil, fmt.Errorf("read sector fail: %w", err)
	}

	var chunk save.Chunk
	if err := chunk.Load(data); err != nil {
		return nil, fmt.Errorf("parse chunk data fail: %w", err)
	}

	c, err = level.ChunkFromSave(&chunk)
	if err != nil {
		return nil, fmt.Errorf("load chunk data fail: %w", err)
	}
	return c, nil
}

func (p *ChunkProvider) getRegion(rx, rz int) (*region.Region, error) {
	filename := fmt.Sprintf("r.%d.%d.mca", rx, rz)
	path := filepath.Join(p.dir, filename)
	r, err := region.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		r, err = region.Create(path)
	}
	return r, err
}

func (p *ChunkProvider) PutChunk(pos [2]int32, c *level.Chunk) (err error) {
	//var chunk save.Chunk
	//err = level.ChunkToSave(c, &chunk)
	//if err != nil {
	//	return fmt.Errorf("encode chunk data fail: %w", err)
	//}
	//
	//data, err := chunk.Data(1)
	//if err != nil {
	//	return fmt.Errorf("record chunk data fail: %w", err)
	//}
	//
	//r, err := p.getRegion(region.At(int(pos[0]), int(pos[1])))
	//if err != nil {
	//	return fmt.Errorf("open region fail: %w", err)
	//}
	//defer func(r *region.Region) {
	//	err2 := r.Close()
	//	if err == nil && err2 != nil {
	//		err = fmt.Errorf("open region fail: %w", err)
	//	}
	//}(r)
	//
	//x, z := region.In(int(pos[0]), int(pos[1]))
	//err = r.WriteSector(x, z, data)
	//if err != nil {
	//	return fmt.Errorf("write sector fail: %w", err)
	//}

	return nil
}

var errChunkNotExist = errors.New("ErrChunkNotExist")

type PlayerProvider struct {
	dir string
}

func NewPlayerProvider(dir string) PlayerProvider {
	return PlayerProvider{dir: dir}
}

func (p *PlayerProvider) GetPlayer(name string, id uuid.UUID, pubKey *user.PublicKey, properties []user.Property) (player *Player, errRet error) {
	f, err := os.Open(filepath.Join(p.dir, id.String()+".dat"))
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		err2 := f.Close()
		if errRet == nil && err2 != nil {
			errRet = fmt.Errorf("close player data fail: %w", err2)
		}
	}(f)
	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("open gzip reader fail: %w", err)
	}
	data, err := save.ReadPlayerData(r)
	if err != nil {
		return nil, fmt.Errorf("read player data fail: %w", err)
	}
	if err := r.Close(); err != nil {
		return nil, fmt.Errorf("close gzip reader fail: %w", err)
	}
	player = &Player{
		Entity: Entity{
			EntityID: NewEntityID(),
			Position: data.Pos,
			Rotation: data.Rotation,
		},
		Name:       name,
		UUID:       id,
		PubKey:     pubKey,
		Properties: properties,
		ChunkPos: [3]int32{
			int32(data.Pos[0]) >> 4,
			int32(data.Pos[1]) >> 4,
			int32(data.Pos[2]) >> 4,
		},
		Gamemode:       data.PlayerGameType,
		EntitiesInView: make(map[int32]*Entity),
		ViewDistance:   10,
	}
	return
}

func (p *PlayerProvider) SavePlayer(player *Player) error {
	if err := os.MkdirAll(p.dir, 0755); err != nil {
		return fmt.Errorf("create playerdata dir fail: %w", err)
	}
	data := save.PlayerData{
		DataVersion: dataVersion,
		Dimension:   "minecraft:overworld",
		Pos:         player.Position,
		Rotation:    player.Rotation,
		OnGround:    boolToByte(bool(player.OnGround)),
		UUID:        uuidToNBT(player.UUID),
		PlayerGameType: player.Gamemode,
		Health:      20,
		FoodLevel:   20,
		Air:         300,
	}
	data.Abilities.MayBuild = 1

	raw, err := nbt.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal player data fail: %w", err)
	}

	path := filepath.Join(p.dir, player.UUID.String()+".dat")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create player data file fail: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if _, err := gw.Write(raw); err != nil {
		return fmt.Errorf("write player data fail: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("close gzip writer fail: %w", err)
	}
	return nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func uuidToNBT(id uuid.UUID) [4]int32 {
	var n [4]int32
	for i := 0; i < 4; i++ {
		n[i] = int32(id[i*4])<<24 | int32(id[i*4+1])<<16 | int32(id[i*4+2])<<8 | int32(id[i*4+3])
	}
	return n
}
