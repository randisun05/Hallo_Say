// Package reminder provides simple file-backed persistence for reminders,
// scoped per chat.
package reminder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Reminder struct {
	ID        string    `json:"id"`
	ChatID    int64     `json:"chat_id"`
	Message   string    `json:"message"`
	RemindAt  time.Time `json:"remind_at"`
	Delivered bool      `json:"delivered"`
}

type Store struct {
	mu        sync.Mutex
	path      string
	reminders []Reminder
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if dir := filepath.Dir(path); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create data dir: %w", err)
			}
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store file: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.reminders); err != nil {
		return nil, fmt.Errorf("parse store file: %w", err)
	}
	return s, nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.reminders, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal reminders: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) Add(chatID int64, message string, remindAt time.Time) (Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := Reminder{
		ID:       fmt.Sprintf("%d", time.Now().UnixNano()),
		ChatID:   chatID,
		Message:  message,
		RemindAt: remindAt,
	}
	s.reminders = append(s.reminders, r)
	if err := s.save(); err != nil {
		return Reminder{}, err
	}
	return r, nil
}

func (s *Store) List(chatID int64) []Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Reminder
	for _, r := range s.reminders {
		if r.ChatID == chatID && !r.Delivered {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) Cancel(chatID int64, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.reminders {
		if r.ChatID == chatID && r.ID == id && !r.Delivered {
			s.reminders = append(s.reminders[:i], s.reminders[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// DueReminders returns undelivered reminders whose time has passed, and
// marks them delivered.
func (s *Store) DueReminders(now time.Time) ([]Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []Reminder
	changed := false
	for i, r := range s.reminders {
		if !r.Delivered && !r.RemindAt.After(now) {
			due = append(due, r)
			s.reminders[i].Delivered = true
			changed = true
		}
	}
	if changed {
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return due, nil
}
