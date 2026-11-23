package ssh05

import "testing"

func TestLaunchOpenTUIMissingFallsBack(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	handled, _ := launchOpenTUI()
	if handled {
		t.Fatal("launchOpenTUI must report false when ssh05-ui is absent")
	}
}
