package downloader

import (
	"testing"

	"worker/models"
)

func TestManagerImplementsService(t *testing.T) {
	var _ Service = (*Manager)(nil)
}

func TestManagerExternalStatusHandler(t *testing.T) {
	mgr := New(t.TempDir(), "worker-1")
	hit := false
	mgr.SetExternalStatusHandler(func(task *models.Task) {
		hit = task.TaskID == "task-1"
	})

	if mgr.externalStatusHandler == nil {
		t.Fatalf("expected external status handler to be set")
	}

	mgr.externalStatusHandler(&models.Task{TaskID: "task-1"})
	if !hit {
		t.Fatalf("external status handler was not invoked")
	}
}
