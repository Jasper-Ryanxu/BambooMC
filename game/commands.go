// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package game

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/server"
	"github.com/google/uuid"
	"github.com/go-mc/server/client"
	"github.com/go-mc/server/world"
)

// ExecuteCommand parses and executes a console command.
// The command may be prefixed with '/' or not.
func (g *Game) ExecuteCommand(line string) error {
	return g.executeCommand(consoleOutput{}, line)
}

// ExecutePlayerCommand parses and executes a command sent by a player in chat.
func (g *Game) ExecutePlayerCommand(c *client.Client, line string) error {
	p := c.GetPlayer()
	if !g.opList.IsOp(p.Name, p.UUID) {
		c.SendSystemChat(chat.Text("你没有权限执行此指令"), false)
		return nil
	}
	if err := g.executeCommand(playerOutput{c: c}, line); err != nil {
		c.SendSystemChat(chat.Text("[错误] "+err.Error()), false)
	}
	return nil
}

func (g *Game) executeCommand(out commandOutput, line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "/") {
		line = line[1:]
	}

	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help", "?":
		return g.cmdHelp(out)
	case "say":
		return g.cmdSay(out, args)
	case "kick":
		return g.cmdKick(out, args)
	case "kill":
		return g.cmdKill(out, args)
	case "stop":
		return g.cmdStop(out)
	case "whitelist":
		return g.cmdWhitelist(out, args)
	case "list":
		return g.cmdList(out)
	case "gamemode", "gm":
		return g.cmdGamemode(out, args)
	case "tp", "teleport":
		return g.cmdTeleport(out, args)
	case "time":
		return g.cmdTime(out, args)
	default:
		return fmt.Errorf("未知指令: %s", cmd)
	}
}

func (g *Game) cmdHelp(out commandOutput) error {
	out.Println("可用指令:")
	out.Println("  say <消息>            - 向所有玩家广播消息")
	out.Println("  kick <玩家> [原因]    - 踢出指定玩家")
	out.Println("  kill <玩家>           - 杀死指定玩家")
	out.Println("  list                  - 列出在线玩家")
	out.Println("  gamemode <模式> [玩家] - 修改玩家游戏模式")
	out.Println("  tp <玩家> <目标玩家|x y z> - 传送玩家")
	out.Println("  time <set|add|query> [值] - 设置/查询时间")
	out.Println("  whitelist <add|remove|list|reload> [玩家]")
	out.Println("                        - 管理白名单")
	out.Println("  stop                  - 关闭服务器")
	out.Println("  help                  - 显示本帮助")
	return nil
}

func (g *Game) cmdSay(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: say <消息>")
	}
	msg := chat.Message{Text: "[服务器] " + strings.Join(args, " ")}
	g.globalChat.broadcastSystemChat(msg, false)
	return nil
}

func (g *Game) cmdKick(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: kick <玩家> [原因]")
	}
	name := args[0]
	reason := "你被管理员踢出服务器"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	found := false
	g.playerList.pingList.Range(func(c server.PlayerListClient, sample server.PlayerSample) {
		if strings.EqualFold(sample.Name, name) {
			found = true
			c.SendDisconnect(chat.Text(reason))
		}
	})
	if !found {
		return fmt.Errorf("找不到玩家: %s", name)
	}
	out.Printf("已踢出玩家 %s\n", name)
	return nil
}

func (g *Game) cmdKill(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: kill <玩家>")
	}
	name := args[0]

	found := false
	g.playerList.pingList.Range(func(c server.PlayerListClient, sample server.PlayerSample) {
		if strings.EqualFold(sample.Name, name) {
			found = true
			cc := c.(*client.Client)
			p := cc.GetPlayer()
			cc.SendSystemChat(chat.Text("你已被杀死"), false)
			g.overworld.RespawnPlayer(cc, p)

			deathMsg := chat.TranslateMsg("chat.type.admin", chat.Text(p.Name), chat.Text("被杀死")).SetColor(chat.Yellow)
			g.globalChat.broadcastSystemChat(deathMsg, false)
		}
	})
	if !found {
		return fmt.Errorf("找不到玩家: %s", name)
	}
	out.Printf("已杀死玩家 %s\n", name)
	return nil
}

func (g *Game) cmdStop(out commandOutput) error {
	g.globalChat.broadcastSystemChat(chat.Text("服务器正在关闭..."), false)
	out.Println("正在关闭服务器...")
	return nil
}

func (g *Game) cmdList(out commandOutput) error {
	var names []string
	g.playerList.pingList.Range(func(_ server.PlayerListClient, sample server.PlayerSample) {
		names = append(names, sample.Name)
	})
	out.Printf("在线玩家 (%d/%d): %s\n", len(names), g.config.MaxPlayers, strings.Join(names, ", "))
	return nil
}

