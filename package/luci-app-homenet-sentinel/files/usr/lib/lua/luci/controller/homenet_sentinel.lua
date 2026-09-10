module("luci.controller.homenet_sentinel", package.seeall)

local STATE_FILE = "/tmp/homenet-sentinel/state.json"

function index()
    if not nixio.fs.access("/etc/config/homenet-sentinel") then
        return
    end

    entry({"admin", "services", "homenet-sentinel"}, cbi("homenet_sentinel"), _("家庭网络哨兵"), 60)
    entry({"admin", "services", "homenet-sentinel", "status"}, call("action_status")).leaf = true
end

-- 采集所有无线接口关联终端的信号强度，返回 { [MAC小写] = 信号dBm }
-- 任何异常都不能让 status 接口 500（否则前端表格永远停在"检测中..."）
local function collect_wifi_signals(util)
    local signals = {}

    local ok, iwinfo = pcall(require, "iwinfo")
    if ok and iwinfo then
        -- 枚举 /sys/class/net 下的无线接口（存在 wireless 子目录）
        local ifs = util.exec("ls -1 /sys/class/net 2>/dev/null") or ""
        for ifname in ifs:gmatch("%S+") do
            if nixio.fs.access("/sys/class/net/" .. ifname .. "/wireless") then
                -- iwinfo.type 对异常接口可能抛错，必须 pcall
                local backend
                pcall(function() backend = iwinfo.type and iwinfo.type(ifname) end)
                local iw = backend and iwinfo[backend]
                local ok_list, list = pcall(function() return iw and iw.assoclist(ifname) end)
                if ok_list and type(list) == "table" then
                    for m, info in pairs(list) do
                        if type(info) == "table" and info.signal and info.signal ~= 0 then
                            signals[m:lower()] = info.signal
                        end
                    end
                end
            end
        end
        return signals
    end

    -- 回退：解析 iwinfo 命令行输出
    local out = util.exec("iwinfo 2>/dev/null") or ""
    for ifname in out:gmatch("(%w+)%s+ESSID") do
        local a = util.exec("iwinfo " .. ifname .. " assoclist 2>/dev/null") or ""
        for mac, sig in a:gmatch("(%x%x:%x%x:%x%x:%x%x:%x%x:%x%x)%s+(-?%d+)%s*dBm") do
            signals[mac:lower()] = tonumber(sig)
        end
    end
    return signals
end

-- 读取守护进程写出的防抖状态文件，返回 { [IP] = { online=, mac=, nud= } }
local function read_daemon_state(util)
    local states = {}
    -- 注意：luci.util 在部分 LuCI 版本没有 readfile，必须用 nixio.fs.readfile
    local raw = nixio.fs.readfile(STATE_FILE)
    if not raw or raw == "" then
        return states
    end
    local ok, jsonc = pcall(require, "luci.jsonc")
    if not ok then
        return states
    end
    local parsed = jsonc.parse(raw)
    if type(parsed) ~= "table" or type(parsed.targets) ~= "table" then
        return states
    end
    for _, t in ipairs(parsed.targets) do
        if type(t) == "table" and t.ip then
            states[t.ip] = {
                online = t.online == true,
                mac = t.mac,
                nud = t.nud
            }
        end
    end
    return states
end

function action_status()
    local http = require "luci.http"
    local sys = require "luci.sys"
    local util = require "luci.util"
    local uci = require "luci.model.uci".cursor()

    local running = (sys.call("/etc/init.d/homenet-sentinel status >/dev/null 2>&1") == 0)

    -- 在线/离线/MAC 一律以守护进程的防抖状态文件为准（守护进程独占 ARP 探测，
    -- 本接口只读不探测，避免页面轮询删邻居条目与守护进程互相干扰导致状态抖动）
    local ok_d, daemon = pcall(read_daemon_state, util)
    if not ok_d or type(daemon) ~= "table" then daemon = {} end
    local ok_s, signals = pcall(collect_wifi_signals, util)
    if not ok_s or type(signals) ~= "table" then signals = {} end

    local targets = {}
    local idx = 0
    uci:foreach("homenet-sentinel", "target", function(section)
        local ip = section.ip
        if ip and ip ~= "" then
            local st = daemon[ip]
            local online, mac, nud
            if st then
                -- 守护进程在运行：用防抖后的结论
                online = st.online
                mac = st.mac
                nud = st.nud or "UNKNOWN"
                -- MAC 兜底：只读邻居表补 lladdr（不做任何探测/删除操作）
                if not mac or mac == "" then
                    local nout = util.trim(util.exec("ip neigh show " .. ip .. " 2>/dev/null") or "")
                    if nout ~= "" then
                        for w in nout:gmatch("%S+") do
                            if w:match("^%x%x:%x%x:%x%x:%x%x:%x%x:%x%x$") then mac = w end
                        end
                    end
                end
            else
                -- 守护进程未运行/状态文件过期：只读邻居表作为降级展示（不做任何探测）
                local out = util.trim(util.exec("ip neigh show " .. ip .. " 2>/dev/null") or "")
                nud = "NONE"
                if out ~= "" then
                    local fields = {}
                    for f in out:gmatch("%S+") do
                        fields[#fields + 1] = f
                    end
                    nud = fields[#fields] or "NONE"
                    for i = 1, #fields - 1 do
                        if fields[i] == "lladdr" then mac = fields[i + 1] end
                    end
                end
                online = (nud == "REACHABLE" or nud == "PERMANENT")
            end

            local sig = mac and signals[mac:lower()] or nil

            targets[#targets + 1] = {
                idx = idx,
                name = section.name or "?",
                ip = ip,
                mac = mac,
                nud = nud,
                signal = sig,
                online = online
            }
            idx = idx + 1
        end
    end)

    http.prepare_content("application/json")
    http.write_json({ running = running, targets = targets })
end
