<h1 align="center">
  <img src="Meta.png" alt="Meta Kennel" width="200">
  <br>Meta Kernel<br>
</h1>

<h3 align="center">Another Mihomo Kernel.</h3>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/MetaCubeX/mihomo">
    <img src="https://goreportcard.com/badge/github.com/MetaCubeX/mihomo?style=flat-square">
  </a>
  <img src="https://img.shields.io/github/go-mod/go-version/MetaCubeX/mihomo/Alpha?style=flat-square">
  <a href="https://github.com/MetaCubeX/mihomo/releases">
    <img src="https://img.shields.io/github/release/MetaCubeX/mihomo/all.svg?style=flat-square">
  </a>
  <a href="https://github.com/MetaCubeX/mihomo">
    <img src="https://img.shields.io/badge/release-Meta-00b4f0?style=flat-square">
  </a>
</p>

## TrafficTracer Complete QuickStart

推荐入口是 `TrafficTracer` 总仓的 `Complete` 分支；它通过 submodule 和 `complete/components.lock.yaml` 固定本核心、TrafficTracer Worker 与 Clash Verge UI，不再要求手工对齐三个 sibling checkout：

```bash
git clone --branch Complete --recurse-submodules \
  git@github.com:RakuLomis/TrafficTracer.git
cd TrafficTracer
make bootstrap
make check-toolchain
make dev
```

在 UI 中导入代理配置，选择 `verge-mihomo-tt`，测速并选择节点，安装服务后开启 TUN，再进入“流量追踪”。可手工运行一个目标，也可加载目标 YAML 后全选或选择子集；批次严格按 YAML 顺序串行执行 capture → Chrome cleanup → analysis → checkpoint，失败或 Worker 中断后可从准确目标继续。每个子目标形成独立 Session，可直接查看连接级请求、代理前后五元组和 PCAP 索引。

Complete 自动按任务开启和恢复 tracing；不要寻找或使用设置侧栏中的独立 tracing 开关。Linux 未显式设置 TUN `device` 时，Mihomo 默认创建 `Meta`。自定义 Session root 必须是可写的绝对路径，切换 root 后重新执行环境检测。完整操作、打包和升级流程见总仓 `docs/complete/quickstart.md`。

## 历史三仓手工联动参考（不用于 Complete 构建）

以下流程仅保留给旧版独立 CLI 调试，并以三个仓库位于同一父目录为例。它包含已经从 Complete UI 删除的手工 tracing 开关，不能作为当前产品操作说明：

| 组件 | 分支 | 职责 |
|---|---|---|
| `mihomo` | `TrafficTracer` | 生成规范化的 `pre_flow` / `post_flow` JSONL 事件 |
| `clash-verge-rev` | `feat/traffic-tracer` | 导入配置、选择核心、测速/选节点、系统代理、TUN 和追踪 UI |
| `TrafficTracer` | `alpha` | 采集 Chrome/CDP/NetLog/pcap，关联代理前后五元组并查询结果 |

目前已验证的平台是 Linux x86-64。代码接口已经对齐，但运行时必须在 Clash Verge 中选择 `verge-mihomo-tt`；标准 `verge-mihomo` 不包含 `/experimental/tracing`，会返回 `404`。

### 1. 检出对应分支

```bash
export TT_WORKSPACE=/path/to/projects

git -C "$TT_WORKSPACE/mihomo" switch TrafficTracer
git -C "$TT_WORKSPACE/clash-verge-rev" switch feat/traffic-tracer
git -C "$TT_WORKSPACE/TrafficTracer" switch alpha
```

### 2. 构建 mihomo-traffictracer

```bash
cd "$TT_WORKSPACE/mihomo"
go mod download
make linux-amd64-compatible

./bin/mihomo-linux-amd64-compatible -v
```

这里生成的核心是 `bin/mihomo-linux-amd64-compatible`。如果仓库中已有确认过版本的 `bin/mihomo-traffictracer-v2`，也可以直接使用它。

### 3. 准备并启动 Clash Verge

先关闭系统中已安装的其他 Clash Verge 实例，避免两个实例争用服务进程和 `/tmp/verge/verge-mihomo.sock`。

```bash
cd "$TT_WORKSPACE/clash-verge-rev"
corepack enable
pnpm install

MIHOMO_TRAFFIC_TRACER_BIN="$TT_WORKSPACE/mihomo/bin/mihomo-linux-amd64-compatible" \
  pnpm prebuild --force

test -x src-tauri/sidecar/verge-mihomo-tt-x86_64-unknown-linux-gnu
pnpm dev
```

在 UI 中依次完成：

