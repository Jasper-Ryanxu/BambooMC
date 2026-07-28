package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/server"
	"github.com/go-mc/server/game"
)

const ServerName = "BambooMC"

var isDebug = flag.Bool("debug", false, "Enable debug log output")

func newLogger(debug bool) (*zap.Logger, error) {
	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}
	cfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "", // 不显示调用位置
		FunctionKey:    "",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    levelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := &cleanEncoder{zapcore.NewConsoleEncoder(cfg)}
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zap.NewAtomicLevelAt(level))
	opts := []zap.Option{zap.ErrorOutput(zapcore.Lock(os.Stderr))}
	if debug {
		opts = append(opts, zap.Development())
	}
	return zap.New(core, opts...), nil
}

// cleanEncoder wraps a console encoder and drops structured fields so that
// logs look like plain console output without JSON-like key/value traces.
type cleanEncoder struct {
	zapcore.Encoder
}

func (e *cleanEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	return e.Encoder.EncodeEntry(entry, nil)
}

func (e *cleanEncoder) Clone() zapcore.Encoder {
	return &cleanEncoder{e.Encoder.Clone()}
}

func levelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch l {
	case zapcore.DebugLevel:
		enc.AppendString("[DEBUG]")
	case zapcore.InfoLevel:
		enc.AppendString("[INFO]")
	case zapcore.WarnLevel:
		enc.AppendString("[WARN]")
	case zapcore.ErrorLevel:
		enc.AppendString("[ERROR]")
	case zapcore.FatalLevel:
		enc.AppendString("[FATAL]")
	default:
		enc.AppendString("[" + l.String() + "]")
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.Parse()
	// initialize log library
	logger := unwrap(newLogger(*isDebug))
	defer func(logger *zap.Logger) {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}(logger)

	logger.Info(fmt.Sprintf("[%s] 服务端启动中...", ServerName))
	printBuildInfo(logger)
	defer logger.Info(fmt.Sprintf("[%s] 服务端已退出", ServerName))

	// EULA 检查
	if err := checkEULA(logger); err != nil {
		logger.Error("EULA 检查失败", zap.Error(err))
		fmt.Println("\n按任意键继续...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return
	}

	// load server config
	config, err := readConfig()
	if err != nil {
		logger.Error("读取配置文件失败", zap.Error(err))
		return
	}

	// initialize player list and server status module, the two modules work together to show server Ping&List information
	playerList := server.NewPlayerList(config.MaxPlayers)
	serverInfo := server.NewPingInfo(
		ServerName+" "+server.ProtocolName,
		server.ProtocolVersion,
		chat.Text(config.MessageOfTheDay),
		nil,
	)

	whitelist, err := game.LoadWhitelist()
	if err != nil {
		logger.Error("加载白名单失败", zap.Error(err))
		return
	}
	opList, err := game.LoadOpList()
	if err != nil {
		logger.Error("加载 OP 列表失败", zap.Error(err))
		return
	}

	gamePlay := game.NewGame(logger, config, playerList, serverInfo, whitelist, opList)

	s := server.Server{
		Logger: zap.NewStdLog(logger),
		ListPingHandler: struct {
			*server.PlayerList
			*server.PingInfo
		}{playerList, serverInfo},
		LoginHandler: &server.MojangLoginHandler{
			OnlineMode:           config.OnlineMode,
			EnforceSecureProfile: config.EnforceSecureProfile,
			Threshold:            config.NetworkCompressionThreshold,
			LoginChecker:         game.NewLoginChecker(playerList, whitelist, config.WhiteList),
		},
		GamePlay: gamePlay,
	}
	listenAddr := config.Address()
	logger.Info("开始监听", zap.String("address", listenAddr))

	done := make(chan struct{})
	go func() {
		if err := s.Listen(listenAddr); err != nil {
			logger.Error("服务器监听错误", zap.Error(err))
			// If binding to a specific IP failed, fall back to all interfaces.
			if config.ServerIP != "" {
				fallback := fmt.Sprintf(":%d", config.ServerPort)
				logger.Warn("尝试回退到所有接口", zap.String("address", fallback))
				if err := s.Listen(fallback); err != nil {
					logger.Error("回退监听仍然失败", zap.Error(err))
				}
			}
		}
		close(done)
	}()

	fmt.Println("控制台已就绪，输入 help 查看可用指令")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := gamePlay.ExecuteCommand(line); err != nil {
			fmt.Println("指令执行失败:", err)
		}
		if line == "stop" || line == "/stop" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error("读取控制台输入失败", zap.Error(err))
	}
	fmt.Println("正在关闭服务器...")
	_ = logger.Sync()
	os.Exit(0)
}

