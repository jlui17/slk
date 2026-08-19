// Package slackclient wraps the slack-go library with token management,
// Socket Mode event handling, and a simplified Web API interface.
package slackclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Token holds browser session credentials for a single Slack workspace.
type Token struct {
	AccessToken string `json:"access_token"` // xoxc-... token
	Cookie      string `json:"cookie"`       // d cookie value
	Domain      string `json:"domain"`       // workspace subdomain (to re-mint)
	TeamID      string `json:"team_id"`
	TeamName    string `json:"team_name"`
}

// TokenStore persists Slack tokens as JSON files in a directory,
// one file per workspace ({teamID}.json).
type TokenStore struct {
	dir string
}

// NewTokenStore creates a TokenStore that reads/writes to the given directory.
func NewTokenStore(dir string) *TokenStore {
	return &TokenStore{dir: dir}
}

// Save writes a token to disk, creating the directory if needed.
// File permissions are restricted to owner-only (0600).
//
// The bytes land in a temp file in the same directory and are renamed
// over the target, so a reader gets either the old token or the new one
// and never a truncated file. Every launch re-mints and rewrites these
// files while another slk instance may be reading them.
func (s *TokenStore) Save(token Token) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("creating token dir: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}

	// os.CreateTemp creates the file 0600, which is the mode a token
	// file has to end up with anyway.
	f, err := os.CreateTemp(s.dir, "."+token.TeamID+"-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp token: %w", err)
	}
	tmp := f.Name()
	// No-op once the rename succeeds; on an earlier return it keeps a
	// failed save from littering the token directory.
	defer os.Remove(tmp)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("writing token: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("syncing token: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing token: %w", err)
	}

	path := filepath.Join(s.dir, token.TeamID+".json")
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing token: %w", err)
	}
	return nil
}

// Load reads a token for the given team ID from disk.
// Returns an error if the token file does not exist.
func (s *TokenStore) Load(teamID string) (Token, error) {
	path := filepath.Join(s.dir, teamID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Token{}, fmt.Errorf("token not found for team %s", teamID)
		}
		return Token{}, fmt.Errorf("reading token: %w", err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, fmt.Errorf("unmarshaling token: %w", err)
	}
	return token, nil
}

// List returns all tokens stored on disk.
// Corrupted token files are silently skipped.
func (s *TokenStore) List() ([]Token, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing tokens: %w", err)
	}

	var tokens []Token
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		teamID := strings.TrimSuffix(entry.Name(), ".json")
		token, err := s.Load(teamID)
		if err != nil {
			continue // skip corrupted tokens
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// Delete removes a token file for the given team ID.
// Returns nil if the file does not exist.
func (s *TokenStore) Delete(teamID string) error {
	path := filepath.Join(s.dir, teamID+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting token: %w", err)
	}
	return nil
}
