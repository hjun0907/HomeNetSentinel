package mqtt

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// NewClient 建立 MQTT 连接，willTopic/willPayload 为遗嘱消息（设备离线时由 broker 发布）
func NewClient(broker, clientID, username, password, willTopic, willPayload string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetWill(willTopic, willPayload, 1, true)

	// 添加认证信息
	if username != "" {
		opts.SetUsername(username)
		if password != "" {
			opts.SetPassword(password)
		}
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	fmt.Println("✅ MQTT connected:", broker)
	return client, nil
}

// PublishRetained 以 QoS1 + retained 方式发布消息（HA 发现与状态主题均需 retained）
// 使用 5 秒超时：QoS1 需等 broker 确认（PUBACK），若 broker 响应慢或网络抖动，
// token.Wait() 会永久阻塞，导致主循环冻住（state.json 不再更新）。
func PublishRetained(client mqtt.Client, topic string, payload string) error {
	token := client.Publish(topic, 1, true, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT publish 超时 (5s): %s", topic)
	}
	return token.Error()
}
