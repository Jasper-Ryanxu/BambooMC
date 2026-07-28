package game

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	ServerIP                    string `toml:"server-ip"`
	ServerPort                  int    `toml:"server-port"`
	MaxPlayers                  int    `toml:"max-players"`
	ViewDistance                int32  `toml:"view-distance"`
	SimulationDistance          int32  `toml:"simulation-distance"`
	MessageOfTheDay             string `toml:"motd"`
	NetworkCompressionThreshold int    `toml:"network-compression-threshold"`
	OnlineMode                  bool   `toml:"online-mode"`
	LevelName                   string `toml:"level-name"`
	LevelSeed                   int64  `toml:"level-seed"`
	EnforceSecureProfile        bool   `toml:"enforce-secure-profile"`
	WhiteList                   bool   `toml:"white-list"`
	Gamemode                    int32  `toml:"gamemode"`

	ChunkLoadingLimiter       Limiter `toml:"chunk-loading-limiter"`
	PlayerChunkLoadingLimiter Limiter `toml:"player-chunk-loading-limiter"`
}

// Address returns the listen address combined from ServerIP and ServerPort.
func (c Config) Address() string {
	ip := strings.TrimSpace(c.ServerIP)
	if ip == "" {
		return fmt.Sprintf(":%d", c.ServerPort)
	}
	return fmt.Sprintf("%s:%d", ip, c.ServerPort)
}

type Limiter struct {
	Every duration `toml:"every"`
	N     int
}

// Limiter convert this to *rate.Limiter
func (l *Limiter) Limiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(l.Every.Duration), l.N)
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) (err error) {
	d.Duration, err = time.ParseDuration(string(text))
	return
}
