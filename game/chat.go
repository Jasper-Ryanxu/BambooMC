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
	"io"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/chat/sign"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
	"github.com/Tnze/go-mc/server"
	"github.com/go-mc/server/client"
)

const MsgExpiresTime = time.Minute * 5

type globalChat struct {
	log           *zap.Logger
	players       *playerList
	chatTypeCodec *registry.Registry[registry.ChatType]
	game          *Game
}

func (g *globalChat) broadcastSystemChat(msg chat.Message, overlay bool) {
	g.log.Info(msg.ClearString())
	g.players.pingList.Range(func(c server.PlayerListClient, _ server.PlayerSample) {
		c.(*client.Client).SendSystemChat(msg, overlay)
	})
}

func (g *globalChat) Handle(p pk.Packet, c *client.Client) error {
	var (
		message       pk.String
		timestampLong pk.Long
		salt          pk.Long
		signature     pk.Option[sign.Signature, *sign.Signature]
		lastSeen      sign.HistoryUpdate
	)
	err := p.Scan(
		&message,
		&timestampLong,
		&salt,
		&signature,
		&lastSeen,
	)
	if err != nil {
		return err
	}

	player := c.GetPlayer()
	timestamp := time.UnixMilli(int64(timestampLong))
	logger := g.log.With(
		zap.String("sender", player.Name),
		zap.Time("timestamp", timestamp),
	)

	if existInvalidCharacter(string(message)) {
		c.SendDisconnect(chat.TranslateMsg("multiplayer.disconnect.illegal_characters"))
		return nil
	}

	if !player.SetLastChatTimestamp(timestamp) {
		c.SendDisconnect(chat.TranslateMsg("multiplayer.disconnect.out_of_order_chat"))
		return nil
	}

	// TODO: check if the client disable chatting
	if false {
		c.SendSystemChat(chat.TranslateMsg("chat.disabled.options").SetColor(chat.Red), false)
		return nil
	}

	// verify message
	//var playerMsg sign.PlayerMessage
	////if player.PubKey != nil {
	////}

	if time.Since(timestamp) > MsgExpiresTime {
		logger.Warn("Player send expired message", zap.String("msg", string(message)))
		return nil
	}
	chatTypeID, decorator := g.chatTypeCodec.Find("minecraft:chat")
	chatType := chat.Type{
		ID:         chatTypeID,
		SenderName: chat.Text(player.Name),
		TargetName: nil,
	}
	decorated := chatType.Decorate(chat.Text(string(message)), &decorator.Chat)
	logger.Info(decorated.ClearString())

	// 兼容没有收到命令树的客户端：将以 / 开头的普通聊天消息当作指令处理。
	msgStr := string(message)
	if strings.HasPrefix(msgStr, "/") && g.game != nil {
		return g.game.ExecutePlayerCommand(c, msgStr)
	}

	// 1.19.4 的聊天签名验证较复杂，为兼容所有客户端，以系统消息形式广播。
	// 装饰后的消息仍会以 <玩家名> 消息 的形式显示。
	g.players.pingList.Range(func(c server.PlayerListClient, _ server.PlayerSample) {
		c.(*client.Client).SendSystemChat(decorated, false)
	})
	return nil
}

func (g *globalChat) handleCommand(p pk.Packet, c *client.Client) error {
	var (
		command       pk.String
		timestampLong pk.Long
		salt          pk.Long
		argSigs       argumentSignatures
		signedPreview pk.Boolean
		lastSeen      sign.HistoryUpdate
	)
	if err := p.Scan(&command, &timestampLong, &salt, &argSigs, &signedPreview, &lastSeen); err != nil {
		return err
	}
	if g.game != nil {
		return g.game.ExecutePlayerCommand(c, string(command))
	}
	return nil
}

// argumentSignatures matches the ServerboundChatCommand argument signatures field.
type argumentSignatures []argumentSignature

type argumentSignature struct {
	Name      pk.String
	Signature pk.ByteArray
}

func (a *argumentSignatures) ReadFrom(r io.Reader) (n int64, err error) {
	var count pk.VarInt
	n, err = count.ReadFrom(r)
	if err != nil {
		return
	}
	*a = make(argumentSignatures, count)
	for i := range *a {
		nn, err := pk.Tuple{&(*a)[i].Name, &(*a)[i].Signature}.ReadFrom(r)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return
}

func (a argumentSignatures) WriteTo(w io.Writer) (int64, error) {
	return pk.Tuple{pk.VarInt(len(a)), pk.Array(a)}.WriteTo(w)
}

func (a argumentSignature) WriteTo(w io.Writer) (int64, error) {
	return pk.Tuple{a.Name, a.Signature}.WriteTo(w)
}

func existInvalidCharacter(msg string) bool {
	for _, c := range msg {
		if c == '§' || c < ' ' || c == '\x7F' {
			return true
		}
	}
	return false
}
