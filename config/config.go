package config

import (
	"os"
	"strings"
)

type Config struct {
	MQTTBroker   string
	MQTTClientID  string
	MQTTTopic     string
	DeviceName    string
	Interval      int
}

func Load() Config {
	get := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	return Config{
		MQTTBroker:  get("MQTT_BROKER", "tcp://127.0.0.1:1883"),
		MQTTClientID: get("MQTT_CLIENTID", "homenetsentinel-"+strings.ReplaceAll(uuidString(), "-", "")),
		MQTTTopic:    get("MQTT_TOPIC", "home/sentinel/device"),
		DeviceName:   get("DEVICE_NAME", "HomeNetSentinel"),
		Interval:     30,
	}
}

func uuidString() string {
	b, _ := os.ReadFile("/proc/sys/kernel/random/uuid")
	return strings.TrimSpace(string(b))
}