package picker

import "testing"

func TestAttachResultCarriesShellCommandAndTarget(t *testing.T) {
	b := &mockBackend{binaryName: "zmx"}

	for _, tt := range []struct {
		dir  string
		want string
	}{
		{"", `ZPICK_SESSION="work" zmx attach work`},
		{"/tmp/my work", `cd "/tmp/my work" && ZPICK_SESSION="work" zmx attach work`},
	} {
		t.Run(tt.dir, func(t *testing.T) {
			result := attachResult(b, "new", "work", tt.dir)
			if result.Command != tt.want {
				t.Fatalf("Command = %q, want %q", result.Command, tt.want)
			}
			if result.Target == nil {
				t.Fatal("Target should be set")
			}
			if result.Target.Action != "new" || result.Target.Name != "work" || result.Target.Dir != tt.dir {
				t.Fatalf("Target = %+v", result.Target)
			}
		})
	}
}
