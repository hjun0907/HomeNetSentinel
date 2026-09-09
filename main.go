package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"HomeNetSentinel/config"
	"HomeNetSentinel/mqtt"
)

const (
	discoveryPrefix = "homeassistant"
	baseTopic       = "lan/presence"
	targetStateFile = "/etc/homenet-sentinel/target_ids"
	connInterval    = 30 * time.Second
)

// ---------- HA 发现 JSON 结构 ----------

type haDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
}

type targetDiscovery struct {
	Name                   string   `json:"name"`
	UniqueID               string   `json:"unique_id"`
	StateTopic             string   `json:"state_topic"`
	PayloadOn              string   `json:"payload_on"`
	PayloadOff             string   `json:"payload_off"`
	DeviceClass            string   `json:"device_class"`
	JSONAttributesTopic    string   `json:"json_attributes_topic,omitempty"`
	JSONAttributesTemplate string   `json:"json_attributes_template,omitempty"`
	AvailabilityTopic      string   `json:"availability_topic"`
	PayloadAvailable       string   `json:"payload_available"`
	PayloadNotAvailable    string   `json:"payload_not_available"`
	Icon                   string   `json:"icon"`
	Device                 haDevice `json:"device"`
}

type numericDiscovery struct {
	Name                string   `json:"name"`
	UniqueID            string   `json:"unique_id"`
	StateTopic          string   `json:"state_topic"`
	AvailabilityTopic   string   `json:"availability_topic"`
	PayloadAvailable    string   `json:"payload_available"`
	PayloadNotAvailable string   `json:"payload_not_available"`
	Icon                string   `json:"icon"`
	UnitOfMeasurement   string   `json:"unit_of_measurement"`
	StateClass          string   `json:"state_class"`
	DeviceClass         string   `json:"device_class,omitempty"`
	Device              haDevice `json:"device"`
}

type textDiscovery struct {
	Name                string   `json:"name"`
	UniqueID            string   `json:"unique_id"`
	StateTopic          string   `json:"state_topic"`
	AvailabilityTopic   string   `json:"availability_topic"`
	PayloadAvailable    string   `json:"payload_available"`
	PayloadNotAvailable string   `json:"payload_not_available"`
	Icon                string   `json:"icon"`
	Device              haDevice `json:"device"`
}

// ---------- 聚合状态 JSON 结构 ----------

type deviceState struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Status    string `json:"status"`
	Nud       string `json:"nud"`
	Online    bool   `json:"online"`
	UpdatedAt string `json:"updated_at"`
}

type aggregatePayload struct {
	Timestamp   string                 `json:"timestamp"`
	OnlineCount int                    `json:"online_count"`
	TotalCount  int                    `json:"total_count"`
	Devices     map[string]deviceState `json:"devices"`
}

// ---------- WAN 采样 ----------

type wanSample struct {
	DownloadBps float64
	UploadBps   float64
	Conntrack   int64
	RxBytes     uint64
	TxBytes     uint64
}

type wanMonitor struct {
	iface    string
	ok       bool
	lastRx   uint64
	lastTx   uint64
	lastTime time.Time
}