func (g *Game) cmdGamemode(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: gamemode <模式> [玩家]")
	}
	modeStr := strings.ToLower(args[0])
	var mode int32
	switch modeStr {
	case "0", "s", "survival":
		mode = 0
	case "1", "c", "creative":
		mode = 1
	case "2", "a", "adventure":
		mode = 2
	case "3", "sp", "spectator":
		mode = 3
	default:
		return fmt.Errorf("未知的游戏模式: %s", modeStr)
	}

	var target string
	if len(args) >= 2 {
		target = args[1]
	} else {
		// 控制台必须指定玩家
		return errors.New("用法: gamemode <模式> <玩家>")
	}

	c, p := g.findOnlinePlayer(target)
	if c == nil {
		return fmt.Errorf("找不到玩家: %s", target)
	}
	p.Gamemode = mode
	c.SendGameEvent(3, float32(mode))
	c.SendPlayerInfoUpdate(client.NewPlayerInfoAction(client.PlayerInfoUpdateGameMode), []*world.Player{p})
	c.SendSystemChat(chat.Text(fmt.Sprintf("你的游戏模式已更新为 %d", mode)), false)
	out.Printf("已将 %s 的游戏模式设置为 %d\n", p.Name, mode)
	return nil
}

func (g *Game) cmdTeleport(out commandOutput, args []string) error {
	if len(args) < 2 {
		return errors.New("用法: tp <玩家> <目标玩家|x y z>")
	}
	sourceName := args[0]
	c, p := g.findOnlinePlayer(sourceName)
	if c == nil {
		return fmt.Errorf("找不到玩家: %s", sourceName)
	}

	var newPos [3]float64
	var newRot [2]float32
	if len(args) == 2 {
		// tp <玩家> <目标玩家>
		targetName := args[1]
		_, tp := g.findOnlinePlayer(targetName)
		if tp == nil {
			return fmt.Errorf("找不到目标玩家: %s", targetName)
		}
		newPos = tp.Position
		newRot = tp.Rotation
		out.Printf("已将 %s 传送至 %s\n", sourceName, targetName)
	} else if len(args) == 4 {
		// tp <玩家> <x> <y> <z>
		x, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("坐标 X 无效: %s", args[1])
		}
		y, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			return fmt.Errorf("坐标 Y 无效: %s", args[2])
		}
		z, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			return fmt.Errorf("坐标 Z 无效: %s", args[3])
		}
		newPos = [3]float64{x, y, z}
		newRot = p.Rotation
		out.Printf("已将 %s 传送至 %.2f %.2f %.2f\n", sourceName, x, y, z)
	} else {
		return errors.New("用法: tp <玩家> <目标玩家|x y z>")
	}

	p.Position = newPos
	p.Rotation = newRot
	p.ChunkPos = [3]int32{int32(newPos[0]) >> 4, int32(newPos[1]) >> 4, int32(newPos[2]) >> 4}
	c.SendPlayerPosition(newPos, newRot)
	c.SendSetChunkCacheCenter([2]int32{p.ChunkPos[0], p.ChunkPos[2]})
	return nil
}

func (g *Game) cmdTime(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: time <set|add|query> [值]")
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "set":
		if len(args) < 2 {
			return errors.New("用法: time set <数值|day|noon|night|midnight>")
		}
		var t int64
		switch strings.ToLower(args[1]) {
		case "day":
			t = 1000
		case "noon":
			t = 6000
		case "night":
			t = 13000
		case "midnight":
			t = 18000
		default:
			v, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("时间值无效: %s", args[1])
			}
			t = v
		}
		g.overworld.SetTime(t)
		out.Printf("时间已设置为 %d\n", t)
	case "add":
		if len(args) < 2 {
			return errors.New("用法: time add <数值>")
		}
		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("时间值无效: %s", args[1])
		}
		g.overworld.AddTime(v)
		out.Printf("时间已增加 %d\n", v)
	case "query":
		out.Printf("当前时间: %d\n", g.overworld.GetTime())
	default:
		return fmt.Errorf("未知的时间子指令: %s", sub)
	}
	return nil
}

func (g *Game) findOnlinePlayer(name string) (*client.Client, *world.Player) {
	var found *client.Client
	var player *world.Player
	g.playerList.pingList.Range(func(c server.PlayerListClient, sample server.PlayerSample) {
		if strings.EqualFold(sample.Name, name) {
			found = c.(*client.Client)
			player = found.GetPlayer()
		}
	})
	return found, player
}

func (g *Game) cmdWhitelist(out commandOutput, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: whitelist <add|remove|list|reload> [玩家]")
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "add":
		if len(args) < 2 {
			return errors.New("用法: whitelist add <玩家>")
		}
		name := args[1]
		if err := g.whitelist.Add(name, uuid.UUID{}); err != nil {
			return err
		}
		out.Printf("已将 %s 加入白名单\n", name)
	case "remove":
		if len(args) < 2 {
			return errors.New("用法: whitelist remove <玩家>")
		}
		name := args[1]
		if err := g.whitelist.Remove(name); err != nil {
			return err
		}
		out.Printf("已将 %s 从白名单移除\n", name)
	case "list":
		entries, err := g.whitelist.List()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			out.Println("白名单为空")
			return nil
		}
		out.Printf("白名单共 %d 人:\n", len(entries))
		for _, e := range entries {
			out.Printf("  - %s\n", e.Name)
		}
	case "reload":
		w, err := LoadWhitelist()
		if err != nil {
			return err
		}
		g.whitelist = w
		out.Println("白名单已重新加载")
	default:
		return fmt.Errorf("未知白名单子指令: %s", sub)
	}
	return nil
}
