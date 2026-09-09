module("luci.controller.homenet_sentinel", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/homenet-sentinel") then
        return
    end

    entry({"admin", "services", "homenet-sentinel"}, cbi("homenet_sentinel"), _("家庭网络哨兵"), 60)
    entry({"admin", "services", "homenet-sentinel", "status"}, call("action_status")).leaf = true
end

-- 采集所有无线接口关联终端的信号强度，返回 { [MAC小写] = 信号dBm }
local function collect_wifi_signals(util)
    local signals = {}

    local ok, iwinfo = pcall(require, "iwinfo")
    if ok and iwinfo then
        -- 枚举 /sys/class/net 下的无线接口（存在 wireless 子目录）
        local ifs = util.exec("ls -1 /sys/class/net 2>/dev/null") or ""
        for ifname in ifs:gmatch("%S+") do
            if nixio.fs.access("/sys/class/net/" .. ifname .. "/wireless") then
                local backend = iwinfo.type and iwinfo.type(ifname)
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

function action_status()
    local http = require "luci.http"
    local sys = require "luci.sys"
    local util = require "luci.util"
    local uci = require "luci.model.uci".cursor()

    local running = (sys.call("/etc/init.d/homenet-sentinel status >/dev/null 2>&1") == 0)

    -- 主动探测：并发删除每个目标的旧邻居条目并 ping，强制内核发起广播 ARP。
    -- 在线设备会在 DTIM 周期内应答 ARP（REACHABLE）；离线设备无应答（FAILED/INCOMPLETE）。
    local target_list = {}
    uci:foreach("homenet-sentinel", "target", function(section)
        local ip = section.ip
        if ip and ip ~= "" then
            target_list[#target_list + 1] = { name = section.name or "?", ip = ip }
            sys.call('( ip neigh del ' .. ip .. ' dev br-lan 2>/dev/null; ping -c 1 -W 1 ' .. ip .. ' >/dev/null 2>&1 ) &')
        end
    end)

    if #target_list > 0 then
        util.exec("sleep 3")
    end

    local signals = collect_wifi_signals(util)

    local targets = {}
    for idx, t in ipairs(target_list) do
        local out = util.trim(util.exec("ip neigh show " .. t.ip .. " 2>/dev/null") or "")
        local nud, mac = "NONE", nil
        if out ~= "" then
            local fields = {}
            for f in out:gmatch("%S+") do
                fields[#fields + 1] = f
            end
            nud = fields[#fields] or "NONE"
            for i = 1, #fields - 1 do
                if fields[i] == "lladdr" then
                    mac = fields[i + 1]
                end
            end
        end

        local online = (nud == "REACHABLE" or nud == "PERMANENT")
        local sig = mac and signals[mac:lower()] or nil

        targets[#targets + 1] = {
            idx = idx - 1,
            name = t.name,
            ip = t.ip,
            mac = mac,
            nud = nud,
            signal = sig,
            online = online
        }
    end

    http.prepare_content("application/json")
    http.write_json({ running = running, targets = targets })
end