func newWanMonitor(iface string) *wanMonitor {
	m := &wanMonitor{iface: iface, lastTime: time.Now()}
	rx, err1 := readUint64File(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", iface))
	tx, err2 := readUint64File(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", iface))
	if err1 != nil || err2 != nil {
		return m
	}
	m.ok = true
	m.lastRx = rx
	m.lastTx = tx
	return m
}

func (m *wanMonitor) sample() (wanSample, bool) {
	if !m.ok {
		return wanSample{}, false
	}
	rx, err1 := readUint64File(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", m.iface))
	tx, err2 := readUint64File(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", m.iface))
	if err1 != nil || err2 != nil {
		return wanSample{}, false
	}

	now := time.Now()
	elapsed := now.Sub(m.lastTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	rxDelta := uint64(0)
	if rx >= m.lastRx {
		rxDelta = rx - m.lastRx
	}
	txDelta := uint64(0)
	if tx >= m.lastTx {
		txDelta = tx - m.lastTx
	}
	m.lastRx, m.lastTx, m.lastTime = rx, tx, now

	return wanSample{
		DownloadBps: float64(rxDelta) / elapsed,
		UploadBps:   float64(txDelta) / elapsed,
		Conntrack:   readInt64File("/proc/sys/net/netfilter/nf_conntrack_count"),
		RxBytes:     rx,
		TxBytes:     tx,
	}, true
}

type wanStatus struct {
	IPv4   string
	IPv6PD string
}

// readUbusStatus 通过 ubus 查询 OpenWrt 网络接口状态（IPv4 地址 / IPv6-PD 前缀）
func readUbusStatus(iface string) *wanStatus {
	out, err := exec.Command("/bin/ubus", "call", "network.interface."+iface, "status").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}
	var doc map[string]interface{}
	if json.Unmarshal(out, &doc) != nil {
		return nil
	}

	st := &wanStatus{IPv4: "unknown", IPv6PD: "unknown"}
	if arr, ok := doc["ipv4-address"].([]interface{}); ok {
		for _, it := range arr {
			if m, ok := it.(map[string]interface{}); ok {
				if a, ok := m["address"].(string); ok && a != "" {
					st.IPv4 = a
					break
				}
			}
		}
	}
	if arr, ok := doc["ipv6-prefix"].([]interface{}); ok {
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			a, ok := m["address"].(string)
			if !ok || strings.TrimSpace(a) == "" {
				continue
			}
			if mask, ok := m["mask"].(float64); ok && mask > 0 {
				st.IPv6PD = fmt.Sprintf("%s/%d", a, int(mask))
			} else {
				st.IPv6PD = a
			}
			break
		}
	}
	return st
}

// ---------- 邻居表（ARP/NUD）在场检测 ----------

func neighborState(ip string) string {
	out, err := exec.Command("/sbin/ip", "neigh", "show", ip).Output()
	if err != nil {
		return "ERROR"
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "NONE"
	}
	fields := strings.Fields(s)
	return fields[len(fields)-1]
}

func isOnline(nud string) bool {
	switch nud {
	case "REACHABLE", "STALE", "DELAY", "PROBE", "PERMANENT":
		return true
	}
	return false
}

// ---------- 工具函数 ----------

func toID(s string) string {
	return strings.ReplaceAll(s, ".", "_")
}

// deviceStateTopic 每个目标的在线/离线独立状态主题（retained，值 online/offline）
func deviceStateTopic(id string) string {
	return fmt.Sprintf("%s/device/%s/state", baseTopic, id)
}

func readUint64File(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

func readInt64File(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return v
}

func loadTargetIDs(path string) map[string]bool {
	set := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return set
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			set[line] = true
		}
	}
	return set
}

func saveTargetIDs(path string, ids []string) {
	sort.Strings(ids)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(strings.Join(ids, "\n")+"\n"), 0o644)
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func wanDevice() haDevice {
	return haDevice{
		Identifiers:  []string{"lan_presence_wan"},
		Name:         "HomeNet Sentinel",
		Manufacturer: "HomeNet Sentinel",
		Model:        "HomeNet Sentinel",
	}
}

// ---------- 主流程 ----------

func main() {
	fmt.Println("🚀 HomeNetSentinel 启动成功")
	cfg := config.Load()
	if cfg.BrokerHost == "" {
		fmt.Println("❌ broker_host is empty")
		return
	}

	availabilityTopic := baseTopic + "/bridge/status"
	aggregateTopic := baseTopic + "/all"
	wanID := toID(cfg.WanInterface)

	wanDownloadTopic := fmt.Sprintf("%s/wan/%s/download_bps", baseTopic, wanID)
	wanUploadTopic := fmt.Sprintf("%s/wan/%s/upload_bps", baseTopic, wanID)
	wanConnTopic := fmt.Sprintf("%s/wan/%s/conntrack_count", baseTopic, wanID)
	wanRxTotalTopic := fmt.Sprintf("%s/wan/%s/rx_gb_total", baseTopic, wanID)
	wanTxTotalTopic := fmt.Sprintf("%s/wan/%s/tx_gb_total", baseTopic, wanID)
	wanIpv4Topic := fmt.Sprintf("%s/wan/%s/ipv4", baseTopic, wanID)
	wanIpv6PdTopic := fmt.Sprintf("%s/wan/%s/ipv6_pd", baseTopic, wanID)

	broker := fmt.Sprintf("tcp://%s:%d", cfg.BrokerHost, cfg.BrokerPort)
	client, err := mqtt.NewClient(broker, cfg.ClientID, cfg.Username, cfg.Password, availabilityTopic, "offline")
	if err != nil {
		fmt.Println("❌ MQTT 连接失败:", err)
		return
	}
	defer client.Disconnect(250)

	pub := func(topic, payload string) {
		if err := mqtt.PublishRetained(client, topic, payload); err != nil {
			fmt.Println("⚠️ MQTT 发布失败:", topic, err)
		}
	}

	// 上线（retained online，覆盖遗嘱 offline）
	pub(availabilityTopic, "online")

	// 清理已删除目标的旧发现实体
	currentIDs := map[string]bool{}
	for _, t := range cfg.Targets {
		currentIDs[toID(t.IP)] = true
	}
	for id := range loadTargetIDs(targetStateFile) {
		if !currentIDs[id] {
			pub(fmt.Sprintf("%s/sensor/%s/config", discoveryPrefix, id), "")
		}
	}

	// 发布每个在场目标的 HA 发现配置（在线/离线二进制传感器）
	for _, t := range cfg.Targets {
		id := toID(t.IP)
		// 清理旧版实体类型（device_tracker / sensor）
		pub(fmt.Sprintf("%s/device_tracker/%s/config", discoveryPrefix, id), "")
		pub(fmt.Sprintf("%s/sensor/%s/config", discoveryPrefix, id), "")
		disc := targetDiscovery{
			Name:                   t.Name,
			UniqueID:               "lan_presence_" + id,
			StateTopic:             deviceStateTopic(id),
			PayloadOn:              "online",
			PayloadOff:             "offline",
			DeviceClass:            "connectivity",
			JSONAttributesTopic:    aggregateTopic,
			JSONAttributesTemplate: fmt.Sprintf("{{ value_json.devices['%s'] | tojson if value_json.devices is defined and value_json.devices['%s'] is defined else '{}' }}", id, id),
			AvailabilityTopic:      availabilityTopic,
			PayloadAvailable:       "online",
			PayloadNotAvailable:    "offline",
			Icon:                   "mdi:cellphone",
			Device:                 wanDevice(),
		}
		pub(fmt.Sprintf("%s/binary_sensor/%s/config", discoveryPrefix, id), mustMarshal(disc))
	}

	ids := make([]string, 0, len(currentIDs))
	for id := range currentIDs {
		ids = append(ids, id)
	}
	saveTargetIDs(targetStateFile, ids)

	// 清理旧版聚合实体
	pub(fmt.Sprintf("%s/sensor/lan_presence_all/config", discoveryPrefix), "")

	// WAN 传感器发现配置
	numericDisc := func(name, uniqueID, stateTopic, icon, unit, stateClass, deviceClass string) string {
		return mustMarshal(numericDiscovery{
			Name: name, UniqueID: uniqueID, StateTopic: stateTopic,
			AvailabilityTopic: availabilityTopic, PayloadAvailable: "online", PayloadNotAvailable: "offline",
			Icon: icon, UnitOfMeasurement: unit, StateClass: stateClass, DeviceClass: deviceClass,
			Device: wanDevice(),
		})
	}
	textDisc := func(name, uniqueID, stateTopic, icon string) string {
		return mustMarshal(textDiscovery{
			Name: name, UniqueID: uniqueID, StateTopic: stateTopic,
			AvailabilityTopic: availabilityTopic, PayloadAvailable: "online", PayloadNotAvailable: "offline",
			Icon: icon, Device: wanDevice(),
		})
	}

	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_download/config", discoveryPrefix, wanID),
		numericDisc("下载速率", "lan_presence_"+wanID+"_download", wanDownloadTopic, "mdi:download-network", "MB/s", "measurement", "data_rate"))
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_upload/config", discoveryPrefix, wanID),
		numericDisc("上传速率", "lan_presence_"+wanID+"_upload", wanUploadTopic, "mdi:upload-network", "MB/s", "measurement", "data_rate"))
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_conntrack/config", discoveryPrefix, wanID),
		numericDisc("连接数", "lan_presence_"+wanID+"_conntrack", wanConnTopic, "mdi:connection", "conn", "measurement", ""))

	// 旧版实体清理
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_uptime_seconds/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_uptime_human/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_rx_bytes_total/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_tx_bytes_total/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_rx_packets_total/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_tx_packets_total/config", discoveryPrefix, wanID), "")
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_ipv6/config", discoveryPrefix, wanID), "")

	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_rx_gb_total/config", discoveryPrefix, wanID),
		numericDisc("接收总流量", "lan_presence_"+wanID+"_rx_gb_total", wanRxTotalTopic, "mdi:database-arrow-down", "GB", "total_increasing", ""))
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_tx_gb_total/config", discoveryPrefix, wanID),
		numericDisc("发送总流量", "lan_presence_"+wanID+"_tx_gb_total", wanTxTotalTopic, "mdi:database-arrow-up", "GB", "total_increasing", ""))
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_ipv4/config", discoveryPrefix, wanID),
		textDisc("IPv4 地址", "lan_presence_"+wanID+"_ipv4", wanIpv4Topic, "mdi:ip-network-outline"))
	pub(fmt.Sprintf("%s/sensor/lan_presence_%s_ipv6_pd/config", discoveryPrefix, wanID),
		textDisc("IPv6-PD", "lan_presence_"+wanID+"_ipv6_pd", wanIpv6PdTopic, "mdi:ip-outline"))

	wanMon := newWanMonitor(cfg.WanInterface)
	if !wanMon.ok {
		fmt.Printf("⚠️ WAN 监控未启用：找不到接口 %s 的统计数据\n", cfg.WanInterface)
	}

	// 主循环状态
	lastStates := map[string]string{}
	hasPublishedAggregate := false
	lastRxGb, lastTxGb := int64(-1), int64(-1)
	lastIpv4, lastIpv6Pd := "", ""
	lastDownloadRate, lastUploadRate := "", ""
	var lastConnPublish, lastWanRatePublish time.Time

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		// 1. 在场检测
		devices := make(map[string]deviceState, len(cfg.Targets))
		onlineCount := 0
		anyChanged := false

		for _, t := range cfg.Targets {
			id := toID(t.IP)
			nud := neighborState(t.IP)
			state := "not_home"
			if isOnline(nud) {
				state = "home"
				onlineCount++
			}
			devices[id] = deviceState{
				Name:      t.Name,
				IP:        t.IP,
				Status:    state,
				Nud:       nud,
				Online:    state == "home",
				UpdatedAt: now.UTC().Format(time.RFC3339Nano),
			}
			if lastStates[id] != state {
				anyChanged = true
				fmt.Printf("%s %s (%s) -> %s (NUD=%s)\n", now.Format("15:04:05"), t.Name, t.IP, state, nud)
				lastStates[id] = state
				// 发布该目标独立的在线/离线状态（retained）
				if state == "home" {
					pub(deviceStateTopic(id), "online")
				} else {
					pub(deviceStateTopic(id), "offline")
				}
			}
		}

		if anyChanged || !hasPublishedAggregate {
			pub(aggregateTopic, mustMarshal(aggregatePayload{
				Timestamp:   now.UTC().Format(time.RFC3339Nano),
				OnlineCount: onlineCount,
				TotalCount:  len(cfg.Targets),
				Devices:     devices,
			}))
			hasPublishedAggregate = true
		}

		// 2. WAN 流量/速率/连接数
		if s, ok := wanMon.sample(); ok {
			downloadText := fmt.Sprintf("%.3f", s.DownloadBps/1_000_000)
			uploadText := fmt.Sprintf("%.3f", s.UploadBps/1_000_000)
			rxGb := int64(s.RxBytes / 1_000_000_000)
			txGb := int64(s.TxBytes / 1_000_000_000)

			if now.Sub(lastWanRatePublish) >= time.Duration(cfg.WanRateRefreshSeconds)*time.Second {
				if downloadText != lastDownloadRate {
					pub(wanDownloadTopic, downloadText)
					lastDownloadRate = downloadText
				}
				if uploadText != lastUploadRate {
					pub(wanUploadTopic, uploadText)
					lastUploadRate = uploadText
				}
				lastWanRatePublish = now
			}

			if now.Sub(lastConnPublish) >= connInterval {
				pub(wanConnTopic, strconv.FormatInt(s.Conntrack, 10))
				lastConnPublish = now
			}

			if rxGb != lastRxGb {
				pub(wanRxTotalTopic, strconv.FormatInt(rxGb, 10))
				lastRxGb = rxGb
			}
			if txGb != lastTxGb {
				pub(wanTxTotalTopic, strconv.FormatInt(txGb, 10))
				lastTxGb = txGb
			}
		}

		// 3. WAN 接口状态（IPv4 / IPv6-PD）
		if st := readUbusStatus(cfg.WanStatusInterface); st != nil && st.IPv4 != lastIpv4 {
			pub(wanIpv4Topic, st.IPv4)
			lastIpv4 = st.IPv4
		}
		if st6 := readUbusStatus(cfg.WanIpv6StatusInterface); st6 != nil {
			ipv6 := st6.IPv6PD
			if ipv6 != "" && !strings.EqualFold(ipv6, "unknown") && ipv6 != lastIpv6Pd {
				pub(wanIpv6PdTopic, ipv6)
				lastIpv6Pd = ipv6
			}
		}
	}
}
