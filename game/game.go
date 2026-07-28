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

package game

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/save"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/yggdrasil/user"
	"github.com/go-mc/server/client"
	"github.com/go-mc/server/world"
)

const dataVersion = 3337 // Minecraft 1.19.4

type Game struct {
	log *zap.Logger

	config     Config
	serverInfo *server.PingInfo
	whitelist  *Whitelist
	opList     *OpList

	playerProvider world.PlayerProvider
	overworld      *world.World

	globalChat globalChat
	*playerList
}

func NewGame(log *zap.Logger, config Config, pingList *server.PlayerList, serverInfo *server.PingInfo, whitelist *Whitelist, opList *OpList) *Game {
	log.Info("正在加载主世界...", zap.String("level", config.LevelName))

	// providers
	overworld, err := createWorld(log, filepath.Join(".", config.LevelName), &config)
	if err != nil {
		log.Fatal("无法加载主世界", zap.Error(err))
	}
	overworld.PreloadSpawnChunks(config.ViewDistance)
	playerProvider := world.NewPlayerProvider(filepath.Join(".", config.LevelName, "playerdata"))

	// keepalive
	keepAlive := server.NewKeepAlive()
	pl := playerList{pingList: pingList, keepAlive: keepAlive}
	keepAlive.AddPlayerDelayUpdateHandler(func(c server.KeepAliveClient, latency time.Duration) {
		pl.updateLatency(c.(*client.Client), latency)
	})
	go keepAlive.Run(context.TODO())

	g := &Game{
		log: log.Named("game"),

		config:     config,
		serverInfo: serverInfo,
		whitelist:  whitelist,
		opList:     opList,

		playerProvider: playerProvider,
		overworld:      overworld,

		globalChat: globalChat{
			log:           log.Named("chat"),
			players:       &pl,
			chatTypeCodec: &world.NetworkCodec.ChatType,
		},
		playerList: &pl,
	}
	g.globalChat.game = g
	return g
}

func createWorld(logger *zap.Logger, path string, config *Config) (*world.World, error) {
	levelPath := filepath.Join(path, "level.dat")
	if _, err := os.Stat(levelPath); errors.Is(err, os.ErrNotExist) {
		logger.Info("未找到世界数据，正在创建新世界", zap.String("path", path))
		if err := createDefaultWorld(path, config); err != nil {
			return nil, fmt.Errorf("create default world fail: %w", err)
		}
	} else if err != nil {
		return nil, err
	}

	f, err := os.Open(levelPath)
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	lv, err := save.ReadLevel(r)
	if err != nil {
		return nil, err
	}
	overworld := world.New(
		logger.Named("overworld"),
		world.NewProvider(filepath.Join(path, "region"), config.ChunkLoadingLimiter.Limiter()),
		world.Config{
			ViewDistance:  config.ViewDistance,
			SpawnAngle:    lv.Data.SpawnAngle,
			SpawnPosition: [3]int32{lv.Data.SpawnX, lv.Data.SpawnY, lv.Data.SpawnZ},
			Seed:          config.LevelSeed,
		},
	)
	return overworld, nil
}

