package main

import (
	"encoding/json"
	"fmt"
	"HomeNetSentinel/config"
	"HomeNetSentinel/mqtt"
	"time"
)

type Status struct {
	Device    string  `json:"device"`
	Online    bool    `json:"online"`
	Timestamp int64   `json:"ts"`
}

func main() {
	fmt.Println("🚀 HomeNetSentinel 启动成功")
	cfg := config.Load()

	client, err := mqtt.NewClient(cfg.MQTTBroker, cfg.MQTTClientID)
	if err != nil {
		fmt.Println("❌ MQTT 连接失败:", err)
		return
	}
	defer client.Disconnect(250)

	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status := Status{
			Device:    cfg.DeviceName,
			Online:    true,
			Timestamp: time.Now().Unix(),
		}
		data, _ := json.Marshal(status)
		_ = mqtt.Publish(client, cfg.MQTTTopic, string(data))
		fmt.Println("📤 上报状态:", string(data))
	}
}