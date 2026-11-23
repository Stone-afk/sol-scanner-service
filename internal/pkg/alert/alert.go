package alert

import (
	"context"
	"log/slog"
	"time"
)

const LevelCritical = "CRITICAL"

type Alert struct {
	Level      string
	Title      string
	Chain      string
	Slot       uint64
	Reason     string
	Detail     string
	OccurredAt time.Time
}

type Alerter interface {
	SendAlert(ctx context.Context, alert Alert) error
}

type LogAlerter struct {
	logger *slog.Logger
}

func NewLogAlerter(logger *slog.Logger) LogAlerter {
	return LogAlerter{logger: logger}
}

func (a LogAlerter) SendAlert(ctx context.Context, al Alert) error {
	a.logger.LogAttrs(ctx, slog.LevelError, al.Title,
		slog.String("level", al.Level),
		slog.String("title", al.Title),
		slog.String("chain", al.Chain),
		slog.Uint64("slot", al.Slot),
		slog.String("reason", al.Reason),
		slog.String("detail", al.Detail),
		slog.Time("occurred_at", al.OccurredAt),
	)
	return nil
}
