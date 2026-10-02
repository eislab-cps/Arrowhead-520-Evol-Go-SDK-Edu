package models

// MQTTInterfaceName is the interface name of the stack's MQTT profile
// (CONTRACT.md C17). A publisher registers it as the templateName of an
// interface with protocol "mqtt".
const MQTTInterfaceName = "MQTT-INSECURE-JSON"

// MQTTProtocol is the interface protocol used with MQTTInterfaceName.
const MQTTProtocol = "mqtt"
