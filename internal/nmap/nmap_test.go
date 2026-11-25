//go:build linux && (amd64 || arm64)

package nmap

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEmbeddedBinary(t *testing.T) {
	path, err := Binary()
	if err != nil {
		t.Fatalf("Binary: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("binary is not executable: %v", info.Mode())
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("nmap --version: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "Nmap version 7.99") {
		t.Fatalf("unexpected version: %s", output)
	}
}

func TestVendorLookup(t *testing.T) {
	if got := Vendor("3c:f0:83:11:22:33"); got != "Arcadyan" {
		t.Fatalf("Vendor = %q", got)
	}
	if got := Vendor("02:00:00:00:00:00"); got != "" {
		t.Fatalf("unknown MAC vendor = %q", got)
	}
}
