package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"worker/config"
	"worker/database"
	"worker/domain"
	"worker/models"
	"worker/transcoder"
	"worker/webrtc"

	webrtcLib "github.com/pion/webrtc/v3"
)

type fakeGateway struct {
	messageHandler domain.GatewayMessageHandler
	statuses       []struct {
		taskID string
		status domain.TaskStatus
	}
	messages []domain.MessageType
	mu       sync.Mutex
}

func (f *fakeGateway) SetMessageHandler(handler domain.GatewayMessageHandler) {
	f.messageHandler = handler
}

func (f *fakeGateway) Connect(domain.NodeInfo) error { return nil }
func (f *fakeGateway) Disconnect()                   {}
func (f *fakeGateway) IsConnected() bool             { return true }

func (f *fakeGateway) SendMessage(msgType domain.MessageType, _ map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msgType)
	return nil
}

func (f *fakeGateway) SendHeartbeat() error { return nil }

func (f *fakeGateway) SendTaskStatus(taskID string, status domain.TaskStatus, _ int, _ map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, struct {
		taskID string
		status domain.TaskStatus
	}{taskID: taskID, status: status})
	return nil
}

func (f *fakeGateway) SendWebRTCAnswer(string, string) error { return nil }
func (f *fakeGateway) SendICECandidate(string, string) error { return nil }

type fakeDownloader struct {
	startCalledWith []string
	tasks           []*models.Task
	lookup          map[string]*models.Task
	statusHandler   func(*models.Task)
}

func (f *fakeDownloader) Start() error { return nil }
func (f *fakeDownloader) Stop()        {}

func (f *fakeDownloader) StartDownload(magnetURL string) (string, error) {
	f.startCalledWith = append(f.startCalledWith, magnetURL)
	return "task-1", nil
}

func (f *fakeDownloader) PauseTask(string) error  { return nil }
func (f *fakeDownloader) ResumeTask(string) error { return nil }
func (f *fakeDownloader) RemoveTask(string) error { return nil }

func (f *fakeDownloader) GetTask(taskID string) (*models.Task, bool) {
	if f.lookup == nil {
		return nil, false
	}
	task, ok := f.lookup[taskID]
	return task, ok
}

func (f *fakeDownloader) GetAllTasks() []*models.Task {
	return f.tasks
}

func (f *fakeDownloader) GetStatusChannel() <-chan *models.Task {
	ch := make(chan *models.Task)
	close(ch)
	return ch
}

func (f *fakeDownloader) SetExternalStatusHandler(handler func(*models.Task)) {
	f.statusHandler = handler
}

type fakeTranscoder struct {
	startCalls []string
	statusCh   chan *transcoder.TranscodeTask
}

func (f *fakeTranscoder) Start() error { return nil }
func (f *fakeTranscoder) Stop()        {}

func (f *fakeTranscoder) StartTranscode(inputPath string) (string, error) {
	f.startCalls = append(f.startCalls, inputPath)
	return "transcode-1", nil
}

func (f *fakeTranscoder) GetTask(string) (*transcoder.TranscodeTask, bool) { return nil, false }
func (f *fakeTranscoder) GetAllTasks() []*transcoder.TranscodeTask         { return nil }

func (f *fakeTranscoder) GetStatusChannel() <-chan *transcoder.TranscodeTask {
	return f.statusCh
}

type fakeWebRTC struct {
	configUpdates int
}

func (f *fakeWebRTC) Start() error { return nil }
func (f *fakeWebRTC) Stop()        {}

func (f *fakeWebRTC) HandleOffer(string, string) (string, error) { return "answer", nil }
func (f *fakeWebRTC) AddICECandidate(string, string) error       { return nil }
func (f *fakeWebRTC) GetSession(string) (*webrtc.Session, bool)  { return nil, false }
func (f *fakeWebRTC) GetAllSessions() []*webrtc.Session          { return nil }

func (f *fakeWebRTC) SetICECandidateHandler(func(string, *webrtcLib.ICECandidate)) {}

