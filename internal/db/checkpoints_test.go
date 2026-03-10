package db

import (
	"encoding/json"
	"testing"
)

func TestCheckpointSaveLoad(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	context := map[string]interface{}{
		"session_state": "active",
		"files":         []string{"main.go", "util.go"},
		"last_action":   "reading config",
	}

	err := database.CheckpointSave(1, "Builder", "builder-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	checkpoint, err := database.CheckpointLoad(1, "Builder")
	if err != nil {
		t.Fatalf("CheckpointLoad failed: %v", err)
	}

	if checkpoint == nil {
		t.Fatal("Expected checkpoint, got nil")
	}

	if checkpoint.TaskID != 1 {
		t.Errorf("Expected task_id 1, got %d", checkpoint.TaskID)
	}

	if checkpoint.AgentRole != "Builder" {
		t.Errorf("Expected agent_role 'Builder', got %s", checkpoint.AgentRole)
	}

	if checkpoint.AgentIdentity != "builder-1" {
		t.Errorf("Expected agent_identity 'builder-1', got %s", checkpoint.AgentIdentity)
	}

	var loadedContext map[string]interface{}
	err = json.Unmarshal([]byte(checkpoint.ContextSnapshot), &loadedContext)
	if err != nil {
		t.Fatalf("Failed to unmarshal context: %v", err)
	}

	if loadedContext["session_state"] != "active" {
		t.Errorf("Expected session_state 'active', got %v", loadedContext["session_state"])
	}
}

func TestCheckpointLoadNotFound(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	checkpoint, err := database.CheckpointLoad(999, "Builder")
	if err != nil {
		t.Fatalf("CheckpointLoad failed: %v", err)
	}

	if checkpoint != nil {
		t.Error("Expected nil checkpoint for non-existent task")
	}
}

func TestCheckpointDelete(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	context := map[string]interface{}{
		"session_state": "active",
	}

	err := database.CheckpointSave(1, "Builder", "builder-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	checkpoint, err := database.CheckpointLoad(1, "Builder")
	if err != nil {
		t.Fatalf("CheckpointLoad failed: %v", err)
	}

	if checkpoint == nil {
		t.Fatal("Expected checkpoint before delete")
	}

	err = database.CheckpointDelete(1, "Builder")
	if err != nil {
		t.Fatalf("CheckpointDelete failed: %v", err)
	}

	checkpoint, err = database.CheckpointLoad(1, "Builder")
	if err != nil {
		t.Fatalf("CheckpointLoad after delete failed: %v", err)
	}

	if checkpoint != nil {
		t.Error("Expected nil checkpoint after delete")
	}
}

func TestCheckpointListByTaskID(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	context := map[string]interface{}{
		"session_state": "active",
	}

	err := database.CheckpointSave(1, "Builder", "builder-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	err = database.CheckpointSave(1, "Scout", "scout-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	checkpoints, err := database.CheckpointListByTaskID(1)
	if err != nil {
		t.Fatalf("CheckpointListByTaskID failed: %v", err)
	}

	if len(checkpoints) != 2 {
		t.Errorf("Expected 2 checkpoints, got %d", len(checkpoints))
	}
}

func TestCheckpointDeleteByTaskID(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	context := map[string]interface{}{
		"session_state": "active",
	}

	err := database.CheckpointSave(1, "Builder", "builder-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	err = database.CheckpointSave(1, "Scout", "scout-1", context)
	if err != nil {
		t.Fatalf("CheckpointSave failed: %v", err)
	}

	err = database.CheckpointDeleteByTaskID(1)
	if err != nil {
		t.Fatalf("CheckpointDeleteByTaskID failed: %v", err)
	}

	checkpoints, err := database.CheckpointListByTaskID(1)
	if err != nil {
		t.Fatalf("CheckpointListByTaskID failed: %v", err)
	}

	if len(checkpoints) != 0 {
		t.Errorf("Expected 0 checkpoints after delete, got %d", len(checkpoints))
	}
}
