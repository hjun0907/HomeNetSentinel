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

    -- 查询每个监控目标的实时在线/离线状态（基于邻居表 NUD）
    local online_states = {
        REACHABLE = true, STALE = true, DELAY = true, PROBE = true, PERMANENT = true
    }
    local targets = {}
    uci:foreach("homenet-sentinel", "target", function(section)
        local name = section.name or "?"
        local ip = section.ip
        if ip and ip ~= "" then
            local out = util.trim(util.exec("ip neigh show " .. ip .. " 2>/dev/null") or "")
            local nud = "NONE"
            if out ~= "" then
                local fields = {}
                for f in out:gmatch("%S+") do
                    fields[#fields + 1] = f
                end
                nud = fields[#fields] or "NONE"
            end
            targets[#targets + 1] = {
                name = name,
                ip = ip,
                nud = nud,
                online = online_states[nud] == true
            }
        end
    end)

    http.prepare_content("application/json")
    http.write_json({ running = running, targets = targets })
end
