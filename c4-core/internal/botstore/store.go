// Package botstore manages Telegram bot configuration files.
//
// Bot data is stored as JSON files under two roots:
//   - Project-local: .c4/bots/{username}/
//   - Global:        ~/.claude/bots/{username}/
//
// Each bot directory contains:
//   - config.json  – bot identity (username, token, display_name)
//   - access.json  – runtime metadata (last_active, scope)
package botstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ErrNotFound is returned when a bot username does not exist in either root.
var ErrNotFound = errors.New("bot not found")

// Bot holds all persisted information about a single Telegram bot.
type Bot struct {
	// From config.json
	Username    string `json:"username"`
	Token       string `json:"token"`
	DisplayName string `json:"display_name,omitempty"`

	// From access.json
	LastActive time.Time `json:"last_active,omitempty"`
	Scope      string    `json:"scope,omitempty"` // "project" or "global"
	AllowFrom  []int64   `json:"allow_from,omitempty"` // Telegram user/chat IDs allowed to send commands

	// Internal — not persisted
	root string
}

// configFile is the on-disk shape of config.json.
type configFile struct {
	Username    string `json:"username"`
	Token       string `json:"token"`
	DisplayName string `json:"display_name,omitempty"`
}

// accessFile is the on-disk shape of access.json.
type accessFile struct {
	LastActive time.Time `json:"last_active,omitempty"`
	Scope      string    `json:"scope,omitempty"`
	AllowFrom  []int64   `json:"allow_from,omitempty"`
}

// Store manages bot configurations across project-local and global directories.
type Store struct {
	projectRoot string // .c4/bots/  (empty string if no project context)
	globalRoot  string // ~/.claude/bots/
}

// globalBotsDir returns ~/.claude/bots/.
func globalBotsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "bots"), nil
}

// New creates a Store.
// projectDir should be the project root (the directory containing .c4/).
// Pass an empty string to skip the project-local root.
func New(projectDir string) (*Store, error) {
	global, err := globalBotsDir()
	if err != nil {
		return nil, err
	}
	s := &Store{globalRoot: global}
	if projectDir != "" {
		s.projectRoot = filepath.Join(projectDir, ".c4", "bots")
	}
	return s, nil
}

// inferScope returns "project" or "global" based on root index.
func (s *Store) inferScope(rootIndex int) string {
	if rootIndex == 0 && s.projectRoot != "" {
		return "project"
	}
	return "global"
}

// roots returns the search order: project first, then global.
func (s *Store) roots() []string {
	if s.projectRoot != "" {
		return []string{s.projectRoot, s.globalRoot}
	}
	return []string{s.globalRoot}
}

// botDir returns the directory for a given username under a root.
func botDir(root, username string) string {
	return filepath.Join(root, username)
}

// readBot loads config.json + access.json from dir into a Bot.
func readBot(dir string) (*Bot, error) {
	cfgPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config.json: %w", err)
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}

	bot := &Bot{
		Username:    cfg.Username,
		Token:       cfg.Token,
		DisplayName: cfg.DisplayName,
		root:        filepath.Dir(dir),
	}

	accPath := filepath.Join(dir, "access.json")
	if accData, err := os.ReadFile(accPath); err == nil {
		var acc accessFile
		if jsonErr := json.Unmarshal(accData, &acc); jsonErr == nil {
			bot.LastActive = acc.LastActive
			bot.Scope = acc.Scope
			bot.AllowFrom = acc.AllowFrom
		}
	}
	return bot, nil
}

// List returns all bots across project + global roots, merged and sorted by
// last_active descending (most-recently-used first).
// Duplicate usernames: project-local entry wins.
func (s *Store) List() ([]Bot, error) {
	seen := make(map[string]bool)
	var bots []Bot

	for i, root := range s.roots() {
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", root, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			username := e.Name()
			if seen[username] {
				continue
			}
			seen[username] = true
			dir := botDir(root, username)
			bot, err := readBot(dir)
			if err != nil {
				continue // skip malformed entries silently
			}
			if bot.Scope == "" {
				bot.Scope = s.inferScope(i)
			}
			bots = append(bots, *bot)
		}
	}

	sort.Slice(bots, func(i, j int) bool {
		return bots[i].LastActive.After(bots[j].LastActive)
	})
	return bots, nil
}

// Get returns the bot with the given username.
// Project-local takes precedence over global.
func (s *Store) Get(username string) (*Bot, error) {
	for i, root := range s.roots() {
		dir := botDir(root, username)
		bot, err := readBot(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if bot.Scope == "" {
			if i == 0 && s.projectRoot != "" {
				bot.Scope = "project"
			} else {
				bot.Scope = "global"
			}
		}
		return bot, nil
	}
	return nil, ErrNotFound
}

// Save persists a bot's config.json and access.json.
// The target root is determined by bot.Scope:
//   - "project" (or empty when projectRoot is set) → projectRoot
//   - "global" or projectRoot is empty → globalRoot
func (s *Store) Save(bot Bot) error {
	root := s.globalRoot
	if s.projectRoot != "" && (bot.Scope == "project" || bot.Scope == "") {
		root = s.projectRoot
	}

	dir := botDir(root, bot.Username)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	cfg := configFile{
		Username:    bot.Username,
		Token:       bot.Token,
		DisplayName: bot.DisplayName,
	}
	if err := writeJSON(filepath.Join(dir, "config.json"), cfg); err != nil {
		return err
	}

	acc := accessFile{
		LastActive: bot.LastActive,
		Scope:      bot.Scope,
		AllowFrom:  bot.AllowFrom,
	}
	if err := writeJSON(filepath.Join(dir, "access.json"), acc); err != nil {
		return err
	}
	return nil
}

// Remove deletes the bot directory for the given username.
// It removes from whichever root contains the username (project first).
func (s *Store) Remove(username string) error {
	for _, root := range s.roots() {
		dir := botDir(root, username)
		if _, err := os.Stat(dir); err == nil {
			return os.RemoveAll(dir)
		}
	}
	return ErrNotFound
}

// writeJSON atomically encodes v as indented JSON to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
