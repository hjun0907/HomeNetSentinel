module("luci.controller.homenet_sentinel", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/homenet-sentinel") then
        return
    end

    entry({"admin", "services", "homenet-sentinel"}, cbi("homenet_sentinel"), _("家庭网络哨兵"), 60)
    entry({"admin", "services", "homenet-sentinel", "status"}, call("action_status")).leaf = true
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

    -- 仅 REACHABLE/PERMANENT 视为在线（STALE/DELAY/PROBE 为未确认状态，可能已离线）
    local online_states = { REACHABLE = true, PERMANENT = true }
    local targets = {}
    for _, t in ipairs(target_list) do
        local out = util.trim(util.exec("ip neigh show " .. t.ip .. " 2>/dev/null") or "")
        local nud = "NONE"
        if out ~= "" then
            local fields = {}
            for f in out:gmatch("%S+") do
                fields[#fields + 1] = f
            end
            nud = fields[#fields] or "NONE"
        end
        targets[#targets + 1] = {
            name = t.name,
            ip = t.ip,
            nud = nud,
            online = online_states[nud] == true
        }
    end

    http.prepare_content("application/json")
    http.write_json({ running = running, targets = targets })
end
