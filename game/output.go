// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package game

import (
	"fmt"

	"github.com/Tnze/go-mc/chat"
	"github.com/go-mc/server/client"
)

// commandOutput is where command feedback is written.
type commandOutput interface {
	Printf(format string, args ...interface{})
	Println(args ...interface{})
	SendChat(msg string)
}

type consoleOutput struct{}

func (consoleOutput) Printf(format string, args ...interface{}) { fmt.Printf(format, args...) }
func (consoleOutput) Println(args ...interface{})                { fmt.Println(args...) }
func (consoleOutput) SendChat(msg string)                        { /* no-op for console */ }

type playerOutput struct {
	c *client.Client
}

func (p playerOutput) Printf(format string, args ...interface{}) {
	p.c.SendSystemChat(chat.Text(fmt.Sprintf(format, args...)), false)
}

func (p playerOutput) Println(args ...interface{}) {
	p.c.SendSystemChat(chat.Text(fmt.Sprint(args...)), false)
}

func (p playerOutput) SendChat(msg string) {
	p.c.SendSystemChat(chat.Text(msg), false)
}
