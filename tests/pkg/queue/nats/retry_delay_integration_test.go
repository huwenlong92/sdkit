//go:build sdkit_queue_nats

package nats_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
	natsqueue "github.com/huwenlong92/sdkit/pkg/queue/nats"

	natsgo "github.com/nats-io/nats.go"
)

func TestRuntimeRetryDelayOverridesNATSFallback(t *testing.T) {
	addr := strings.TrimSpace(os.Getenv("SDKIT_NATS_TEST_ADDR"))
	if addr == "" {
		t.Skip("set SDKIT_NATS_TEST_ADDR to run the NATS retry-delay integration test")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	stream := "SDKT_RETRY_" + suffix
	queueName := "retry"
	client, err := natsqueue.New(queue.Config{
		Driver:      "nats",
		Addr:        addr,
		Concurrency: 1,
		Queues:      map[string]int{queueName: 1},
		NATS: queue.NATSConfig{
			Stream: stream, SubjectPrefix: "sdkit.retry." + suffix, DurablePrefix: "sdkit_retry_" + suffix,
			AckWait: 5 * time.Second, MaxDeliver: 3, Storage: "memory", Replicas: 1,
			FetchBatch: 1, FetchWait: 100 * time.Millisecond, RetryDelay: 50 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		connection, connectErr := natsgo.Connect(addr)
		if connectErr != nil {
			return
		}
		defer connection.Close()
		jetStream, jetStreamErr := connection.JetStream()
		if jetStreamErr == nil {
			_ = jetStream.DeleteStream(stream)
		}
	})

	attempts := make(chan time.Time, 2)
	client.Handle("retry-delay-contract", func(_ context.Context, message *queue.Message) error {
		attempts <- time.Now()
		if message.RetryCount == 0 {
			return queue.RetryableAfter(750*time.Millisecond, errors.New("retry delay contract"))
		}
		return nil
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- client.Run(runCtx)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := client.Enqueue(context.Background(), queue.Task{
		ID: "retry-delay-" + suffix, Type: "retry-delay-contract", Queue: queueName, Payload: map[string]any{"run": suffix},
	}, queue.Queue(queueName), queue.MaxRetry(2)); err != nil {
		t.Fatal(err)
	}

	first := waitAttempt(t, attempts)
	second := waitAttempt(t, attempts)
	delay := second.Sub(first)
	if delay < 650*time.Millisecond || delay > 3*time.Second {
		t.Fatalf("NATS retry delay=%s, want runtime RetryIn near 750ms instead of 50ms fallback", delay)
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(3 * time.Second):
		t.Fatal("NATS worker did not stop")
	}
}

func TestRunReconcilesExistingConsumerRetryLimits(t *testing.T) {
	addr := strings.TrimSpace(os.Getenv("SDKIT_NATS_TEST_ADDR"))
	if addr == "" {
		t.Skip("set SDKIT_NATS_TEST_ADDR to run the NATS consumer reconciliation integration test")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	stream := "SDKT_CONSUMER_" + suffix
	queueName := "reconcile"
	taskType := "consumer-reconcile"
	config := func(maxDeliver int) queue.Config {
		return queue.Config{
			Driver:      "nats",
			Addr:        addr,
			Concurrency: 1,
			Queues:      map[string]int{queueName: 1},
			NATS: queue.NATSConfig{
				Stream: stream, SubjectPrefix: "sdkit.consumer." + suffix, DurablePrefix: "sdkit_consumer_" + suffix,
				AckWait: 5 * time.Second, MaxDeliver: maxDeliver, Storage: "memory", Replicas: 1,
				FetchBatch: 1, FetchWait: 100 * time.Millisecond, RetryDelay: 50 * time.Millisecond,
			},
		}
	}

	connection, err := natsgo.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	jetStream, err := connection.JetStream()
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = jetStream.DeleteStream(stream)
		connection.Close()
	})

	run := func(maxDeliver int) {
		client, err := natsqueue.New(config(maxDeliver))
		if err != nil {
			t.Fatal(err)
		}
		client.Handle(taskType, func(context.Context, *queue.Message) error { return nil })
		runCtx, cancel := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() {
			runDone <- client.Run(runCtx)
		}()

		durable := "sdkit_consumer_" + suffix + "_" + queueName + "_" + taskType
		waitConsumerMaxDeliver(t, jetStream, stream, durable, maxDeliver)
		cancel()
		select {
		case <-runDone:
		case <-time.After(3 * time.Second):
			t.Fatal("NATS worker did not stop")
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}

	run(2)
	run(4)
}

func waitConsumerMaxDeliver(t *testing.T, jetStream natsgo.JetStreamContext, stream string, durable string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, err := jetStream.ConsumerInfo(stream, durable)
		if err == nil && info.Config.MaxDeliver == want {
			return
		}
		if err != nil && !errors.Is(err, natsgo.ErrConsumerNotFound) {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("consumer %s max_deliver did not become %d", durable, want)
}

func waitAttempt(t *testing.T, attempts <-chan time.Time) time.Time {
	t.Helper()
	select {
	case value := <-attempts:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for NATS delivery")
		return time.Time{}
	}
}
