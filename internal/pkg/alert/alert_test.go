package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogAlerterAL01(t *testing.T) {
	// AL-01 / AC-051
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	alerter := NewLogAlerter(logger)

	occurred := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	detail := "expected parent_slot=100 previous_blockhash=aaa; actual parent_slot=100 previous_blockhash=bbb"
	err := alerter.SendAlert(context.Background(), Alert{
		Level:      LevelCritical,
		Title:      "sol-scanner halted",
		Chain:      "solana-mainnet",
		Slot:       101,
		Reason:     "HASH_MISMATCH",
		Detail:     detail,
		OccurredAt: occurred,
	})
	if err != nil {
		t.Fatalf("SendAlert error = %v, want nil", err)
	}

	raw := buf.String()
	if !strings.Contains(raw, `"level":"ERROR"`) {
		t.Fatalf("expected ERROR-level log, raw=%s", raw)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log: %v\nraw=%s", err, raw)
	}
	for _, key := range []string{"level", "title", "chain", "slot", "reason", "detail", "occurred_at"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("missing log field %q; entry=%v", key, entry)
		}
	}
	if entry["title"] != "sol-scanner halted" {
		t.Fatalf("title = %v", entry["title"])
	}
	if entry["chain"] != "solana-mainnet" {
		t.Fatalf("chain = %v", entry["chain"])
	}
	if entry["slot"] != float64(101) {
		t.Fatalf("slot = %v, want 101", entry["slot"])
	}
	if entry["reason"] != "HASH_MISMATCH" {
		t.Fatalf("reason = %v", entry["reason"])
	}
	if entry["detail"] != detail {
		t.Fatalf("detail = %v", entry["detail"])
	}
	if entry["occurred_at"] != occurred.Format(time.RFC3339) && entry["occurred_at"] != occurred.Format(time.RFC3339Nano) {
		t.Fatalf("occurred_at = %v, want %s", entry["occurred_at"], occurred.Format(time.RFC3339Nano))
	}
}
