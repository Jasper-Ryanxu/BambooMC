// This file is part of BambooMC server project.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package game

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const opPath = "ops.json"

// OpList stores server operators.
type OpList struct {
	mu      sync.RWMutex
	entries map[string]opEntry // key is lowercase player name
}

type opEntry struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Level int32     `json:"level"`
}

// LoadOpList loads operators from ops.json.
// If the file does not exist, an empty list is returned and the file is created.
func LoadOpList() (*OpList, error) {
	o := &OpList{
		entries: make(map[string]opEntry),
	}
	if _, err := os.Stat(opPath); errors.Is(err, os.ErrNotExist) {
		return o, o.Save()
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(opPath)
	if err != nil {
		return nil, err
	}
	var entries []opEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		o.entries[strings.ToLower(e.Name)] = e
	}
	return o, nil
}

// IsOp reports whether a player is an operator.
func (o *OpList) IsOp(name string, id uuid.UUID) bool {
	if o == nil {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	entry, ok := o.entries[strings.ToLower(name)]
	if !ok {
		return false
	}
	if entry.UUID == uuid.Nil {
		return true
	}
	return entry.UUID == id
}

// Add adds a player as operator.
func (o *OpList) Add(name string, id uuid.UUID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.entries[strings.ToLower(name)] = opEntry{UUID: id, Name: name, Level: 4}
	return o.Save()
}

// Remove removes a player from operators.
func (o *OpList) Remove(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.entries, strings.ToLower(name))
	return o.Save()
}

// List returns all operators.
func (o *OpList) List() ([]opEntry, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	entries := make([]opEntry, 0, len(o.entries))
	for _, e := range o.entries {
		entries = append(entries, e)
	}
	return entries, nil
}

// Save writes the operator list to disk.
func (o *OpList) Save() error {
	o.mu.RLock()
	entries := make([]opEntry, 0, len(o.entries))
	for _, e := range o.entries {
		entries = append(entries, e)
	}
	o.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(opPath, data, 0644)
}
