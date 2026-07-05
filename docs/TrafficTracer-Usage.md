# TrafficTracer 分支使用指南

## 1. 编译产物

已编译版本位于:

```
/data/ytluo/projects/mihomo/bin/mihomo-linux-amd64
```

- 平台: linux/amd64 (x86_64, GOAMD64=v3, 支持 AVX2)
- 版本: 0897f41f
- 构建时间: 2026-07-05
- 编译标签: with_gvisor (内含 gVisor TUN 网络栈)

如需自行编译:

```bash
export PATH=$HOME/go-sdk/go/bin:$PATH
make linux-amd64          # amd64 v3 (需 CPU 支持 AVX2)
make linux-amd64-compatible  # amd64 v1 (兼容性更好)
```

---

## 2. 基本启动

```bash
./bin/mihomo-linux-amd64 -f /path/to/config.yaml

# 常用参数:
#   -f <file>      指定配置文件路径
#   -d <dir>       指定配置目录
#   -secret <key>  设置 API 认证密钥
#   -v             打印版本信息
```

---

## 3. TrafficTracer 新增功能：连接级流量追踪 (Connection Tracing)

TrafficTracer 分支的核心新增功能是**连接级流量追踪系统**，可以记录每一个 TCP/UDP 连接的生命周期事件，包括连接建立、代理选择和流量统计，输出为 JSON Lines 格式，方便与外部监控系统集成。

### 3.1 事件类型

| 事件类型 | 触发时机 | 适用协议 |
|----------|----------|----------|
| `tcp_connect` | TCP 连接到达，元数据解析完成后 | TCP |
| `tcp_proxy_dial` | 代理成功拨号到远端后 | TCP |
| `tcp_close` | TCP 连接关闭时 | TCP |
| `udp_out` | 每个 UDP 数据包发送到代理时 | UDP |
| `udp_in` | 每个 UDP 数据包从代理收到时 | UDP |

### 3.2 事件 JSON 格式

**tcp_connect**（连接建立）:
```json
{
    "ts":           "2026-07-05T12:00:00.123456789Z",
    "type":         "tcp_connect",
    "conn_id":      "a1b2c3d4-...",
    "src":          "192.168.1.100:54321",
    "dst":          "1.2.3.4:443",
    "host":         "example.com",
    "process":      "chrome",
    "process_path": "/usr/bin/chrome",
    "in_name":      "mixed"
}
```

字段说明:
- `conn_id`: 连接 UUID，唯一标识一次 TCP 连接，用于关联同一次连接的所有事件
- `src`: 客户端源地址 (IP:Port)
- `dst`: 目标地址 (IP:Port)
- `host`: 域名（可能为空，取决于是否走 DNS 解析）
- `process` / `process_path`: 发起连接的进程名和路径（需系统支持）
- `in_name`: 入站接口名称（如 mixed、tun、redir 等）

**tcp_proxy_dial**（代理拨号）:
```json
{
    "ts":           "2026-07-05T12:00:00.500000000Z",
    "type":         "tcp_proxy_dial",
    "conn_id":      "a1b2c3d4-...",
    "proxy":        "MyProxy",
    "proxy_type":   "ss",
    "proxy_addr":   "5.6.7.8:8388",
    "out_src":      "10.0.0.1:12345"
}
```

字段说明:
- `conn_id`: 与 tcp_connect 相同的 UUID
- `proxy`: 代理节点名称
- `proxy_type`: 代理类型（ss/vmess/trojan/snell/hysteria 等）
- `proxy_addr`: 代理服务器地址
- `out_src`: 本地出站地址

**tcp_close**（连接关闭）:
```json
{
    "ts":           "2026-07-05T12:01:00.000000000Z",
    "type":         "tcp_close",
    "conn_id":      "a1b2c3d4-...",
    "bytes_up":     1024000,
    "bytes_down":   5120000,
    "duration_ms":  60000
}
```

字段说明:
- `conn_id`: 与 tcp_connect 相同的 UUID
- `bytes_up`: 总上传字节数
- `bytes_down`: 总下载字节数
- `duration_ms`: 连接持续时间（毫秒）

**udp_out**（UDP 上行）:
```json
{
    "ts":           "2026-07-05T12:00:00.100000000Z",
    "type":         "udp_out",
    "conn_key":     "192.168.1.100:54321->8.8.8.8:53",
    "seq":          1,
    "src":          "192.168.1.100:54321",
    "dst":          "8.8.8.8:53",
    "len":          128,
    "proxy":        "MyProxy"
}
```

**udp_in**（UDP 下行）:
```json
{
    "ts":           "2026-07-05T12:00:00.200000000Z",
    "type":         "udp_in",
    "conn_key":     "192.168.1.100:54321->8.8.8.8:53",
    "seq":          1,
    "from":         "5.6.7.8:8388",
    "len":          256
}
```

字段说明:
- `conn_key`: NAT 会话键（格式 `src->dst`），用于关联同一 UDP 会话的所有数据包
- `seq`: 序列号（上行和下行独立编号，从 1 开始）
- `len`: 数据包长度

### 3.3 启用追踪

**方式一：通过配置文件**

在 `config.yaml` 的 `experimental` 节中添加:

```yaml
experimental:
  tracing: true
```

**方式二：通过 REST API（推荐，支持运行时动态开关和切换输出）**

查询当前状态:
```bash
curl http://localhost:9090/experimental/tracing
# 返回: {"enabled":false,"output":""}
```

启用追踪并输出到 stdout:
```bash
curl -X PATCH http://localhost:9090/experimental/tracing \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true, "output": ""}'
```

启用追踪并输出到文件:
```bash
curl -X PATCH http://localhost:9090/experimental/tracing \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true, "output": "/var/log/mihomo/trace.jsonl"}'
```

