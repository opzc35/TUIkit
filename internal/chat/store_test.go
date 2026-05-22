package chat

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenStoreCreatesDefaultChannel(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "chat.json"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if _, ok := store.Channel("general"); !ok {
		t.Fatal("expected default general channel")
	}
}

func TestPostAndDeleteMessage(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "chat.json"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	message, err := store.PostMessage("general", "alice", "hello")
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}

	visible := store.RecentMessages("general", 10, false)
	if len(visible) != 1 || visible[0].Body != "hello" {
		t.Fatalf("RecentMessages() = %#v", visible)
	}

	if err := store.DeleteMessage(message.ID, "admin"); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	if visible := store.RecentMessages("general", 10, false); len(visible) != 0 {
		t.Fatalf("expected deleted message hidden, got %#v", visible)
	}

	all := store.RecentMessages("general", 10, true)
	if len(all) != 1 || !all[0].Deleted || all[0].DeletedBy != "admin" {
		t.Fatalf("expected deleted message in moderation view, got %#v", all)
	}
}

func TestMutedUserCannotPost(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "chat.json"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	if err := store.MuteUser("alice", 1, "spam", "admin"); err != nil {
		t.Fatalf("MuteUser() error = %v", err)
	}

	_, err = store.PostMessage("general", "alice", "hello")
	if !errors.Is(err, ErrMutedUser) {
		t.Fatalf("PostMessage() error = %v, want ErrMutedUser", err)
	}

	if err := store.UnmuteUser("alice"); err != nil {
		t.Fatalf("UnmuteUser() error = %v", err)
	}

	if _, err := store.PostMessage("general", "alice", "hello"); err != nil {
		t.Fatalf("PostMessage() after unmute error = %v", err)
	}
}

func TestSubscribeReceivesMessageEvents(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "chat.json"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	events, unsubscribe := store.Subscribe("general")
	defer unsubscribe()

	message, err := store.PostMessage("general", "alice", "hello")
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventMessage || event.Message.ID != message.ID {
			t.Fatalf("event = %#v, want message %d", event, message.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message event")
	}
}

func TestSubscribeReceivesModerationEvents(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "chat.json"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}

	message, err := store.PostMessage("general", "alice", "hello")
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}

	events, unsubscribe := store.Subscribe("general")
	defer unsubscribe()

	if err := store.DeleteMessage(message.ID, "admin"); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventDelete || event.Message.ID != message.ID {
			t.Fatalf("event = %#v, want delete %d", event, message.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delete event")
	}
}
