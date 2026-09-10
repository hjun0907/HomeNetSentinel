# HomeNet Sentinel

HomeNet Sentinel 是一个运行在 OpenWrt 上的家庭网络监控服务，支持：

- 基于 ARP/邻居表的在家检测（可配置多个目标 IP）
- 广域网流量与连接数采集（例如 `pppoe-wan`）
- 通过 MQTT 自动发现（Home Assistant MQTT Discovery）发布传感器
- 提供 LuCI 页面进行可视化配置

项目已包含 `luci-app-homenet-sentinel` 打包结构，可直接构建 `ipk` 安装包。

---

## 效果图

### Home Assistant 传感器样式

![Home Assistant 传感器样式](img/ha.png)

### OpenWrt / LuCI 页面效果

![OpenWrt 页面效果](img/luci-app-homenet-sentinel.png)

---

## 功能概览

### 1) 在家检测

- 支持多个目标（名称 + IP）
- 状态：`home` / `not_home`
- 结果发布到聚合主题，并自动注册为 HA 传感器

### 2) 广域网监控

- 下载速率（MB/s）
- 上传速率（MB/s）
- 连接数（conn，默认 30 秒发布一次）
- 接收总流量（GB，整数）
- 发送总流量（GB，整数）
- IPv4 地址
- IPv6-PD（来自 `wan_6` 的 `ipv6-prefix`）

### 3) MQTT / HA 集成

- 使用 MQTT retained 消息
- 自动注册 Home Assistant 实体

---


## 配置方式

通过 LuCI：`服务 -> 家庭网络哨兵`

页面顶部会显示服务运行状态（运行中/未运行）。

如勾选“启用服务”后未立即生效，可手动执行：

```sh
/etc/init.d/homenet-sentinel start
```


说明：

- 接口与刷新项有默认值：
  - `wan_interface=pppoe-wan`
  - `wan_status_interface=wan`
  - `wan_ipv6_status_interface=wan_6`
  - `wan_rate_refresh_interval_seconds=3`（上传/下载刷新延迟）
- 连接数发送间隔固定为 30 秒，其他数据有变动即刷新
- MQTT 服务器需手动配置
- 在家检测目标可选；不配置目标时，服务仍可运行并仅发布 WAN 监控相关数据

---

## 打包 IPK（ipkg-build）

### 1) 先编译二进制

打包前请先编译：

```sh
./build-aot.sh \
  HomeNetSentinel \
  HomeNetSentinel.csproj \
  linux-musl-x64 \
  $HOME/.build/HomeNetSentinel
```

默认产物路径：

- `$HOME/.build/HomeNetSentinel/out-musl/HomeNetSentinel`

如需使用预置路径，也可以将可执行文件放到：

- `package/luci-app-homenet-sentinel/prebuilt/HomeNetSentinel`

或在打包时通过 `--bin` 指定任意路径。

### 2) 执行打包

示例

```sh
./build_ipk.sh \
  --bin "$HOME/.build/HomeNetSentinel/out-musl/HomeNetSentinel" \
  --arch x86_64 \
  --version 1.0.0-0 \
  --output "$HOME/.build/HomeNetSentinel/" \
  --ipkg-build "$HOME/immortalwrt-sdk/immortalwrt-sdk-24.10.2-x86-64_gcc-13.3.0_musl.Linux-x86_64/scripts/ipkg-build"
```

### 3) 常用参数

- `--bin <path>`：指定 HomeNetSentinel 二进制
- `--arch <arch>`：包架构（如 `x86_64`）
- `--version <ver-rel>`：版本（如 `1.0.0-0`）
- `--output <dir>`：输出目录
- `--ipkg-build <path>`：指定 `ipkg-build` 路径

---

## 手动运行可执行文件

```sh
HNS_BROKER_HOST="192.168.5.1" \
HNS_BROKER_PORT="1883" \
HNS_MQTT_USERNAME="admin" \
HNS_MQTT_PASSWORD="your_password" \
HNS_TARGETS="我的手机|192.168.5.16;家人手机|192.168.5.20" \
HNS_WAN_INTERFACE="pppoe-wan" \
HNS_WAN_STATUS_INTERFACE="wan" \
HNS_WAN_IPV6_STATUS_INTERFACE="wan_6" \
HNS_WAN_RATE_REFRESH_INTERVAL_SECONDS="3" \
./HomeNetSentinel
```

---


## MQTT 主题（主要）

- 聚合在家状态：`lan/presence/all`
- 可用性：`lan/presence/bridge/status`
- WAN 下载：`lan/presence/wan/<wan_if_id>/download_bps`
- WAN 上传：`lan/presence/wan/<wan_if_id>/upload_bps`
- WAN 连接数：`lan/presence/wan/<wan_if_id>/conntrack_count`
- WAN 接收总流量（GB）：`lan/presence/wan/<wan_if_id>/rx_gb_total`
- WAN 发送总流量（GB）：`lan/presence/wan/<wan_if_id>/tx_gb_total`
- WAN IPv4：`lan/presence/wan/<wan_if_id>/ipv4`
- WAN IPv6-PD：`lan/presence/wan/<wan_if_id>/ipv6_pd`

