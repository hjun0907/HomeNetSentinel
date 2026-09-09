package config

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// Target 是一个被监控的在场检测目标（手机等设备）
type Target struct {
	Name string
	IP   string
}

type Config struct {
	BrokerHost string
	BrokerPort int
	Username   string
	Password   string
	ClientID   string

	Targets                []Target
	WanInterface           string
	WanStatusInterface     string
	WanIpv6StatusInterface string
	WanRateRefreshSeconds  int
}

func Load() Config {
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

	port, err := strconv.Atoi(strings.TrimSpace(getHNS("BROKER_PORT", "1883")))
	if err != nil || port <= 0 || port > 65535 {
		port = 1883
	}

	rate, err := strconv.Atoi(strings.TrimSpace(getHNS("WAN_RATE_REFRESH_INTERVAL_SECONDS", "3")))
	if err != nil || rate <= 0 {
		rate = 3
	}

	return Config{
		BrokerHost:             strings.TrimSpace(getHNS("BROKER_HOST", "")),
		BrokerPort:             port,
		Username:               getHNS("MQTT_USERNAME", ""),
		Password:               getHNS("MQTT_PASSWORD", ""),
		ClientID:               "lan_presence_" + strings.ReplaceAll(uuidString(), "-", ""),
		Targets:                parseTargets(os.Getenv("HNS_TARGETS")),
		WanInterface:           getHNS("WAN_INTERFACE", "pppoe-wan"),
		WanStatusInterface:     getHNS("WAN_STATUS_INTERFACE", "wan"),
		WanIpv6StatusInterface: getHNS("WAN_IPV6_STATUS_INTERFACE", "wan_6"),
		WanRateRefreshSeconds:  rate,
	}
}

// parseTargets 解析 "名称|IP;名称|IP" 格式的目标列表
func parseTargets(raw string) []Target {
	var result []Target
	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		fields := strings.SplitN(pair, "|", 2)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		ip := strings.TrimSpace(fields[1])
		if name == "" || ip == "" || net.ParseIP(ip) == nil {
			continue
		}
		result = append(result, Target{Name: name, IP: ip})
	}
	return result
}

func uuidString() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/uuid")
	if err != nil {
		// 如果无法读取 UUID，返回时间戳和进程 ID 作为替代
		return strconv.Itoa(os.Getpid())
	}
	return strings.TrimSpace(string(b))
}
