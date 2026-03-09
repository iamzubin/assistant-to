package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*DB, string) {
	t.Helper()

	// Create a temporary directory for the test database
	dir, err := os.MkdirTemp("", "assistant-to-db-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(dir, "state.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	err = database.InitSchema()
	if err != nil {
		t.Fatalf("failed to initialize schema: %v", err)
	}

	return database, dir
}

func teardownTestDB(t *testing.T, database *DB, dir string) {
	t.Helper()
	database.Close()
	os.RemoveAll(dir)
}

func TestMailCRUD(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	// Test SendMail
	err := database.SendMail("agent-1", "coordinator", "Hello", "System initialized", MailTypeStatus, PriorityNormal)
	if err != nil {
		t.Fatalf("SendMail failed: %v", err)
	}

	// Test GetUnreadMail
	mail, err := database.GetUnreadMail("coordinator")
	if err != nil {
		t.Fatalf("GetUnreadMail failed: %v", err)
	}

	if len(mail) != 1 {
		t.Fatalf("Expected 1 unread mail, got %d", len(mail))
	}

	if mail[0].Sender != "agent-1" || mail[0].Subject != "Hello" {
		t.Errorf("Unexpected mail content: %+v", mail[0])
	}

	// Test MarkMailRead
	err = database.MarkMailRead(mail[0].ID)
	if err != nil {
		t.Fatalf("MarkMailRead failed: %v", err)
	}

	// Verify it's no longer unread
	mailAgain, err := database.GetUnreadMail("coordinator")
	if err != nil {
		t.Fatalf("GetUnreadMail failed: %v", err)
	}
	if len(mailAgain) != 0 {
		t.Errorf("Expected 0 unread mail after MarkMailRead, got %d", len(mailAgain))
	}
}

func TestTasksCRUD(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	// Test AddTask
	taskID, err := database.AddTask("Test DB", "Write db tests", "internal/db/db_test.go", 0)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if taskID != 1 {
		t.Errorf("Expected first task ID to be 1, got %d", taskID)
	}

	// Test ListTasksByStatus (all)
	allTasks, err := database.ListTasksByStatus("")
	if err != nil {
		t.Fatalf("ListTasksByStatus (all) failed: %v", err)
	}

	if len(allTasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(allTasks))
	}

	if allTasks[0].Title != "Test DB" || allTasks[0].Status != TaskStatusPending {
		t.Errorf("Unexpected task content: %+v", allTasks[0])
	}

	// Test UpdateTaskStatus
	err = database.UpdateTaskStatus(int(taskID), TaskStatusStarted)
	if err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	// Test ListTasksByStatus (specific)
	startedTasks, err := database.ListTasksByStatus(TaskStatusStarted)
	if err != nil {
		t.Fatalf("ListTasksByStatus ('started') failed: %v", err)
	}

	if len(startedTasks) != 1 {
		t.Fatalf("Expected 1 started task, got %d", len(startedTasks))
	}
	if startedTasks[0].Status != TaskStatusStarted {
		t.Errorf("Expected task status to be 'started', got '%s'", startedTasks[0].Status)
	}

	pendingTasks, err := database.ListTasksByStatus("pending")
	if err != nil {
		t.Fatalf("ListTasksByStatus ('pending') failed: %v", err)
	}
	if len(pendingTasks) != 0 {
		t.Errorf("Expected 0 pending tasks, got %d", len(pendingTasks))
	}

	// Test GetTaskByID
	task, err := database.GetTaskByID(int(taskID))
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if task.ID != int(taskID) || task.Title != "Test DB" {
		t.Errorf("Unexpected task content: %+v", task)
	}

	_, err = database.GetTaskByID(999)
	if err == nil {
		t.Errorf("Expected error for non-existent task ID in GetTaskByID")
	}

	// Test UpdateTaskStatus for non-existent task
	err = database.UpdateTaskStatus(999, TaskStatusStarted)
	if err == nil {
		t.Errorf("Expected error for non-existent task ID in UpdateTaskStatus")
	}

	// Test RemoveTask for non-existent task
	err = database.RemoveTask(999)
	if err == nil {
		t.Errorf("Expected error for non-existent task ID in RemoveTask")
	}

	// Test successful RemoveTask
	err = database.RemoveTask(int(taskID))
	if err != nil {
		t.Fatalf("RemoveTask failed: %v", err)
	}

	// Verify it's gone
	_, err = database.GetTaskByID(int(taskID))
	if err == nil {
		t.Errorf("Expected error when getting removed task")
	}
}

func TestEventsCRUD(t *testing.T) {
	database, dir := setupTestDB(t)
	defer teardownTestDB(t, database, dir)

	// Test RecordEvent
	err := database.RecordEvent("builder-A", "tool_call", "bash 'ls -la'")
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure distinct timestamps just in case

	err = database.RecordEvent("builder-A", "file_write", "main.go modified")
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	err = database.RecordEvent("coordinator", "spawn", "builder-A created")
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	// Test GetAgentHistory
	history, err := database.GetAgentHistory("builder-A")
	if err != nil {
		t.Fatalf("GetAgentHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("Expected 2 events for builder-A, got %d", len(history))
	}

	if history[0].EventType != "tool_call" || history[1].EventType != "file_write" {
		t.Errorf("Unexpected event history order or content")
	}

	// Test GetLastHeartbeat
	lastHB, err := database.GetLastHeartbeat("builder-A")
	if err != nil {
		t.Fatalf("GetLastHeartbeat failed: %v", err)
	}

	if lastHB.IsZero() {
		t.Fatalf("GetLastHeartbeat returned zero time")
	}

	if !lastHB.Equal(history[1].Timestamp) {
		t.Errorf("Expected last heartbeat to match latest event timestamp")
	}
}
