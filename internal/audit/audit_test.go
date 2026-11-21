package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerWritesSanitizedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "audit.log")
	logger, err := New(path, 1<<20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Log(Event{
		Action:    "session.open",
		SessionID: "ses-1",
		Result:    "ok",
	})
	logger.Log(Event{
		Action:    "hosts.import",
		Result:    "error",
		ErrorCode: "invalid_params",
		Detail:    "keepass",
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	for _, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid JSON %q: %v", line, err)
		}
		if event.Time.IsZero() || event.Action == "" {
			t.Fatalf("incomplete event: %+v", event)
		}
	}
	if strings.Contains(string(data), "password") || strings.Contains(string(data), "token") {
		t.Fatalf("secret-like text in audit: %s", data)
	}
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o", info.Mode().Perm())
		}
	}
	if info, err := os.Stat(filepath.Dir(path)); err == nil {
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode = %o", info.Mode().Perm())
		}
	}
}

func TestLoggerRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := New(path, 128)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		logger.Log(Event{Time: time.Now(), Action: "test", Result: "ok"})
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotation file missing: %v", err)
	}
}

func TestLoggerRetainsBoundedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := NewRotating(path, 128, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		logger.Log(Event{Time: time.Now(), Action: "test", Result: "ok"})
	}
	for _, suffix := range []string{"", ".1", ".2"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Fatalf("expected %s: %v", path+suffix, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected extra rotation file: %v", err)
	}
}

func TestNilLoggerIsNoop(t *testing.T) {
	var logger *Logger
	logger.Log(Event{Action: "test", Result: "ok"})
}
