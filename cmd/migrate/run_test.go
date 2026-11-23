package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{
			name:       "MG-01",
			args:       nil,
			wantCode:   2,
			wantStderr: "usage: migrate <up|down|status>",
		},
		{
			name:     "MG-02",
			args:     []string{"sideways", "--config", "x"},
			wantCode: 2,
		},
		{
			name:     "MG-03",
			args:     []string{"up"},
			wantCode: 2,
		},
		{
			name:       "MG-04",
			args:       []string{"up", "--config", "/nonexistent.yaml"},
			wantCode:   1,
			wantStderr: "load migration configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tt.args, &stdout, &stderr, func(string) (string, bool) {
				t.Fatal("usage/config argument errors must not read environment")
				return "", false
			})
			if code != tt.wantCode {
				t.Fatalf("run(%v) code = %d, want %d; stderr=%q", tt.args, code, tt.wantCode, stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("run(%v) stderr = %q, want fragment %q", tt.args, stderr.String(), tt.wantStderr)
			}
		})
	}
}