func createDefaultWorld(path string, config *Config) error {
	dirs := []string{
		path,
		filepath.Join(path, "region"),
		filepath.Join(path, "playerdata"),
		filepath.Join(path, "data"),
		filepath.Join(path, "entities"),
		filepath.Join(path, "poi"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s fail: %w", dir, err)
		}
	}

	spawnX, spawnY, spawnZ := world.FindSuitableSpawnPosition(config.LevelSeed)
	lv := save.Level{
		Data: save.LevelData{
			AllowCommands:    1,
			DataVersion:      dataVersion,
			Difficulty:       1, // 固定为简单
			DifficultyLocked: false,
			GameRules: map[string]string{
				"doDaylightCycle":     "true",
				"doWeatherCycle":      "true",
				"keepInventory":       "false",
				"doMobSpawning":       "true",
				"doEntityDrops":       "true",
				"doFireTick":          "true",
				"doImmediateRespawn":  "false",
				"doLimitedCrafting":   "false",
				"doPatrolSpawning":    "true",
				"doTileDrops":         "true",
				"doTraderSpawning":    "true",
				"doWardenSpawning":    "true",
				"drowningDamage":      "true",
				"fallDamage":          "true",
				"fireDamage":          "true",
				"freezeDamage":        "true",
				"mobGriefing":         "true",
				"naturalRegeneration": "true",
				"showDeathMessages":   "true",
				"spawnRadius":         "10",
				"spectatorsGenerateChunks": "true",
				"tntExplodes":         "true",
			},
			WorldGenSettings: save.WorldGenSettings{
				BonusChest:       false,
				GenerateFeatures: true,
				Seed:             config.LevelSeed,
				Dimensions:       save.DefaultDimensionsGenerators,
			},
			GameType:     config.Gamemode,
			HardCore:     false,
			Initialized:  true,
			LastPlayed:   time.Now().UnixMilli(),
			LevelName:    config.LevelName,
			MapFeatures:  true,
			RandomSeed:   config.LevelSeed,
			SpawnAngle:   0,
			SpawnX:       spawnX,
			SpawnY:       spawnY,
			SpawnZ:       spawnZ,
			Time:         0,
			DayTime:      0,
			Version: struct {
				ID       int32 `nbt:"Id"`
				Name     string
				Series   string
				Snapshot byte
			}{
				ID:       dataVersion,
				Name:     "1.19.4",
				Series:   "main",
				Snapshot: 0,
			},
			StorageVersion: dataVersion,
		},
	}
	if lv.Data.RandomSeed == 0 {
		lv.Data.RandomSeed = time.Now().UnixNano()
		lv.Data.WorldGenSettings.Seed = lv.Data.RandomSeed
	}

	data, err := nbt.Marshal(lv)
	if err != nil {
		return fmt.Errorf("marshal level data fail: %w", err)
	}

	f, err := os.Create(filepath.Join(path, "level.dat"))
	if err != nil {
		return fmt.Errorf("create level.dat fail: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if _, err := gw.Write(data); err != nil {
		return fmt.Errorf("write level.dat fail: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("close level.dat gzip writer fail: %w", err)
	}

	return nil
}

// AcceptPlayer will be called in an independent goroutine when new player login
func (g *Game) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	logger := g.log.With(
		zap.String("name", name),
		zap.String("uuid", id.String()),
		zap.Int32("protocol", protocol),
	)

	spawnPos, spawnAngle := g.overworld.SpawnPositionAndAngle()
	p, err := g.playerProvider.GetPlayer(name, id, profilePubKey, properties)
	if errors.Is(err, os.ErrNotExist) {
		p = &world.Player{
			Entity: world.Entity{
				EntityID: world.NewEntityID(),
				Position: [3]float64{float64(spawnPos[0]), float64(spawnPos[1]), float64(spawnPos[2])},
				Rotation: [2]float32{spawnAngle, 0},
			},
			Name:           name,
			UUID:           id,
			PubKey:         profilePubKey,
			Properties:     properties,
			Gamemode:       g.config.Gamemode,
			ChunkPos:       [3]int32{spawnPos[0] >> 4, spawnPos[1] >> 4, spawnPos[2] >> 4},
			EntitiesInView: make(map[int32]*world.Entity),
			ViewDistance:   g.config.ViewDistance,
		}
	} else if err != nil {
		logger.Error("读取玩家数据失败", zap.Error(err))
		return
	}
	c := client.New(logger, conn, p)

	logger.Info("玩家加入", zap.Int32("eid", p.EntityID))
	defer func() {
		if err := g.playerProvider.SavePlayer(p); err != nil {
			logger.Error("保存玩家数据失败", zap.Error(err))
		}
	}()
	defer logger.Info("玩家离开")

	c.SendLogin(g.overworld, p)
	c.SendWorldTime(g.overworld.GetTime(), g.overworld.GetTime())
	c.SendServerData(g.serverInfo.Description(), g.serverInfo.FavIcon(), g.config.EnforceSecureProfile)

	joinMsg := chat.TranslateMsg("multiplayer.player.joined", chat.Text(p.Name)).SetColor(chat.Yellow)
	leftMsg := chat.TranslateMsg("multiplayer.player.left", chat.Text(p.Name)).SetColor(chat.Yellow)
	g.globalChat.broadcastSystemChat(joinMsg, false)
	defer g.globalChat.broadcastSystemChat(leftMsg, false)
	c.AddHandler(packetid.ServerboundChat, g.globalChat.Handle)
	c.AddHandler(packetid.ServerboundChatCommand, g.globalChat.handleCommand)
	g.registerPlayerActionHandler(c)

	g.playerList.addPlayer(c, p)
	defer g.playerList.removePlayer(c)

	c.SendPlayerPosition(p.Position, p.Rotation)
	g.overworld.AddPlayer(c, p, g.config.PlayerChunkLoadingLimiter.Limiter())
	defer g.overworld.RemovePlayer(c, p)
	c.SendPacket(packetid.ClientboundUpdateTags, pk.Array(defaultTags))
	c.SendSetDefaultSpawnPosition(g.overworld.SpawnPositionAndAngle())

	c.Start()
}
