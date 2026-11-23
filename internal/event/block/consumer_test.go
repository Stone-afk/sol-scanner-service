package block

import (
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestBlockConsumerRequiresInjectedClient(t *testing.T) {
	// notification-platform style: event package does not dial Kafka itself.
	t.Parallel()
	if _, err := NewConsumer(nil, "t", nil); err == nil {
		t.Fatal("NewConsumer(nil): want error")
	}
	if _, err := NewConsumer(&kafka.Consumer{}, "", nil); err == nil {
		t.Fatal("NewConsumer empty topic: want error")
	}
}

func TestOffsetFromTopicPartitionRoundTrip(t *testing.T) {
	t.Parallel()
	topic := "t"
	tp := kafka.TopicPartition{Topic: &topic, Partition: 2, Offset: 7}
	off := OffsetFromTopicPartition(tp)
	if off.Topic != "t" || off.Partition != 2 || off.Offset != 7 {
		t.Fatalf("OffsetFromTopicPartition = %+v", off)
	}
	back := off.TopicPartition()
	if back.Topic == nil || *back.Topic != "t" || back.Partition != 2 || int64(back.Offset) != 7 {
		t.Fatalf("TopicPartition = %+v", back)
	}
}

func TestIsRetriablePollError(t *testing.T) {
	// Auto-create race: subscribe before the topic exists must not kill the process.
	t.Parallel()
	cases := []struct {
		code kafka.ErrorCode
		want bool
	}{
		{kafka.ErrUnknownTopic, true},
		{kafka.ErrUnknownTopicOrPart, true},
		{kafka.ErrAllBrokersDown, false},
		{kafka.ErrFatal, false},
	}
	for _, tc := range cases {
		got := isRetriablePollError(kafka.NewError(tc.code, "", false))
		if got != tc.want {
			t.Fatalf("code=%v: got %v want %v", tc.code, got, tc.want)
		}
	}
}
