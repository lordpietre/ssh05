package ssh05

import (
	"errors"
	"testing"
)

func TestNonInteractiveDisablesSecretPrompt(t *testing.T) {
	release := enterNonInteractive(true)
	defer release()

	if terminalInputAllowed() {
		t.Fatal("terminal input must be disabled")
	}
	if _, err := readSecret("password: "); !errors.Is(err, errNonInteractive) {
		t.Fatalf("readSecret err = %v", err)
	}
	if _, _, err := getKeyboardInput(); !errors.Is(err, errNonInteractive) {
		t.Fatalf("getKeyboardInput err = %v", err)
	}
}

func TestInteractivePolicyIsScoped(t *testing.T) {
	release := enterNonInteractive(true)
	release()
	if !terminalInputAllowed() {
		t.Fatal("terminal input must be enabled after release")
	}
}
