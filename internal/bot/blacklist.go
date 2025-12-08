package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Blacklist stores blocked phrases
type Blacklist struct {
	mu      sync.RWMutex
	Phrases [][]string `json:"phrases"`
	file    string
}

// NewBlacklist creates a blocklist backed by a JSON file in data/
func NewBlacklist(file string) BlacklistInterface {
	_ = os.MkdirAll("data", 0755)
	bl := &Blacklist{file: filepath.Join("data", filepath.Base(file))}
	bl.load()
	return bl
}

// AddPhrase adds a phrase to the blacklist
func (b *Blacklist) AddPhrase(words []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	lower := toLowerSlice(words)
	b.Phrases = append(b.Phrases, lower)
	_ = b.save()
}

// RemovePhrase removes a phrase from the blacklist
func (b *Blacklist) RemovePhrase(words []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	target := strings.Join(toLowerSlice(words), " ")
	before := len(b.Phrases)
	b.Phrases = slices.DeleteFunc(b.Phrases, func(p []string) bool {
		return strings.Join(p, " ") == target
	})
	if len(b.Phrases) < before {
		_ = b.save()
		return true
	}
	return false
}

func toLowerSlice(words []string) []string {
	result := make([]string, len(words))
	for i, w := range words {
		result[i] = strings.ToLower(w)
	}
	return result
}

// CheckMessage checks if a message contains any blacklisted phrases
func (b *Blacklist) CheckMessage(msg string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	text := strings.ToLower(msg)
	words := strings.Fields(text)
	return slices.ContainsFunc(b.Phrases, func(phrase []string) bool {
		if len(phrase) == 1 {
			return slices.Contains(words, phrase[0])
		}
		for _, pw := range phrase {
			if !strings.Contains(text, pw) {
				return false
			}
		}
		return true
	})
}

// List returns a copy of the blacklisted phrases
func (b *Blacklist) List() [][]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return slices.Clone(b.Phrases)
}

// save persists the blacklist to disk
func (b *Blacklist) save() error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(b.file, data, 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// load reads the blacklist from the disk
func (b *Blacklist) load() {
	data, err := os.ReadFile(b.file)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, b)
}