`<wan_if_id>` 由接口名转换（例如 `pppoe-wan`）。

---

## 故障排查

### 1) 安装报架构不兼容

检查目标设备架构：

```sh
opkg print-architecture
```

打包时 `--arch` 需与设备兼容。

### 2) HA 里仍显示旧目标

- 已删除目标可能是旧 retained discovery 残留
- 重启服务后新版本会自动清理；若仍残留，重载 MQTT 集成或重启 HA

### 3) 服务未启动

```sh
/etc/init.d/homenet-sentinel status
logread | grep homenet-sentinel
```

也可以在 OpenWrt 后台日志页面查看：`状态 -> 系统日志`（`/cgi-bin/luci/admin/status/log`）。

### 4) openwrt 25.12.4定制apk文件
rm -rf /tmp/ex && mkdir /tmp/ex
apk --allow-untrusted extract --destination /tmp/ex /tmp/luci.apk
cp -f /tmp/ex/usr/bin/HomeNet-Sentinel /usr/bin/HomeNet-Sentinel
chmod 755 /usr/bin/HomeNet-Sentinel

# 验证新二进制包含 HNS_BROKER
strings /usr/bin/HomeNet-Sentinel | grep -i HNS_BROKER

# 重启服务
/etc/init.d/homenet-sentinel restart
sleep 2
logread | tail -5 | grep -i homenet

### 5) fanchmwrt 覆盖安装完整步骤（解决 about.htm 缺失 / LuCI 500）

fanchmwrt 因包源与官方 snapshot 冲突无法用 `apk add` 正常安装，须用 `apk extract` 解包后手动覆盖。
注意：必须拷贝 view 目录下**所有** `.htm`（包含 `about.htm`），否则 LuCI 渲染 `cbi/map` 时找不到模板会 500。
`model/cbi` 下的 `.lua` 也要一并覆盖，否则 CBI 配置页无法加载。

```sh
# 1. 解包 apk（假设已将 luci.apk 放到 /tmp/）
rm -rf /tmp/ex && mkdir /tmp/ex
apk --allow-untrusted extract --destination /tmp/ex /tmp/luci.apk

# 2. 覆盖二进制
cp -f /tmp/ex/usr/bin/HomeNet-Sentinel /usr/bin/HomeNet-Sentinel
chmod 755 /usr/bin/HomeNet-Sentinel

# 3. 覆盖 LuCI 控制器、CBI 模型、视图模板（*.htm 通配符，含 about.htm 与 status.htm）
cp -f /tmp/ex/usr/lib/lua/luci/controller/homenet_sentinel.lua /usr/lib/lua/luci/controller/
cp -f /tmp/ex/usr/lib/lua/luci/model/cbi/homenet_sentinel.lua /usr/lib/lua/luci/model/cbi/
mkdir -p /usr/lib/lua/luci/view/homenet_sentinel
cp -f /tmp/ex/usr/lib/lua/luci/view/homenet_sentinel/*.htm /usr/lib/lua/luci/view/homenet_sentinel/

# 4. 验证 about.htm 已就位
ls -la /usr/lib/lua/luci/view/homenet_sentinel/

# 5. 重启服务并清 LuCI 模板缓存
/etc/init.d/homenet-sentinel restart
rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
```

强制刷新浏览器（Ctrl+Shift+R）后即可看到三个 Tab 与"关于"页面。

---
## 记录
2026-09-09 
这次排查跨越了三层问题：

CI 打包失败 → APK 版本号不能是 master、~ 非法，改为 0.0.0；SDK 容器需 setup.sh 引导、feed 指向包父目录、产物挂载 /builder/bin；
Release 多出一堆 apk → 收集步骤过滤，只取 luci-app-homenet-sentinel*.apk，并加了 apk 解包自检；
路由器装不上/连不上 MQTT → ① snapshot 内核漂移 + fanchmwrt 坏 world 导致 apk 求解失败，用 apk extract 绕过；② 根因是本地 4 个 commit（含 getHNS 环境变量修复）没推送到 GitHub，CI 一直用旧代码编译，旧二进制读不到 HNS_BROKER_HOST 兜底到 127.0.0.1。
覆盖安装并清 LuCI 缓存即可看到新界面：
rm -rf /tmp/ex && mkdir /tmp/ex
apk --allow-untrusted extract --destination /tmp/ex /tmp/luci.apk
cp -f /tmp/ex/usr/bin/HomeNet-Sentinel /usr/bin/
cp -f /tmp/ex/usr/lib/lua/luci/controller/homenet_sentinel.lua /usr/lib/lua/luci/controller/
cp -f /tmp/ex/usr/lib/lua/luci/view/homenet_sentinel/status.htm /usr/lib/lua/luci/view/homenet_sentinel/
chmod 755 /usr/bin/HomeNet-Sentinel
/etc/init.d/homenet-sentinel restart
rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
## 许可

本项目采用 `MIT` 许可证，详见 `LICENSE`。
