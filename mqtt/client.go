package mqtt

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func NewClient(broker, clientID string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(10 * time.Second)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	fmt.Println("✅ MQTT connected:", broker)
	return client, nil
}

func Publish(client mqtt.Client, topic string, payload string) error {
	token := client.Publish(topic, 0, false, payload)
	token.Wait()
	return token.Error()
}