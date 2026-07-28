// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package world

import (
	"bytes"
	"math"

	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	"github.com/aquilax/go-perlin"
)

const (
	worldMinY    = -64
	worldMaxY    = 320
	seaLevel     = 63
	sectionCount = 24
)

// generateTerrain fills a chunk with Perlin-noise based terrain.
func generateTerrain(c *level.Chunk, pos [2]int32, seed int64) {
	baseNoise := perlin.NewPerlin(2, 2, 1, seed)
	hillNoise := perlin.NewPerlin(2, 2, 2, seed+1)
	detailNoise := perlin.NewPerlin(2, 2, 3, seed+2)
	riverNoise := perlin.NewPerlin(2, 2, 1, seed+3)

	stone := block.ToStateID[block.Stone{}]
	dirt := block.ToStateID[block.Dirt{}]
	grass := block.ToStateID[block.GrassBlock{}]
	bedrock := block.ToStateID[block.Bedrock{}]
	sand := block.ToStateID[block.Sand{}]
	gravel := block.ToStateID[block.Gravel{}]
	water := block.ToStateID[block.Water{}]

	for lx := 0; lx < 16; lx++ {
		for lz := 0; lz < 16; lz++ {
			wx := float64(pos[0]*16 + int32(lx))
			wz := float64(pos[1]*16 + int32(lz))

			h := terrainHeight(wx, wz, baseNoise, hillNoise, detailNoise)
			riverFactor := riverFactor(wx, wz, riverNoise)

			height := int32(h)
			riverBed := int32(seaLevel - 6)
			if riverBed < worldMinY+1 {
				riverBed = worldMinY + 1
			}

			// carve river valleys
			if riverFactor > 0 {
				height = int32(lerp(float64(height), float64(riverBed), riverFactor))
			}

			if height > worldMaxY-1 {
				height = worldMaxY - 1
			}
			if height < worldMinY+1 {
				height = worldMinY + 1
			}

			for y := int32(worldMinY); y <= int32(worldMaxY); y++ {
				var stateID block.StateID
				switch {
				case y == int32(worldMinY):
					stateID = bedrock
				case y < height-3:
					stateID = stone
				case y < height:
					stateID = dirt
				case y == height:
					if height <= seaLevel+1 {
						stateID = sand
					} else if riverFactor > 0.3 {
						stateID = gravel
					} else {
						stateID = grass
					}
				case y <= seaLevel:
					// fill water in rivers / oceans
					stateID = water
				default:
					stateID = 0 // air
				}
				if stateID != 0 {
					setBlock(c, lx, int(y), lz, stateID)
				}
			}
		}
	}
	c.Status = level.StatusFull
	initChunkLight(c)
}

// initChunkLight fills every section with full sky light and zero block light.
// This prevents chunks from appearing completely dark on the client.
func initChunkLight(c *level.Chunk) {
	fullSky := bytes.Repeat([]byte{0xFF}, 2048)
	noBlock := make([]byte, 2048)
	for i := range c.Sections {
		if c.Sections[i].SkyLight == nil {
			c.Sections[i].SkyLight = append([]byte(nil), fullSky...)
		}
		if c.Sections[i].BlockLight == nil {
			c.Sections[i].BlockLight = append([]byte(nil), noBlock...)
		}
	}
}

func terrainHeight(x, z float64, baseNoise, hillNoise, detailNoise *perlin.Perlin) float64 {
	h := 64.0
	// large continents
	h += baseNoise.Noise2D(x/600.0, z/600.0) * 32.0
	// rolling hills
	h += hillNoise.Noise2D(x/200.0, z/200.0) * 18.0
	h += hillNoise.Noise2D(x/100.0, z/100.0) * 9.0
	// small details
	h += detailNoise.Noise2D(x/50.0, z/50.0) * 4.0
	h += detailNoise.Noise2D(x/25.0, z/25.0) * 2.0
	return h
}

// FindSuitableSpawnPosition searches around (0,0) for a solid, dry spawn point.
// It returns the block above the surface and falls back to (0, height+1, 0).
func FindSuitableSpawnPosition(seed int64) (x, y, z int32) {
	const searchRadius = 256
	for r := int32(0); r <= searchRadius; r += 16 {
		for dx := -r; dx <= r; dx += 16 {
			for dz := -r; dz <= r; dz += 16 {
				if dx != -r && dx != r && dz != -r && dz != r {
					continue
				}
				h := GetTerrainHeight(dx, dz, seed)
				// prefer dry land above sea level with room above
				if h > seaLevel+1 {
					return dx, h + 1, dz
				}
			}
		}
	}
	h := GetTerrainHeight(0, 0, seed)
	return 0, h + 1, 0
}

// GetTerrainHeight returns the surface Y at the given world coordinates.
func GetTerrainHeight(x, z int32, seed int64) int32 {
	baseNoise := perlin.NewPerlin(2, 2, 1, seed)
	hillNoise := perlin.NewPerlin(2, 2, 2, seed+1)
	detailNoise := perlin.NewPerlin(2, 2, 3, seed+2)
	riverNoise := perlin.NewPerlin(2, 2, 1, seed+3)

	h := terrainHeight(float64(x), float64(z), baseNoise, hillNoise, detailNoise)
	rf := riverFactor(float64(x), float64(z), riverNoise)
	height := int32(h)
	riverBed := int32(seaLevel - 6)
	if riverBed < worldMinY+1 {
		riverBed = worldMinY + 1
	}
	if rf > 0 {
		height = int32(lerp(float64(height), float64(riverBed), rf))
	}
	if height > worldMaxY-1 {
		height = worldMaxY - 1
	}
	if height < worldMinY+1 {
		height = worldMinY + 1
	}
	return height
}

// riverFactor returns 0 when there is no river, and up to 1 in the river bed.
func riverFactor(x, z float64, riverNoise *perlin.Perlin) float64 {
	r := riverNoise.Noise2D(x/450.0, z/450.0)
	// carve a narrow channel around the zero crossings
	const width = 0.12
	dist := math.Abs(r)
	if dist > width {
		return 0
	}
	// smooth transition from bank to river bed
	t := dist / width
	return smoothstep(1, 0, t)
}

func smoothstep(edge0, edge1, x float64) float64 {
	t := (x - edge0) / (edge1 - edge0)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// setBlock writes a block state into the chunk at local/absolute coordinates.
// lx, lz are 0..15; y is absolute world Y.
func setBlock(c *level.Chunk, lx, y, lz int, stateID block.StateID) {
	if y < worldMinY || y > worldMaxY {
		return
	}
	secIdx := (y - worldMinY) / 16
	if secIdx < 0 || secIdx >= sectionCount {
		return
	}
	localY := y - worldMinY - secIdx*16
	idx := ((localY * 16) + lz) * 16 + lx
	c.Sections[secIdx].SetBlock(idx, stateID)
}