1. 打开“订阅/Profiles”，导入可用的 Mihomo YAML 配置。
2. 打开“设置 → Clash 设置 → Clash Core”，选择 `verge-mihomo-tt`，等待核心重启。
3. 打开“代理/Proxies”，运行节点延迟测试并选择目标节点或策略组。
4. 按需开启“系统代理”。需要捕获透明代理流量时，安装 Clash Verge 服务并开启 TUN。
5. 开启 TUN 前用 `ip route show default` 记录物理出口网卡；开启后用 `ip -brief link` 记录新出现的 TUN 网卡名。

验证当前运行的是 TrafficTracer 核心：

```bash
curl --unix-socket /tmp/verge/verge-mihomo.sock \
  http://localhost/experimental/tracing
```

成功时返回至少包含 `enabled` 的 JSON。返回 `404` 表示仍选择了标准核心，需要重新选择 `verge-mihomo-tt` 并重启。

### 4. 验证 UI 追踪开关

```bash
mkdir -p /tmp/mihomo-traffictracer
```

进入“设置 → Clash 设置”：

1. 开启“TrafficTracer/流量追踪”。
2. 将输出路径设置为绝对路径 `/tmp/mihomo-traffictracer/manual.jsonl`。
3. 访问一个网页，然后检查日志：

```bash
tail -n 20 /tmp/mihomo-traffictracer/manual.jsonl
```

父目录必须预先存在且对核心可写。空输出路径表示写到核心标准输出。完成手工验证后可以关闭 UI 追踪；TrafficTracer 采集程序会按访问任务临时开启追踪，并在结束或异常时恢复原状态和原输出路径。

### 5. 配置 TrafficTracer 采集器

```bash
cd "$TT_WORKSPACE/TrafficTracer"
python -m venv .venv
. .venv/bin/activate
pip install pyyaml websockets pytest

sudo setcap cap_net_raw,cap_net_admin=eip "$(command -v dumpcap)"
```

找到 Clash Verge 生成的核心配置。开发版通常位于：

```text
~/.local/share/io.github.clash-verge-rev.clash-verge-rev.dev/clash-verge.yaml
```

安装版通常位于：

```text
~/.local/share/io.github.clash-verge-rev.clash-verge-rev/clash-verge.yaml
```

也可以从正在运行的核心命令行中查看 `-f` 后面的实际路径：

```bash
ps -eo args | grep '[v]erge-mihomo-tt'
```

复制 `sites.clash-verge.example.yaml` 为 `sites.yaml`，然后填写真实绝对路径和网卡名：

```yaml
global:
  mihomo:
    managed: false
    binary: /absolute/path/to/mihomo/bin/mihomo-linux-amd64-compatible
    config: /absolute/path/to/clash-verge.yaml
    api: "unix:///tmp/verge/verge-mihomo.sock"
    secret: ""  # 空值时从上面的 Clash Verge 生成配置读取

  chrome:
    binary: google-chrome
    user_data_dir: /tmp/traffictracer-chrome
    headless: false
    enable_cdp: true

  network:
    tun_interface: Mihomo  # 替换为 ip -brief link 显示的实际 TUN 名
    phys_interface: wlp2s0 # 替换为开启 TUN 前的默认出口网卡

  output:
    base_dir: /absolute/path/to/traffictracer-output

sites:
  - domain: example.com
    url: https://example.com
    wait: 10
    traffic_type: all
```

联合模式必须使用 `managed: false`，因为核心生命周期由 Clash Verge 管理。`api` 显式指向 Unix Socket；`secret` 留空时 TrafficTracer 从 `config` 指定的生成配置读取。日志路径会在发送给外部核心前转换为绝对路径。

### 6. 采集与分析

保持 Clash Verge、目标代理节点和 TUN 运行：

```bash
cd "$TT_WORKSPACE/TrafficTracer"
. .venv/bin/activate

python capture.py --config sites.yaml --only example.com
```

命令会打印本次 session 目录。随后运行：

```bash
python analyze.py --session /absolute/path/to/traffictracer-output/YYYY-MM-DD_HH-MM-SS
```

主要产物包括：

```text
<session>/logs/mihomo_trace_<domain>_<run>.jsonl
<session>/logs/cdp_<domain>_<run>.json
<session>/logs/netlog_<domain>_<run>.json
<session>/captures/<domain>/<run>/tun.pcap
<session>/captures/<domain>/<run>/phys.pcap
<session>/results/correlation.json
```

`correlation.json` 同时保留兼容字段和规范化字段：`pre_flow`、`post_flow`、`match_status`、`match_confidence`、`conn_id`、`outer_conn_id`。完整规范化五元组使用：

