package transcoder

import "testing"

func TestManagerImplementsService(t *testing.T) {
	var _ Service = (*Manager)(nil)
}

func TestManagerStatusChannelExposure(t *testing.T) {
	mgr := New(t.TempDir(), t.TempDir())
	if mgr.GetStatusChannel() != mgr.statusChan {
		t.Fatalf("GetStatusChannel should expose underlying status channel")
	}
}
