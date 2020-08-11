package flywheel

import (
	"time"
)

// Task is the detail about a task
type Task struct {
	CheckinAt   string   `json:"checkin_at,omitempty"`
	CompletedAt string   `json:"completed_at,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	ID          string   `json:"id"`
	Status      string   `json:"status"`
	Events      []string `json:"events,omitempty"`
	FailedAt    string   `json:"failed_at,omitempty"`
	Failure     string   `json:"failure,omitempty"`
}

func NewTask() *Task {
	return &Task{
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ID:        NewID(),
		Status:    "created",
	}
}

func (t *Task) taskToMapString() map[string]interface{} {
	return map[string]interface{}{
		"checkin_at":   t.CheckinAt,
		"completed_at": t.CompletedAt,
		"created_at":   t.CreatedAt,
		"id":           t.ID,
		"status":       t.Status,
		"failed_at":    t.FailedAt,
		"failure":      t.Failure,
	}
}

func (t *Task) mapToTask(task map[string]string, events []string) error {
	if checkinAt, ok := task["checkin_at"]; ok {
		t.CheckinAt = checkinAt
	}

	if completedAt, ok := task["completed_at"]; ok {
		t.CompletedAt = completedAt
	}

	if createdAt, ok := task["created_at"]; ok {
		t.CreatedAt = createdAt
	}

	if failedAt, ok := task["failed_at"]; ok {
		t.FailedAt = failedAt
	}

	if failure, ok := task["failure"]; ok {
		t.Failure = failure
	}

	if id, ok := task["id"]; ok {
		t.ID = id
	}

	if status, ok := task["status"]; ok {
		t.Status = status
	}

	t.Events = events

	return nil
}
