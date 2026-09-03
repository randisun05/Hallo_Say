package reminder

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddListCancelDue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reminders.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	if _, err := store.Add(1, "beli susu", past); err != nil {
		t.Fatalf("Add past: %v", err)
	}
	r2, err := store.Add(1, "meeting", future)
	if err != nil {
		t.Fatalf("Add future: %v", err)
	}
	if _, err := store.Add(2, "other chat", future); err != nil {
		t.Fatalf("Add other chat: %v", err)
	}

	list := store.List(1)
	if len(list) != 2 {
		t.Fatalf("List(1) len = %d, want 2", len(list))
	}

	due, err := store.DueReminders(time.Now())
	if err != nil {
		t.Fatalf("DueReminders: %v", err)
	}
	if len(due) != 1 || due[0].Message != "beli susu" {
		t.Fatalf("DueReminders = %+v, want just 'beli susu'", due)
	}

	list = store.List(1)
	if len(list) != 1 || list[0].ID != r2.ID {
		t.Fatalf("List(1) after due = %+v, want only future reminder", list)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	if len(reloaded.List(1)) != 1 {
		t.Fatalf("reloaded store lost data: %+v", reloaded.reminders)
	}

	ok, err := reloaded.Cancel(1, r2.ID)
	if err != nil || !ok {
		t.Fatalf("Cancel = %v, %v", ok, err)
	}
	if len(reloaded.List(1)) != 0 {
		t.Fatalf("List(1) after cancel = %+v, want empty", reloaded.List(1))
	}
}
