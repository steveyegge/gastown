package bus

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// RedisClient is the interface for Redis operations needed by the bus.
// This avoids a hard dependency on a specific Redis library — the caller
// provides the concrete client. For production, use go-redis/v9.
type RedisClient interface {
	// Publish publishes a message to a channel.
	Publish(ctx context.Context, channel string, message []byte) error

	// Subscribe subscribes to a channel and calls fn for each message.
	// Should block until ctx is cancelled or an error occurs.
	Subscribe(ctx context.Context, channel string, fn func([]byte)) error

	// Close closes the Redis connection.
	Close() error
}

// RedisBus is a Redis-backed pub/sub bus for distributed step events.
type RedisBus struct {
	client RedisClient
	logger *log.Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.RWMutex
	subscribers map[string][]subscriber
	nextID      int
	listening   map[string]context.CancelFunc // active subscription goroutines
}

// NewRedisBus creates a Redis-backed bus.
func NewRedisBus(client RedisClient, logger *log.Logger) *RedisBus {
	ctx, cancel := context.WithCancel(context.Background())
	return &RedisBus{
		client:      client,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[string][]subscriber),
		listening:   make(map[string]context.CancelFunc),
	}
}

// Publish sends an event to Redis.
func (b *RedisBus) Publish(ev StepEvent) error {
	data, err := ev.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}
	return b.client.Publish(b.ctx, ev.Channel(), data)
}

// Subscribe registers a local callback and starts a Redis subscription if needed.
func (b *RedisBus) Subscribe(channel string, fn func(StepEvent)) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subscribers[channel] = append(b.subscribers[channel], subscriber{id: id, fn: fn})

	// Start Redis listener for this channel if not already running.
	if _, ok := b.listening[channel]; !ok {
		subCtx, subCancel := context.WithCancel(b.ctx)
		b.listening[channel] = subCancel
		go b.listen(subCtx, channel)
	}
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[channel]
		for i, s := range subs {
			if s.id == id {
				b.subscribers[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		// Stop listener if no more subscribers.
		if len(b.subscribers[channel]) == 0 {
			if cancel, ok := b.listening[channel]; ok {
				cancel()
				delete(b.listening, channel)
			}
		}
	}
}

// listen runs a Redis subscription in a goroutine with reconnect.
func (b *RedisBus) listen(ctx context.Context, channel string) {
	for {
		err := b.client.Subscribe(ctx, channel, func(data []byte) {
			ev, err := UnmarshalStepEvent(data)
			if err != nil {
				b.logger.Printf("bus: unmarshal error on %s: %v", channel, err)
				return
			}
			b.dispatch(channel, ev)
		})

		if ctx.Err() != nil {
			return // Context cancelled, stop listening.
		}
		if err != nil {
			b.logger.Printf("bus: subscription error on %s: %v, reconnecting in 5s", channel, err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

// dispatch fans out an event to all local subscribers.
func (b *RedisBus) dispatch(channel string, ev StepEvent) {
	b.mu.RLock()
	subs := b.subscribers[channel]
	copied := make([]subscriber, len(subs))
	copy(copied, subs)
	b.mu.RUnlock()

	for _, s := range copied {
		s.fn(ev)
	}
}

// Close stops all subscriptions and closes the Redis connection.
func (b *RedisBus) Close() error {
	b.cancel()
	return b.client.Close()
}
