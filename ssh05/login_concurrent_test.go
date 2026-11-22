package ssh05

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in concurrency check:
//
//	TRSS_TEST_SSH_ALIAS=trss-ui-1 \
//	TRSS_TEST_SSH_CONFIG=/tmp/trss-ui-smoke-ssh-config \
//	go test -race -run TestSshLoginConcurrent -v ./ssh05
func TestSshLoginConcurrent(t *testing.T) {
	alias := os.Getenv("TRSS_TEST_SSH_ALIAS")
	if alias == "" {
		t.Skip("set TRSS_TEST_SSH_ALIAS to run the concurrent SSH test")
	}
	config := os.Getenv("TRSS_TEST_SSH_CONFIG")

	const count = 3
	clients := make([]SshClient, count)
	errs := make([]error, count)
	start := time.Now()
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			clients[index], errs[index] = SshLogin(&SshArgs{
				Destination: alias,
				ConfigFile:  config,
			})
		}(index)
	}
	wg.Wait()
	t.Logf("%d concurrent logins finished in %s", count, time.Since(start))

	for index, err := range errs {
		if err != nil {
			t.Fatalf("login %d: %v", index, err)
		}
		if clients[index] == nil {
			t.Fatalf("login %d returned a nil client", index)
		}
	}

	for index, client := range clients {
		session, err := client.NewSession()
		if err != nil {
			t.Fatalf("session %d: %v", index, err)
		}
		output, err := session.CombinedOutput("echo TRSS_CONCURRENT_OK")
		_ = session.Close()
		_ = client.Close()
		if err != nil {
			t.Fatalf("command %d: %v", index, err)
		}
		if !strings.Contains(string(output), "TRSS_CONCURRENT_OK") {
			t.Fatalf("output %d = %q", index, output)
		}
	}
}
