package client

import (
	"testing"

	"worker/domain"
)

func TestGatewayClientImplementsGateway(t *testing.T) {
	var _ Gateway = (*GatewayClient)(nil)
}

func TestGatewayClientSendMessageWithoutConnection(t *testing.T) {
	gc := New("ws://localhost:1234", "worker-1")
	if err := gc.SendMessage(domain.MessageTypeHeartbeat, map[string]interface{}{"foo": "bar"}); err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestGatewayClientSetMessageHandler(t *testing.T) {
	captured := make([]domain.MessageType, 0, 1)
	handler := func(msgType domain.MessageType, _ map[string]interface{}) {
		captured = append(captured, msgType)
	}

	gc := New("ws://localhost:1234", "worker-1")
	gc.SetMessageHandler(handler)

	if gc.messageHandler == nil {
		t.Fatalf("expected message handler to be registered")
	}

	gc.messageHandler(domain.MessageTypeTaskSubmit, nil)
	if len(captured) != 1 || captured[0] != domain.MessageTypeTaskSubmit {
		t.Fatalf("handler not invoked as expected; captured=%v", captured)
	}
}