func (f *fakeWebRTC) SetConnectionStateHandler(func(string, webrtcLib.PeerConnectionState)) {}

func (f *fakeWebRTC) UpdateConfiguration(webrtcLib.Configuration) {
	f.configUpdates++
}

func (f *fakeWebRTC) SendData(string, []byte) error { return nil }
func (f *fakeWebRTC) BroadcastData([]byte)          {}

type fakeTaskRepository struct {
	store map[string]*models.Task
}

func (f *fakeTaskRepository) Create(task *models.Task) error {
	if f.store == nil {
		f.store = make(map[string]*models.Task)
	}
	f.store[task.TaskID] = task
	return nil
}

func (f *fakeTaskRepository) GetByTaskID(taskID string) (*models.Task, error) {
	if task, ok := f.store[taskID]; ok {
		return task, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeTaskRepository) GetAll() ([]models.Task, error) { return nil, nil }
func (f *fakeTaskRepository) GetByWorkerID(string) ([]models.Task, error) {
	return nil, nil
}

func (f *fakeTaskRepository) GetByStatus(domain.TaskStatus) ([]models.Task, error) {
	return nil, nil
}

func (f *fakeTaskRepository) Update(task *models.Task) error {
	f.store[task.TaskID] = task
	return nil
}

func (f *fakeTaskRepository) UpdateStatus(taskID string, status domain.TaskStatus) error {
	if task, ok := f.store[taskID]; ok {
		task.Status = status
		return nil
	}
	return errors.New("not found")
}

func (f *fakeTaskRepository) UpdateProgress(string, int, int64, int64) error { return nil }
func (f *fakeTaskRepository) Delete(string) error                            { return nil }
func (f *fakeTaskRepository) GetActiveTasksCount(string) (int64, error)      { return 0, nil }

func TestWorkerHandleTaskSubmitUsesDownloaderAndGateway(t *testing.T) {
	cfg := config.Default()
	cfg.Node.ID = "worker-1"

	gw := &fakeGateway{}
	dl := &fakeDownloader{}
	tr := &fakeTranscoder{statusCh: make(chan *transcoder.TranscodeTask)}
	wr := &fakeWebRTC{}

	worker, err := New(cfg, Dependencies{
		Gateway:    gw,
		Downloader: dl,
		Transcoder: tr,
		WebRTC:     wr,
		TaskRepoFactory: func() database.TaskRepository {
			return &fakeTaskRepository{store: map[string]*models.Task{"task-1": {TaskID: "task-1"}}}
		},
		Clock: func() time.Time { return time.Now() },
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	worker.handleTaskSubmit(map[string]interface{}{"magnet_url": "magnet"})

	if len(dl.startCalledWith) != 1 {
		t.Fatalf("expected downloader start to be invoked once")
	}

	if len(gw.statuses) != 1 || gw.statuses[0].status != domain.TaskStatusDownloading {
		t.Fatalf("expected gateway to receive status update, got %+v", gw.statuses)
	}
}

func TestWorkerHandleGetTasksResponds(t *testing.T) {
	cfg := config.Default()
	cfg.Node.ID = "worker-1"

	gw := &fakeGateway{}
	dl := &fakeDownloader{}
	dl.tasks = []*models.Task{{TaskID: "task-1"}}
	tr := &fakeTranscoder{statusCh: make(chan *transcoder.TranscodeTask)}
	wr := &fakeWebRTC{}

	worker, err := New(cfg, Dependencies{
		Gateway:    gw,
		Downloader: dl,
		Transcoder: tr,
		WebRTC:     wr,
		TaskRepoFactory: func() database.TaskRepository {
			return &fakeTaskRepository{}
		},
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	worker.handleGetTasks(map[string]interface{}{})

	if len(gw.messages) != 1 || gw.messages[0] != domain.MessageTypeTasksResponse {
		t.Fatalf("expected tasks response to be sent, got %v", gw.messages)
	}
}