```text
<tcp|udp>|<src_ip>:<src_port>|<dst_ip>:<dst_port>
```

IPv6 端点使用方括号，例如 `tcp|[2001:db8::1]:50000|[2001:db8::2]:443`。

### 7. 用代理前五元组查询代理后五元组

```bash
cd "$TT_WORKSPACE/TrafficTracer"
python query_flow.py \
  /absolute/path/to/mihomo_trace_example.com_all_1.jsonl \
  tcp \
  198.18.0.1 44000 \
  1.1.1.1 443
```

匹配成功时输出一个数组，其中 `pre_flow` 是代理前五元组，`post_flow` 是核心实际建立的代理后五元组。无匹配时输出 `[]` 并返回退出码 `1`。同一代理前五元组如果在不同时间被复用，结果会保留多个会话；`post_flow.shared: true` 表示复用的外层连接，不能解释为逻辑流独占的 NAT 映射。

### 8. 常见故障

- `mihomo returned 404`：当前是标准核心；切换到 `verge-mihomo-tt` 并重启。
- `No core binaries found`：重新执行带 `--force` 的 `pnpm prebuild`，确认目标 sidecar 存在且可执行。
- `IPC path not ready`：关闭重复 UI/残留核心，确认 service install/uninstall helper 与主程序同目录，然后在 UI 中重试安装或修复服务。
- TUN 无法开启：先安装/修复 Clash Verge 服务，并检查系统授权；仅系统代理模式不等同于 TUN 双接口采集。
- trace 文件不存在：使用绝对路径，先创建父目录，并确认核心进程对目录有写权限。
- TrafficTracer 无法连接控制器：确认 `/tmp/verge/verge-mihomo.sock` 存在、Clash Verge 核心正在运行，且 `sites.yaml` 的生成配置路径正确。
- `post_flow` 为空：连接可能尚未拨号成功、被拒绝/取消，或当前协议只能提供共享/不完整的外层端点；结合 `status`、`stage`、`error` 和 `shared` 判断。


## Features

- Local HTTP/HTTPS/SOCKS server with authentication support
- VMess, VLESS, Shadowsocks, Trojan, Snell, TUIC, Hysteria protocol support
- Built-in DNS server that aims to minimize DNS pollution attack impact, supports DoH/DoT upstream and fake IP.
- Rules based off domains, GEOIP, IPCIDR or Process to forward packets to different nodes
- Remote groups allow users to implement powerful rules. Supports automatic fallback, load balancing or auto select node
  based off latency
- Remote providers, allowing users to get node lists remotely instead of hard-coding in config
- Netfilter TCP redirecting. Deploy Mihomo on your Internet gateway with `iptables`.
- Comprehensive HTTP RESTful API controller

## Dashboard

A web dashboard with first-class support for this project has been created; it can be checked out at [metacubexd](https://github.com/MetaCubeX/metacubexd).

## Configration example

Configuration example is located at [/docs/config.yaml](https://github.com/MetaCubeX/mihomo/blob/Alpha/docs/config.yaml).

## Docs

Documentation can be found in [mihomo Docs](https://wiki.metacubex.one/).

## For development

Requirements:
[Go 1.20 or newer](https://go.dev/dl/)

Build mihomo:

```shell
git clone https://github.com/MetaCubeX/mihomo.git
cd mihomo && go mod download
go build
```

Set go proxy if a connection to GitHub is not possible:

```shell
go env -w GOPROXY=https://goproxy.io,direct
```

Build with gvisor tun stack:

```shell
go build -tags with_gvisor
```

### IPTABLES configuration

Work on Linux OS which supported `iptables`

```yaml
# Enable the TPROXY listener
tproxy-port: 9898

iptables:
  enable: true # default is false
  inbound-interface: eth0 # detect the inbound interface, default is 'lo'
```

## Debugging

Check [wiki](https://wiki.metacubex.one/api/#debug) to get an instruction on using debug
API.

## Credits

- [Dreamacro/clash](https://github.com/Dreamacro/clash)
- [SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- [riobard/go-shadowsocks2](https://github.com/riobard/go-shadowsocks2)
- [v2ray/v2ray-core](https://github.com/v2ray/v2ray-core)
- [WireGuard/wireguard-go](https://github.com/WireGuard/wireguard-go)
- [yaling888/clash-plus-pro](https://github.com/yaling888/clash)

## License

This software is released under the GPL-3.0 license.

**In addition, any downstream projects not affiliated with `MetaCubeX` shall not contain the word `mihomo` in their names.**
