// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package game

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/server"
	"github.com/google/uuid"
)

// loginChecker combines player limit and whitelist checks.
type loginChecker struct {
	playerList *server.PlayerList
	whitelist  *Whitelist
	enabled    bool
}

// NewLoginChecker creates a LoginChecker that checks player limit and whitelist.
func NewLoginChecker(playerList *server.PlayerList, whitelist *Whitelist, enabled bool) server.LoginChecker {
	return &loginChecker{
		playerList: playerList,
		whitelist:  whitelist,
		enabled:    enabled,
	}
}

func (c *loginChecker) CheckPlayer(name string, id uuid.UUID, protocol int32) (ok bool, reason chat.Message) {
	if ok, reason = c.playerList.CheckPlayer(name, id, protocol); !ok {
		return false, reason
	}
	if c.enabled {
		if !c.whitelist.IsWhitelisted(name, id) {
			return false, chat.TranslateMsg("multiplayer.disconnect.not_whitelisted")
		}
	}
	return true, chat.Message{}
}
