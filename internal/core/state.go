package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

// State holds user quiz results and newbie flags
type State struct {
	mu          sync.RWMutex
	UserCorrect map[int]int  `json:"user_correct"`
	NewbieMap   map[int]bool `json:"is_newbie"`
	file        string
}

// NewState allocates a new State and loads persisted data
func NewState() UserState {
	_ = os.MkdirAll("data", 0755)
	s := &State{
		UserCorrect: make(map[int]int),
		NewbieMap:   make(map[int]bool),
		file:        filepath.Join("data", "state.json"),
	}
	s.load()
	return s
}

func (s *State) InitUser(id int)    { s.withLock(func() { s.UserCorrect[id] = 0 }) }
func (s *State) IncCorrect(id int)  { s.withLock(func() { s.UserCorrect[id]++ }) }
func (s *State) Reset(id int)       { s.withLock(func() { delete(s.UserCorrect, id) }) }
func (s *State) SetNewbie(id int)   { s.withLock(func() { s.NewbieMap[id] = true }) }
func (s *State) ClearNewbie(id int) { s.withLock(func() { delete(s.NewbieMap, id) }) }

func (s *State) TotalCorrect(id int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserCorrect[id]
}

func (s *State) IsNewbie(id int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.NewbieMap[id]
}

func (s *State) withLock(fn func()) {
	s.mu.Lock()
	fn()
	s.mu.Unlock()
	s.save()
}

func (s *State) save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		logrus.WithError(err).Error("state marshal")
		return
	}
	if err := os.WriteFile(s.file, data, 0644); err != nil {
		logrus.WithError(err).Error("state write")
	}
}

func (s *State) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, s)
	if s.UserCorrect == nil {
		s.UserCorrect = make(map[int]int)
	}
	if s.NewbieMap == nil {
		s.NewbieMap = make(map[int]bool)
	}
}
