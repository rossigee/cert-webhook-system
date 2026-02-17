package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	maxReconnectAttempts = 5
	initialBackoff       = 1 * time.Second
	maxBackoff           = 30 * time.Second
)

// Client represents a RabbitMQ client
type Client struct {
	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
	closed  bool
}

// NewClient creates a new RabbitMQ client
func NewClient(url string) (*Client, error) {
	client := &Client{
		url: url,
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	client.watchConnection()

	return client, nil
}

// connect establishes a connection to RabbitMQ
func (c *Client) connect() error {
	var err error

	c.conn, err = amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	c.channel, err = c.conn.Channel()
	if err != nil {
		_ = c.conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	return nil
}

// watchConnection listens for connection close events and triggers reconnection
func (c *Client) watchConnection() {
	go func() {
		closeCh := c.conn.NotifyClose(make(chan *amqp.Error, 1))
		err := <-closeCh
		if err == nil {
			return // graceful close
		}

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()

		_ = c.reconnectWithBackoff()
	}()
}

// reconnectWithBackoff attempts to reconnect with exponential backoff
func (c *Client) reconnectWithBackoff() error {
	for attempt := range maxReconnectAttempts {
		backoff := time.Duration(math.Min(
			float64(initialBackoff)*math.Pow(2, float64(attempt)),
			float64(maxBackoff),
		))
		time.Sleep(backoff)

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return fmt.Errorf("client closed during reconnection")
		}

		if err := c.reconnect(); err != nil {
			c.mu.Unlock()
			continue
		}

		c.mu.Unlock()
		c.watchConnection()
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxReconnectAttempts)
}

// reconnect attempts to reconnect to RabbitMQ
func (c *Client) reconnect() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil && !c.conn.IsClosed() {
		_ = c.conn.Close()
	}

	return c.connect()
}

// ensureConnection ensures the connection is healthy and reconnects if necessary
func (c *Client) ensureConnection() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.conn.IsClosed() || c.channel == nil {
		return c.reconnect()
	}
	return nil
}

// Publish publishes a message to RabbitMQ
func (c *Client) Publish(ctx context.Context, exchange, routingKey string, message any) error {
	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to ensure connection: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Declare exchange (idempotent)
	if err := c.channel.ExchangeDeclare(
		exchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = c.channel.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// HealthCheck performs a health check on the RabbitMQ connection
func (c *Client) HealthCheck() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.conn.IsClosed() {
		return fmt.Errorf("connection is closed")
	}
	if c.channel == nil {
		return fmt.Errorf("channel is nil")
	}

	// Use a passive queue inspect on a non-existent queue to verify channel liveness.
	// QueueInspect returns an error for non-existent queues, but the channel stays open
	// if the connection is healthy. We use an empty-name auto-delete queue declare instead.
	_, err := c.channel.QueueDeclare(
		"",    // empty name = server-generated
		false, // durable
		true,  // auto-delete
		true,  // exclusive
		false, // no-wait (we want the server response)
		nil,
	)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Close closes the RabbitMQ connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true

	var err error

	if c.channel != nil {
		if channelErr := c.channel.Close(); channelErr != nil {
			err = fmt.Errorf("failed to close channel: %w", channelErr)
		}
	}

	if c.conn != nil {
		if connErr := c.conn.Close(); connErr != nil {
			if err != nil {
				err = fmt.Errorf("%v; failed to close connection: %w", err, connErr)
			} else {
				err = fmt.Errorf("failed to close connection: %w", connErr)
			}
		}
	}

	return err
}
