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

const whitelistPath = "whitelist.json"

// Whitelist stores players allowed to join the server.
type Whitelist struct {
	mu      sync.RWMutex
	entries map[string]whitelistEntry // key is lowercase player name
}

type whitelistEntry struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// LoadWhitelist loads the whitelist from whitelist.json.
// If the file does not exist, an empty whitelist is returned and the file is created.
func LoadWhitelist() (*Whitelist, error) {
	w := &Whitelist{
		entries: make(map[string]whitelistEntry),
	}
	if _, err := os.Stat(whitelistPath); errors.Is(err, os.ErrNotExist) {
		return w, w.Save()
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(whitelistPath)
	if err != nil {
		return nil, err
	}
	var entries []whitelistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		w.entries[strings.ToLower(e.Name)] = e
	}
	return w, nil
}

// IsWhitelisted reports whether a player is in the whitelist.
func (w *Whitelist) IsWhitelisted(name string, id uuid.UUID) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	entry, ok := w.entries[strings.ToLower(name)]
	if !ok {
		return false
	}
	// If UUID is zero, only name matters; otherwise check both.
	if entry.UUID == uuid.Nil {
		return true
	}
	return entry.UUID == id
}

// Add adds a player to the whitelist.
func (w *Whitelist) Add(name string, id uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries[strings.ToLower(name)] = whitelistEntry{UUID: id, Name: name}
	return w.Save()
}

// Remove removes a player from the whitelist.
func (w *Whitelist) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.entries, strings.ToLower(name))
	return w.Save()
}

// List returns all whitelist entries.
func (w *Whitelist) List() ([]whitelistEntry, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	entries := make([]whitelistEntry, 0, len(w.entries))
	for _, e := range w.entries {
		entries = append(entries, e)
	}
	return entries, nil
}

// Save writes the whitelist to disk.
func (w *Whitelist) Save() error {
	w.mu.RLock()
	entries := make([]whitelistEntry, 0, len(w.entries))
	for _, e := range w.entries {
		entries = append(entries, e)
	}
	w.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(whitelistPath, data, 0644)
}
