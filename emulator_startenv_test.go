package portalis

import "testing"

// TestStartUsesRecordedStartEnv verifies that a PTY spawned via Start()
// (no explicit env) still receives the env recorded with SetStartEnv.
// Regression: Automata used to lose PI_CODING_AGENT_DIR when the emulator
// was started through the ItemSelectedMsg path (Start with nil env).
func TestStartUsesRecordedStartEnv(t *testing.T) {
	em := NewEmulator("s1", "chat", "/bin/sh", nil)
	em.SetStartEnv([]string{"PI_CODING_AGENT_DIR=/tmp/agent"})

	if got := em.effectiveEnv(nil); len(got) != 1 || got[0] != "PI_CODING_AGENT_DIR=/tmp/agent" {
		t.Fatalf("effectiveEnv(nil) = %#v, want recorded startEnv", got)
	}

	t.Log("explicit env wins over the recorded one")
	if got := em.effectiveEnv([]string{"X=1"}); len(got) != 1 || got[0] != "X=1" {
		t.Fatalf("effectiveEnv(explicit) = %#v, want explicit env", got)
	}
}

// TestStartEnvGetterRoundTrip covers SetStartEnv/StartEnv.
func TestStartEnvGetterRoundTrip(t *testing.T) {
	em := NewEmulator("s2", "chat", "/bin/sh", nil)
	if got := em.StartEnv(); got != nil {
		t.Fatalf("StartEnv before set = %#v, want nil", got)
	}
	env := []string{"A=1", "B=2"}
	em.SetStartEnv(env)
	got := em.StartEnv()
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Fatalf("StartEnv = %#v, want %#v", got, env)
	}
}
