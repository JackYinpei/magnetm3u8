package domain

// MessageType defines the semantic type of a gateway message.
type MessageType string

const (
	MessageTypeRegistrationConfirmed MessageType = "registration_confirmed"
	MessageTypeTaskSubmit            MessageType = "task_submit"
	MessageTypeGetTasks              MessageType = "get_tasks"
	MessageTypeGetTaskDetail         MessageType = "get_task_detail"
	MessageTypeWebRTCOffer           MessageType = "webrtc_offer"
	MessageTypeICECandidate          MessageType = "ice_candidate"
	MessageTypeTasksResponse         MessageType = "tasks_response"
	MessageTypeTaskDetailResponse    MessageType = "task_detail_response"
	MessageTypeTaskStatus            MessageType = "task_status"
	MessageTypeHeartbeat             MessageType = "heartbeat"
	MessageTypeWebRTCAnswer          MessageType = "webrtc_answer"
)

// TaskStatus captures the lifecycle state of a download/transcode task.
type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusDownloading TaskStatus = "downloading"
	TaskStatusCompleted   TaskStatus = "completed"
	TaskStatusError       TaskStatus = "error"
	TaskStatusPaused      TaskStatus = "paused"
	TaskStatusTranscoding TaskStatus = "transcoding"
	TaskStatusReady       TaskStatus = "ready"
)

// TranscodeStatus captures the lifecycle of a transcoding job.
type TranscodeStatus string

const (
	TranscodeStatusPending    TranscodeStatus = "pending"
	TranscodeStatusProcessing TranscodeStatus = "processing"
	TranscodeStatusCompleted  TranscodeStatus = "completed"
	TranscodeStatusError      TranscodeStatus = "error"
)

// WorkerStatus captures the runtime state of a worker node.
type WorkerStatus string

const (
	WorkerStatusOnline  WorkerStatus = "online"
	WorkerStatusOffline WorkerStatus = "offline"
)

// GatewayMessageHandler defines the callback signature used by gateway clients.
type GatewayMessageHandler func(MessageType, map[string]interface{})

// NodeInfo conveys worker registration details to the gateway.
type NodeInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Status       WorkerStatus      `json:"status"`
	Capabilities []string          `json:"capabilities"`
	Resources    map[string]int    `json:"resources"`
	Metadata     map[string]string `json:"metadata"`
}
