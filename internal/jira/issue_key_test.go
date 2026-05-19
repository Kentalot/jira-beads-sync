package jira

import "testing"

func TestValidateIssueKey(t *testing.T) {
	tests := []struct {
		key string
		ok  bool
	}{
		{"PROJ-1", true},
		{"PROJ-123", true},
		{"A-1", true},
		{"AB-999999", true},
		{"", false},
		{"PROJ", false},
		{"PROJ-", false},
		{"PROJ-0", false},
		{"PROJ-abc", false},
		{"../evil", false},
		{"PROJ-123/extra", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := ValidateIssueKey(tt.key)
			if tt.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
