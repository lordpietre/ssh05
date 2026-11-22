package supervisor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lordpietre/ssh05/contracts"
)

func BenchmarkBackendDataPath(b *testing.B) {
	runner := &fakeRunner{}
	backend := NewBackend(runner, Options{})
	defer backend.CloseAll()
	id, err := backend.Open(context.Background(), "host-1", "", contracts.SessionOptions{})
	if err != nil {
		b.Fatal(err)
	}
	run := runner.last()
	payload := make([]byte, 32<<10)
	for index := range payload {
		payload[index] = 'x'
	}
	var received atomic.Int64
	backend.OnData(id, func(data []byte) { received.Add(int64(len(data))) })

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		run.emit(string(payload))
	}
	b.StopTimer()

	want := int64(b.N * len(payload))
	deadline := time.Now().Add(5 * time.Second)
	for received.Load() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if received.Load() < want {
		b.Fatalf("received %d of %d bytes", received.Load(), want)
	}
}
