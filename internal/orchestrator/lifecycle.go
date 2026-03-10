package orchestrator

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"dwight/internal/config"
	"dwight/internal/db"
	"dwight/internal/sandbox"
)

type AgentContext struct {
	TaskID        int             `json:"task_id"`
	AgentRole     string          `json:"agent_role"`
	AgentIdentity string          `json:"agent_identity"`
	SessionState  json.RawMessage `json:"session_state"`
	WorktreeDir   string          `json:"worktree_dir"`
	CreatedAt     time.Time       `json:"created_at"`
}

type LifecycleManager struct {
	DB       *db.DB
	PWD      string
	Worktree string
}

func NewLifecycleManager(database *db.DB, pwd string) *LifecycleManager {
	return &LifecycleManager{
		DB:  database,
		PWD: pwd,
	}
}

func (l *LifecycleManager) CheckForCheckpoint(taskID int, role string) (*db.Checkpoint, error) {
	checkpoint, err := l.DB.CheckpointLoad(taskID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to check for checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (l *LifecycleManager) SaveCheckpoint(taskID int, role, identity string, sessionState interface{}) error {
	cfg := config.Default()
	worktreesDir := cfg.GetWorktreesDir(l.PWD)
	worktreeDir := ""
	if worktreesDir != "" {
		worktreeDir = worktreesDir
	}

	context := AgentContext{
		TaskID:        taskID,
		AgentRole:     role,
		AgentIdentity: identity,
		WorktreeDir:   worktreeDir,
		CreatedAt:     time.Now(),
	}

	stateJSON, err := json.Marshal(sessionState)
	if err != nil {
		return fmt.Errorf("failed to serialize session state: %w", err)
	}
	context.SessionState = stateJSON

	err = l.DB.CheckpointSave(taskID, role, identity, context)
	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	log.Printf("LifecycleManager: Saved checkpoint for task %d, role %s", taskID, role)
	return nil
}

func (l *LifecycleManager) ResumeFromCheckpoint(taskID int, role string) (*AgentContext, error) {
	checkpoint, err := l.DB.CheckpointLoad(taskID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if checkpoint == nil {
		return nil, nil
	}

	var ctx AgentContext
	err = json.Unmarshal([]byte(checkpoint.ContextSnapshot), &ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize checkpoint context: %w", err)
	}

	log.Printf("LifecycleManager: Resuming task %d from checkpoint (role: %s)", taskID, role)
	return &ctx, nil
}

func (l *LifecycleManager) DeleteCheckpoint(taskID int, role string) error {
	err := l.DB.CheckpointDelete(taskID, role)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	log.Printf("LifecycleManager: Deleted checkpoint for task %d, role %s", taskID, role)
	return nil
}

func (l *LifecycleManager) HandleTaskStart(taskID int, role string) (bool, *AgentContext, error) {
	checkpoint, err := l.CheckForCheckpoint(taskID, role)
	if err != nil {
		return false, nil, err
	}

	if checkpoint == nil {
		return false, nil, nil
	}

	log.Printf("LifecycleManager: Found checkpoint for task %d, role %s", taskID, role)

	ctx, err := l.ResumeFromCheckpoint(taskID, role)
	if err != nil {
		return false, nil, err
	}

	return true, ctx, nil
}

func (l *LifecycleManager) HandleTaskComplete(taskID int, role string) error {
	err := l.DeleteCheckpoint(taskID, role)
	if err != nil {
		return err
	}

	log.Printf("LifecycleManager: Cleaned up checkpoint after task completion (task: %d, role: %s)", taskID, role)
	return nil
}

func (l *LifecycleManager) HandleAgentCrash(taskID int, role, identity string) error {
	log.Printf("LifecycleManager: Agent crash detected for task %d, role %s - preserving checkpoint", taskID, role)

	sessionName := sandbox.ProjectPrefix(l.PWD) + fmt.Sprintf("%d", taskID)
	session := &sandbox.TmuxSession{SessionName: sessionName}

	sessionState := map[string]interface{}{
		"session_exists": session.HasSession(),
		"crash_time":     time.Now().Unix(),
	}

	err := l.SaveCheckpoint(taskID, role, identity, sessionState)
	if err != nil {
		log.Printf("LifecycleManager: Failed to save checkpoint on crash: %v", err)
		return err
	}

	l.DB.RecordEvent(identity, "agent_crash", fmt.Sprintf("Agent crashed - checkpoint preserved for task %d", taskID))

	return nil
}

func (l *LifecycleManager) StartCheckpointCleanupLoop(ctx interface{ Done() <-chan struct{} }) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("LifecycleManager: Checkpoint cleanup loop stopping")
			return
		case <-ticker.C:
			rows, err := l.DB.CleanupExpiredCheckpoints()
			if err != nil {
				log.Printf("LifecycleManager: Failed to cleanup expired checkpoints: %v", err)
			} else if rows > 0 {
				log.Printf("LifecycleManager: Cleaned up %d expired checkpoints", rows)
			}
		}
	}
}