// printBuildInfo reading compile information of the binary program with runtime/debug package，and print it to log
func printBuildInfo(logger *zap.Logger) {
	binaryInfo, _ := debug.ReadBuildInfo()
	settings := make(map[string]string)
	for _, v := range binaryInfo.Settings {
		settings[v.Key] = v.Value
	}
	logger.Debug("Build info", zap.Any("settings", settings))
}

// readConfig read server config from config file. Throw error when meet unknown setting
func readConfig() (game.Config, error) {
	var c game.Config
	const configPath = "config.toml"
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := createDefaultConfig(configPath); err != nil {
			return game.Config{}, fmt.Errorf("create default config fail: %w", err)
		}
	} else if err != nil {
		return game.Config{}, err
	}

	meta, err := toml.DecodeFile(configPath, &c)
	if err != nil {
		return game.Config{}, err
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		var err errUnknownConfig
		for _, key := range undecoded {
			err = append(err, key.String())
		}
		return game.Config{}, err
	}

	return c, nil
}

func createDefaultConfig(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(defaultConfig)
	return err
}

const defaultConfig = `# BambooMC 服务端配置文件
# 修改后重启服务端生效

# 服务器 IP，留空则监听所有接口
server-ip = ""
# 服务器端口
server-port = 25565
# 最大同时在线玩家数
max-players = 20
# 客户端视距（区块）
view-distance = 10
# 模拟距离（区块）
simulation-distance = 10
# 服务器 MOTD
motd = "A BambooMC Server"
# 网络压缩阈值，-1 表示禁用压缩
network-compression-threshold = 256
# 是否开启正版验证
online-mode = true
# 主世界文件夹名称
level-name = "world"
# 世界种子，0 表示随机种子
level-seed = 0
# 是否强制安全聊天配置
enforce-secure-profile = true
# 是否启用白名单
white-list = false
# 默认游戏模式：0=生存，1=创造，2=冒险，3=旁观
gamemode = 0

[chunk-loading-limiter]
# 全局区块加载限速：每 every 时间加载 N 个区块
every = "10ms"
n = 64

[player-chunk-loading-limiter]
# 每个玩家区块加载限速
every = "10ms"
n = 16
`

func checkEULA(logger *zap.Logger) error {
	const eulaPath = "eula.txt"
	if _, err := os.Stat(eulaPath); errors.Is(err, os.ErrNotExist) {
		if err := createDefaultEULA(eulaPath); err != nil {
			return fmt.Errorf("create eula.txt fail: %w", err)
		}
		fmt.Println("========================================")
		fmt.Println("首次启动检测到未同意 Minecraft EULA。")
		fmt.Println("请阅读 eula.txt 文件，")
		fmt.Println("并将 eula=false 改为 eula=true 以表示同意。")
		fmt.Println("========================================")
		return errors.New("eula not accepted")
	} else if err != nil {
		return err
	}

	agreed, err := parseEULA(eulaPath)
	if err != nil {
		return fmt.Errorf("parse eula.txt fail: %w", err)
	}
	if !agreed {
		fmt.Println("========================================")
		fmt.Println("未同意 Minecraft EULA。")
		fmt.Println("请将 eula.txt 中的 eula=false 改为 eula=true。")
		fmt.Println("========================================")
		return errors.New("eula not accepted")
	}
	logger.Info("已同意 Minecraft EULA")
	return nil
}

func createDefaultEULA(path string) error {
	content := `# 使用本服务端即表示您同意 Minecraft 最终用户许可协议 (EULA)。
# 若您不同意，请勿运行此服务器。
# 将 eula 的值改为 true 即表示您同意 EULA。
eula=false
`
	return os.WriteFile(path, []byte(content), 0644)
}

func parseEULA(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "eula=") {
			value := strings.TrimSpace(line[len("eula="):])
			return strconv.ParseBool(strings.ToLower(value))
		}
	}
	return false, errors.New("eula field not found")
}

type errUnknownConfig []string

func (e errUnknownConfig) Error() string {
	return "unknown config keys: [" + strings.Join(e, ", ") + "]"
}

func unwrap[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
