package main

import (
	"strings"
	"testing"
)

func TestArmOnceAndIfUnarmedSaysAlreadyArmed(t *testing.T) {
	config := capturedGroupConfig(t, observedSpec)
	code, out, errs := gentalent(t, "--config", config, "arm", "--live")
	if code != 0 || out != "Armed for new messages only. Historical commands will not be processed.\n" {
		t.Fatalf("B160 arm: %d %q %q", code, out, errs)
	}
	if code, out, errs := gentalent(t, "--config", config, "arm", "--live", "--if-unarmed"); code != 0 || out != "Already armed.\n" {
		t.Fatalf("B199 arm --if-unarmed: %d %q %q", code, out, errs)
	}
	code, out, errs = gentalent(t, "--config", config, "arm", "--live")
	if code != 1 || out != "" || !strings.Contains(errs, "already armed. Restart run normally; rearming could discard missed commands") {
		t.Fatalf("B160 rearming is refused: %d %q %q", code, out, errs)
	}
	if code, out, _ := gentalent(t, "--config", config, "arm"); code != 0 || !strings.HasPrefix(out, "Armed") {
		t.Fatalf("B188 without --live arm uses the dry-run database: %d %q", code, out)
	}
}