关闭追踪:
```bash
curl -X PATCH http://localhost:9090/experimental/tracing \
  -H 'Content-Type: application/json' \
  -d '{"enabled": false}'
```

**注意**: 配置文件只能控制 `enabled` 开关，无法通过配置设置 output 路径。推荐使用 API 方式同时设置 `enabled` 和 `output`。

### 3.4 使用示例

**输出到 stdout 并在终端查看**:
```bash
# 终端1: 启动 mihomo，追踪输出到 stdout
./bin/mihomo-linux-amd64 -f config.yaml 2>/dev/null &
# 启用追踪
curl -X PATCH http://localhost:9090/experimental/tracing \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true, "output": ""}'
# 追踪事件会实时输出到 stdout
```

**输出到文件并使用 jq 分析**:
```bash
# 启用文件输出
curl -X PATCH http://localhost:9090/experimental/tracing \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true, "output": "/tmp/trace.jsonl"}'

# 分析数据 - 统计各代理流量
cat /tmp/trace.jsonl | jq -r 'select(.type=="tcp_close") | [.conn_id, .bytes_up, .bytes_down, .duration_ms] | @tsv'

# 分析数据 - 查看某连接完整生命周期
cat /tmp/trace.jsonl | jq -s 'group_by(.conn_id) | .[] | select(.[0].type=="tcp_connect")'

# 分析数据 - 统计 UDP 流量按 destination 聚合
cat /tmp/trace.jsonl | jq -r 'select(.type=="udp_out") | [.conn_key, .len] | @tsv'
```

### 3.5 注意事项

1. **性能影响**: 追踪系统在 disabled 状态使用 atomic 操作做快速路径检查，开销可忽略。启用后每个事件触发一次 JSON 序列化 + write + flush，高并发时可能成为瓶颈。

2. **UDP 日志量**: UDP 追踪是每个数据包一个事件，高吞吐量场景（DNS、视频、游戏）会产生大量日志，务必输出到文件而非 stdout。

3. **文件轮转**: 追踪系统本身不提供日志轮转功能。如果输出到文件，建议配合外部日志轮转工具（如 logrotate）。

4. **线程安全**: 所有追踪写入通过单一 mutex 序列化，确保 JSON Lines 输出不会被交错破坏。

---

## 4. 流量匹配相关 REST API

TrafficTracer 分支除了新增追踪功能外，还提供了完整的 REST API 用于流量监控和匹配。以下是流量匹配相关的核心接口。

### 4.1 连接查询 (GET /connections)

获取当前所有活跃连接的快照信息:

```bash
curl http://localhost:9090/connections
```

返回示例:
```json
{
    "downloadTotal": 5242880,
    "uploadTotal": 1048576,
    "connections": [
        {
            "id": "a1b2c3d4-...",
            "metadata": {
                "network": "tcp",
                "type": "http",
                "srcIP": "192.168.1.100",
                "srcPort": "54321",
                "dstIP": "1.2.3.4",
                "dstPort": "443",
                "host": "example.com",
                "dnsMode": "normal",
                "process": "chrome",
                "processPath": "/usr/bin/chrome"
            },
            "upload": 102400,
            "download": 512000,
            "start": "2026-07-05T12:00:00Z",
            "chains": ["MyProxy"],
            "rule": "MATCH",
            "rulePayload": ""
        }
    ]
}
```

**WebSocket 推送**（实时连接信息）:

```bash
# 使用 websocat 等工具连接
websocat ws://localhost:9090/connections?interval=1000
```

### 4.2 关闭连接 (DELETE /connections/{id})

```bash
# 关闭单个连接
curl -X DELETE http://localhost:9090/connections/a1b2c3d4-...

# 关闭所有连接
curl -X DELETE http://localhost:9090/connections
```

### 4.3 规则查询 (GET /rules)

查看当前所有路由规则及其匹配次数:

```bash
curl http://localhost:9090/rules
```

返回示例:
```json
{
    "rules": [
        {
            "type": "DOMAIN-SUFFIX",
            "payload": "google.com",
            "proxy": "MyProxy",
            "size": -1
        },
        {
            "type": "GEOIP",
            "payload": "CN",
            "proxy": "DIRECT",
            "size": 0
        },
        {
            "type": "MATCH",
            "payload": "",
            "proxy": "MyProxy",
            "size": -1
        }
    ]
}
```

- `size` 字段对 GEOIP/GEOSITE 规则显示匹配次数，其他规则为 -1

### 4.4 流量统计 (GET /traffic)

获取全局流量统计:

```bash
curl http://localhost:9090/traffic
```

返回:
```json
{
    "up": 1048576,
    "down": 5242880
}
```

### 4.5 代理节点查询 (GET /proxies)

查看所有代理节点的实时延迟和状态:

```bash
curl http://localhost:9090/proxies
```

---

## 5. 典型使用场景

### 场景一：调试代理路由

```
启动追踪 -> 访问目标网站 -> 查看 JSONL 输出中某连接的 tcp_connect + tcp_proxy_dial 事件，
确认流量走的代理是否正确，以及代理类型和地址
```

### 场景二：流量统计与分析

```
启用追踪输出到文件 -> 运行一段时间 -> 使用 jq 聚合 tcp_close 事件，
按时间窗口统计各代理的流量总量和平均时长
```

### 场景三：连接生命周期监控

```
通过 WebSocket 连接 /connections 获取实时连接列表，
配合 /rules 查看规则配置，结合追踪 JSONL 输出进行离线分析
```

### 场景四：UDP 流量排查

```
启用追踪 -> 执行需要排查的 UDP 操作 -> 查看 JSONL 中各 udp_out/udp_in 事件，
确认 UDP 数据包的流向、大小和序列号
```
