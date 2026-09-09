package state

import (
	"testing"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/nested/state.json")
	want := agent.PersistentState{AgentID: "agent-1", AgentToken: "secret", MachineID: "machine-1", InventorySequence: 9, ProcessedCommands: map[string]agent.CommandResult{}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != want.AgentID || got.AgentToken != want.AgentToken || got.InventorySequence != want.InventorySequence {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}
