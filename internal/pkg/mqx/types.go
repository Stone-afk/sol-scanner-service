package mqx

import "context"

// FlushTimeoutMs is the Flush timeout in milliseconds. Must remain 1000
// (not int(time.Second), which librdkafka treats as ~11.5 days).
const FlushTimeoutMs = 1000

// Keyed is a keyed Kafka message. Value semantics only.
type Keyed[T any] struct {
	Key   string
	Value T
}

// Producer publishes keyed batches and closes the underlying client.
type Producer[T any] interface {
	ProduceBatch(ctx context.Context, msgs []Keyed[T]) error
	Close()
}
