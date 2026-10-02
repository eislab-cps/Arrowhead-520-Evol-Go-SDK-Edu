// Command events subscribes to a topic, publishes one JSON message on it, and
// prints the message when the broker delivers it back.
//
//	go run ./examples/events -broker tcp://localhost:1883 -topic sensors/temperature
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/events"
)

type reading struct {
	Sensor string  `json:"sensor"`
	Value  float64 `json:"value"`
}

func main() {
	broker := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL (plain TCP)")
	topic := flag.String("topic", "sensors/temperature", "topic to publish and subscribe")
	flag.Parse()
	if err := run(*broker, *topic); err != nil {
		fmt.Fprintln(os.Stderr, "events example:", err)
		os.Exit(1)
	}
}

func run(broker, topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := events.Connect(ctx, broker, fmt.Sprintf("events-example-%d", time.Now().UnixNano()))
	if err != nil {
		return err
	}
	defer c.Close()

	got := make(chan events.Message, 1)
	if err := c.Subscribe(ctx, topic, func(m events.Message) { got <- m }); err != nil {
		return err
	}
	fmt.Printf("subscribed to %s\n", topic)

	if err := c.Publish(ctx, topic, reading{Sensor: "t1", Value: 21.5}); err != nil {
		return err
	}
	fmt.Printf("published on %s\n", topic)

	select {
	case m := <-got:
		var r reading
		if err := m.DecodeJSON(&r); err != nil {
			return err
		}
		fmt.Printf("received on %s: sensor=%s value=%.1f\n", m.Topic, r.Sensor, r.Value)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("no message within the deadline: %w", ctx.Err())
	}
}
