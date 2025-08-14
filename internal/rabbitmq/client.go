package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client represents a RabbitMQ client
type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
}

// NewClient creates a new RabbitMQ client
func NewClient(url string) (*Client, error) {
	client := &Client{
		url: url,
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return client, nil
}

// connect establishes a connection to RabbitMQ
func (c *Client) connect() error {
	var err error

	// Connect to RabbitMQ
	c.conn, err = amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create channel
	c.channel, err = c.conn.Channel()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	return nil
}

// reconnect attempts to reconnect to RabbitMQ
func (c *Client) reconnect() error {
	// Close existing connections
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	// Attempt to reconnect
	return c.connect()
}

// ensureConnection ensures the connection is healthy and reconnects if necessary
func (c *Client) ensureConnection() error {
	if c.conn == nil || c.conn.IsClosed() || c.channel == nil {
		return c.reconnect()
	}
	return nil
}

// Publish publishes a message to RabbitMQ
func (c *Client) Publish(ctx context.Context, exchange, routingKey string, message interface{}) error {
	// Ensure connection is healthy
	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to ensure connection: %w", err)
	}

	// Declare exchange (idempotent)
	if err := c.channel.ExchangeDeclare(
		exchange,
		"topic", // type
		true,    // durable
		false,   // auto-deleted
		false,   // internal
		false,   // no-wait
		nil,     // arguments
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Convert message to JSON
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Publish message
	err = c.channel.PublishWithContext(
		ctx,
		exchange,   // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // make message persistent
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
	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("connection unhealthy: %w", err)
	}

	// Try to declare a temporary queue to test the connection
	_, err := c.channel.QueueDeclare(
		"health-check", // name
		false,          // durable
		true,           // delete when unused
		true,           // exclusive
		true,           // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Close closes the RabbitMQ connection
func (c *Client) Close() error {
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
