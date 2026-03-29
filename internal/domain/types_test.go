package domain

import "testing"

func TestEnumValidation(t *testing.T) {
	tests := []struct {
		name string
		got  bool
	}{
		{name: "schedule kind valid", got: ScheduleKindInterval.IsValid()},
		{name: "schedule kind invalid", got: ScheduleKind("bad").IsValid()},
		{name: "trigger type valid", got: RunTriggerTypeManual.IsValid()},
		{name: "trigger type invalid", got: RunTriggerType("bad").IsValid()},
		{name: "run status valid", got: RunStatusSucceeded.IsValid()},
		{name: "run status invalid", got: RunStatus("bad").IsValid()},
		{name: "policy valid", got: ConcurrencyPolicyQueue.IsValid()},
		{name: "policy invalid", got: ConcurrencyPolicy("bad").IsValid()},
	}

	want := map[string]bool{
		"schedule kind valid":   true,
		"schedule kind invalid": false,
		"trigger type valid":    true,
		"trigger type invalid":  false,
		"run status valid":      true,
		"run status invalid":    false,
		"policy valid":          true,
		"policy invalid":        false,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != want[tt.name] {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, want[tt.name])
			}
		})
	}
}
