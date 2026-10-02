// Package events publishes and subscribes JSON messages on an MQTT broker,
// the stack's MQTT-INSECURE-JSON profile (CONTRACT.md C17; DECISIONS D3).
//
// It is MQTT and nothing else: no Event Handler, no push orchestration (that
// is in the orchestration package). The connection is plain TCP without
// broker authentication, which is what INSECURE in the interface name means.
//
// Everything is explicit: Connect opens one connection and never reconnects on
// its own; a message is sent when you call Publish; Subscribe calls your
// handler for each message on the MQTT client's delivery goroutine until you
// Unsubscribe or Close.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
)

// QoS used for publish and subscribe: at least once.
const QoS byte = 1

// Client is one MQTT connection.
type Client struct {
	c mqtt.Client
}

// Message is one received MQTT message.
type Message struct {
	Topic   string
	Payload []byte
}

// DecodeJSON decodes the payload into v.
func (m Message) DecodeJSON(v any) error {
	if err := json.Unmarshal(m.Payload, v); err != nil {
		return fmt.Errorf("events: decode message on %s: %w", m.Topic, err)
	}
	return nil
}

// Connect opens a connection to brokerURL (for example tcp://localhost:1883)
// with the given client ID, and waits until the broker accepts it or ctx ends.
// A dropped connection is reported by the next call; it is not re-opened.
func Connect(ctx context.Context, brokerURL, clientID string) (*Client, error) {
	if clientID == "" {
		return nil, errors.New("events: client ID is empty")
	}
	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetConnectTimeout(timeoutFrom(ctx, 10*time.Second))
	c := mqtt.NewClient(opts)
	if err := wait(ctx, c.Connect()); err != nil {
		return nil, fmt.Errorf("events: connect %s: %w", brokerURL, err)
	}
	return &Client{c: c}, nil
}

// Publish sends payload as JSON on topic and waits for the broker's
// acknowledgement or the end of ctx.
func (c *Client) Publish(ctx context.Context, topic string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("events: encode payload: %w", err)
	}
	if err := wait(ctx, c.c.Publish(topic, QoS, false, b)); err != nil {
		return fmt.Errorf("events: publish %s: %w", topic, err)
	}
	return nil
}

// Subscribe registers handler for messages on topic (MQTT wildcards allowed)
// and waits for the broker's acknowledgement. handler runs on the MQTT
// client's delivery goroutine; keep it short.
func (c *Client) Subscribe(ctx context.Context, topic string, handler func(Message)) error {
	if handler == nil {
		return errors.New("events: handler is nil")
	}
	tok := c.c.Subscribe(topic, QoS, func(_ mqtt.Client, m mqtt.Message) {
		handler(Message{Topic: m.Topic(), Payload: m.Payload()})
	})
	if err := wait(ctx, tok); err != nil {
		return fmt.Errorf("events: subscribe %s: %w", topic, err)
	}
	return nil
}

// Unsubscribe stops delivery for topic.
func (c *Client) Unsubscribe(ctx context.Context, topic string) error {
	if err := wait(ctx, c.c.Unsubscribe(topic)); err != nil {
		return fmt.Errorf("events: unsubscribe %s: %w", topic, err)
	}
	return nil
}

// Close disconnects, giving in-flight work up to 250 ms.
func (c *Client) Close() {
	c.c.Disconnect(250)
}

// Interface returns the interface a publisher registers (registry.Register)
// so consumers can find its topic: templateName MQTT-INSECURE-JSON, protocol
// mqtt, and the broker address, port and topic as string properties. The
// topic goes in basePath, which the orchestrator returns as service.serviceUri.
func Interface(brokerAddress string, brokerPort int, topic string) models.Interface {
	return models.Interface{
		TemplateName: models.MQTTInterfaceName,
		Protocol:     models.MQTTProtocol,
		Policy:       models.PolicyNone,
		Properties: map[string]string{
			models.PropertyAccessAddresses: brokerAddress,
			models.PropertyAccessPort:      strconv.Itoa(brokerPort),
			models.PropertyBasePath:        topic,
		},
	}
}

func wait(ctx context.Context, tok mqtt.Token) error {
	select {
	case <-tok.Done():
		return tok.Error()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func timeoutFrom(ctx context.Context, fallback time.Duration) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 {
			return d
		}
	}
	return fallback
}
