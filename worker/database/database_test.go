package database

import (
	"testing"
	"time"

	"worker/domain"
	"worker/models"
)

func TestTaskRepositoryCRUD(t *testing.T) {
	path := t.TempDir()
	if err := Initialize(path); err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	t.Cleanup(func() {
		if err := Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}
		DB = nil
	})

	repo := NewTaskRepository()
	task := &models.Task{
		TaskID:    "task_1",
		MagnetURL: "magnet:?xt=urn:btih:dummy",
		WorkerID:  "worker-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := repo.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	fetched, err := repo.GetByTaskID(task.TaskID)
	if err != nil {
		t.Fatalf("get task by id: %v", err)
	}
	if fetched.TaskID != task.TaskID {
		t.Fatalf("unexpected task id: %s", fetched.TaskID)
	}

	if err := repo.UpdateStatus(task.TaskID, domain.TaskStatusDownloading); err != nil {
		t.Fatalf("update status: %v", err)
	}

	byStatus, err := repo.GetByStatus(domain.TaskStatusDownloading)
	if err != nil {
		t.Fatalf("get by status: %v", err)
	}
	if len(byStatus) != 1 {
		t.Fatalf("expected 1 task, got %d", len(byStatus))
	}

	if err := repo.UpdateProgress(task.TaskID, 50, 1024, 2048); err != nil {
		t.Fatalf("update progress: %v", err)
	}

	if err := repo.Delete(task.TaskID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	if _, err := repo.GetByTaskID(task.TaskID); err == nil {
		t.Fatalf("expected error fetching deleted task")
	}
}
