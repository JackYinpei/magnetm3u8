package webrtc

import (
	"testing"

	webrtcLib "github.com/pion/webrtc/v3"
)

func TestManagerImplementsService(t *testing.T) {
	var _ Service = (*Manager)(nil)
}

func TestManagerSendDataWithoutSession(t *testing.T) {
	mgr := New()
	if err := mgr.SendData("missing", []byte("payload")); err == nil {
		t.Fatalf("expected error when sending without session")
	}
}

func TestManagerIceCandidateHandler(t *testing.T) {
	mgr := New()
	mgr.SetICECandidateHandler(func(string, *webrtcLib.ICECandidate) {})
	if mgr.iceCandidateHandler == nil {
		t.Fatalf("expected ICE candidate handler to be stored")
	}
}
