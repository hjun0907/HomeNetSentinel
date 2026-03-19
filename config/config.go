package config

import (
	"os"
	"strings"
	"strconv"
)

type Config struct {
	MQTTBroker   string
	MQTTClientID  string
	MQTTTopic     string
	DeviceName    string
	Interval      int
	Username      string
	Password      string
}

func Load() Config {
	get := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	// 支持 HNS_ 前缀的环境变量
	getHNS := func(key, def string) string {
		if v := os.Getenv("HNS_" + key); v != "" {
			return v
		}
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	// 从 HNS_ 环境变量构建 broker URL
	brokerHost := getHNS("BROKER_HOST", "127.0.0.1")
	brokerPort := getHNS("BROKER_PORT", "1883")
	broker := "tcp://" + brokerHost + ":" + brokerPort

	// 获取间隔时间
	intervalStr := getHNS("WAN_RATE_REFRESH_INTERVAL_SECONDS", "30")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		interval = 30
	}

	return Config{
		MQTTBroker:   broker,
		MQTTClientID: getHNS("MQTT_CLIENTID", "homenetsentinel-"+strings.ReplaceAll(uuidString(), "-", "")),
		MQTTTopic:    getHNS("MQTT_TOPIC", "home/sentinel/device"),
		DeviceName:   getHNS("DEVICE_NAME", "HomeNetSentinel"),
		Username:     getHNS("MQTT_USERNAME", ""),
		Password:     getHNS("MQTT_PASSWORD", ""),
		Interval:     interval,
	}
}

func uuidString() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/uuid")
	if err != nil {
		// 如果无法读取 UUID，返回时间戳和进程 ID 作为替代
		return strconv.Itoa(os.Getpid())
	}
	return strings.TrimSpace(string(b))
}