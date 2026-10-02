package events

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
)

func TestInterface(t *testing.T) {
	i := Interface("broker.local", 1883, "sensors/temperature")
	b, _ := json.Marshal(i)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	want := map[string]any{"accessAddresses": "broker.local", "accessPort": "1883", "basePath": "sensors/temperature"}
	props := m["properties"].(map[string]any)
	for k, v := range want {
		if props[k] != v {
			t.Errorf("property %s = %#v, want %#v", k, props[k], v)
		}
	}
	if m["templateName"] != models.MQTTInterfaceName || m["protocol"] != "mqtt" || m["policy"] != "NONE" {
		t.Errorf("interface = %v", m)
	}
}

func TestMessageDecode(t *testing.T) {
	var v struct {
		Value int `json:"value"`
	}
	if err := (Message{Topic: "t", Payload: []byte(`{"value":80}`)}).DecodeJSON(&v); err != nil || v.Value != 80 {
		t.Errorf("%+v %v", v, err)
	}
	if err := (Message{Topic: "t", Payload: []byte(`nope`)}).DecodeJSON(&v); err == nil {
		t.Error("non-JSON accepted")
	}
}

// A refused connection is an error, returned once; no reconnect loop.
func TestConnectRefusedIsAnError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // nothing listens there now

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := Connect(ctx, "tcp://"+addr, "test-client"); err == nil {
		t.Fatal("connect to a closed port succeeded")
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("connect took %v; it should fail at once, not retry", time.Since(start))
	}
	if _, err := Connect(ctx, "tcp://"+addr, ""); err == nil {
		t.Error("empty client ID accepted")
	}
}
