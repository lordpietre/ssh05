package supervisor

import (
	"context"
	"testing"

	"github.com/lordpietre/ssh05/contracts"
	"github.com/lordpietre/ssh05/internal/ipc"
)

func TestCloseMissingSessionIsIdempotent(t *testing.T) {
	backend := NewBackend(&fakeRunner{}, Options{})
	service := NewService(backend, nil)
	socket := startServiceServer(t, service)

	client, err := ipc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = ipc.CallTyped[contracts.SessionsCloseParams, struct{}](
		context.Background(),
		client,
		contracts.MethodSessionsClose,
		contracts.SessionsCloseParams{SessionID: "ses-missing", Intent: contracts.CloseView},
	)
	if err != nil {
		t.Fatalf("closing an already closed session must succeed: %v", err)
	}
}
