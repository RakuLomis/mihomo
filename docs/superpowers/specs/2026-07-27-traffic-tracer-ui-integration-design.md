# TrafficTracer UI 控制集成 — 设计文档

**日期**: 2026-07-27  
**范围**: 在 clash-verge-rev 中集成 TrafficTracer 的启用/禁用及输出路径控制  
**决策**: 方案 A — Rust Tauri Command + reqwest 直连 mihomo REST API

---

## 1. 背景

TrafficTracer 是 mihomo 的一个功能分支，提供连接级流量追踪。它通过 2 个 REST API 端点暴露控制能力：

```
GET  /experimental/tracing   → { enabled: bool, output: string }
PATCH /experimental/tracing  → { enabled?: bool, output?: string }
```

clash-verge-rev 是 mihomo 的桌面 GUI 壳（Tauri v2 + React + MUI），已实现核心启停、TUN 开关、配置读取和节点选择。本设计目标是在 clash-verge-rev 的 Settings > Clash 设置区块中，增加 TrafficTracer 的 toggle 开关和输出路径输入框。

## 2. 架构总览

```
┌──────────────────────────────────────────────────┐
│ 前端 (React / MUI)                                │
│  setting-clash.tsx                                │
│    ├─ Switch (enabled toggle)                     │
│    └─ TextField (output path)                     │
│         │                                         │
│  use-tracing.ts (React Query hook)                │
│         │ invoke("get_tracing_state")              │
│         │ invoke("patch_tracing_state", {payload}) │
├─────────┼─────────────────────────────────────────┤
│  Tauri IPC                                         │
├─────────┼─────────────────────────────────────────┤
│  Rust: cmd/tracing.rs                              │
│    ├─ get_tracing_state()    #[tauri::command]     │
│    └─ patch_tracing_state()  #[tauri::command]     │
│         │                                         │
│    reqwest::Client                                │
│         │ HTTP GET/PATCH                          │
│         ▼                                         │
│  http://127.0.0.1:{port}/experimental/tracing     │
│  Authorization: Bearer {secret}                   │
├───────────────────────────────────────────────────┤
│  mihomo 内核 (TrafficTracer 分支)                  │
│  component/tracer/tracer.go                       │
└───────────────────────────────────────────────────┘
```

## 3. Rust 后端

### 3.1 新增文件 `src-tauri/src/cmd/tracing.rs`

```rust
// 类型定义
#[derive(Debug, Serialize, Deserialize)]
struct TracingState {
    enabled: bool,
    output: String,
}

#[derive(Debug, Deserialize)]
struct TracingPatch {
    enabled: Option<bool>,
    output: Option<String>,
}

// Tauri command: 查询当前追踪状态
#[tauri::command]
pub async fn get_tracing_state() -> CmdResult<TracingState> {
    // 1. 从 Config::clash() 获取端口和 secret
    // 2. 构造 URL: http://127.0.0.1:{port}/experimental/tracing
    // 3. reqwest GET 请求，带 Authorization: Bearer {secret}
    // 4. 解析 JSON 返回 TracingState
    // 5. 错误处理: stringify_err
}

// Tauri command: 修改追踪状态
#[tauri::command]
pub async fn patch_tracing_state(payload: TracingPatch) -> CmdResult<TracingState> {
    // 1. 同上前置逻辑
    // 2. reqwest PATCH 请求，body 为 serde_json::to_value(payload)
    // 3. 返回更新后的 TracingState
}
```

**关键实现细节**：
- 端口来源：`Config::clash().await.data_arc().get_mixed_port()`
- secret 来源：`Config::clash().await.data_arc().get_secret()`
- 使用 `reqwest`（已在 `Cargo.toml` 中声明）发起 HTTPS 直连
- 错误统一使用 `CmdResult` + `stringify_err` 模式（与现有 commands 一致）

### 3.2 修改文件 `src-tauri/src/lib.rs`

在 `generate_handlers()` 中注册 2 个新 command（约第 161 行 `cmd::patch_clash_config` 旁）:

```rust
cmd::get_tracing_state,
cmd::patch_tracing_state,
```

### 3.3 模块声明

在 `src-tauri/src/cmd/mod.rs` 中追加:

```rust
pub mod tracing;
pub use tracing::*;
```

## 4. 前端

### 4.1 `src/services/cmds.ts` — 新增 API 函数

```typescript
// 约第 586 行（文件末尾附近）
export async function getTracingState() {
  return invoke<ITracingState>("get_tracing_state")
}

export async function patchTracingState(payload: ITracingPatch) {
  return invoke<ITracingState>("patch_tracing_state", { payload })
}
```

### 4.2 `src/hooks/use-tracing.ts` — 新增 React Query hook

```typescript
import { useQuery } from "@tanstack/react-query"
import { getTracingState, patchTracingState } from "@/services/cmds"

export const useTracing = () => {
  const { data: tracing, refetch: mutateTracing } = useQuery({
    queryKey: ["getTracingState"],
    queryFn: getTracingState,
  })

  const patchTracing = async (payload: ITracingPatch) => {
    await patchTracingState(payload)
    await mutateTracing()
  }

  return { tracing, mutateTracing, patchTracing }
}
```

### 4.3 `src/components/setting/setting-clash.tsx` — 新增 UI

在现有 `SettingList` 末尾（第 284 行 `TunnelsViewer` 之后、`</SettingList>` 之前）追加：

```tsx
// 新增 import
import { useTracing } from "@/hooks/use-tracing"

// 组件内新增 hook 调用
const { tracing, patchTracing } = useTracing()

// UI: TrafficTracer 启用开关
<SettingItem label={t("settings.sections.clash.form.fields.tracing")}>
  <GuardState
    value={tracing?.enabled ?? false}
    valueProps="checked"
    onFormat={(_, v: boolean) => v}
    onChange={(v) => {} /* optimistic update */ }
    onGuard={(v) => patchTracing({ enabled: v })}
  >
    <Switch edge="end" />
  </GuardState>
</SettingItem>

// UI: 输出路径（仅在启用时显示/可编辑）
{tracing?.enabled && (
  <SettingItem
    label={t("settings.sections.clash.form.fields.tracingOutput")}
    extra={
      <TooltipIcon
        title={t("settings.sections.clash.form.tooltips.tracingOutput")}
        sx={{ opacity: "0.7" }}
      />
    }
  >
    <TextField
      autoComplete="new-password"
      size="small"
      value={tracing?.output ?? ""}
      placeholder="stdout"
      sx={{ width: 250, input: { py: "7.5px" } }}
      onBlur={(e) => {
        if (e.target.value !== tracing?.output) {
          patchTracing({ output: e.target.value || "" })
        }
      }}
    />
  </SettingItem>
)}
```

### 4.4 TypeScript 类型定义

追加到现有类型声明文件（`src/types/` 或内联于 hooks）：

```typescript
interface ITracingState {
  enabled: boolean
  output: string
}

interface ITracingPatch {
  enabled?: boolean
  output?: string
}
```

### 4.5 i18n 翻译键

在 `locales/{en,zh,...}.json` 的 `settings.sections.clash.form.fields` 和 `tooltips` 下追加：

| key | en | zh |
|-----|----|----|
| `fields.tracing` | Traffic Tracer | 流量追踪 |
| `fields.tracingOutput` | Output Path | 输出路径 |
| `tooltips.tracingOutput` | Empty = stdout | 留空即输出到标准输出 |

## 5. 数据流（端到端）

```
用户点击 Switch ON
  → GuardState.onChange({ enabled: true })
  → GuardState.onGuard → patchTracing({ enabled: true })
  → patchTracingState({ enabled: true })  [TS invoke]
  → Rust: patch_tracing_state(payload)
  → reqwest PATCH http://127.0.0.1:{port}/experimental/tracing
     Body: {"enabled":true}
     Header: Authorization: Bearer {secret}
  → mihomo: tracer.SetEnabled(true)
  → 返回 {"enabled":true, "output":""}
  → Rust 返回 TracingState 给前端
  → mutateTracing() 刷新 React Query 缓存
  → UI 更新：TextField 显示（因 enabled=true）
```

## 6. 错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| mihomo 核心未运行 | `CmdResult` 返回错误，前端 `notice-service` 展示通知 |
| 端口/secret 获取失败 | Rust 侧 logging error，返回 `Err` |
| PATCH 请求失败（4xx/5xx） | `reqwest::Error` → `stringify_err` → 前端通知 |
| 输出路径不可写 | mihomo 侧 `tracer.SetOutput` 返回错误，透传给前端 |

## 7. 测试要点

- **单元测试**：无需新增（Rust command 逻辑为薄封装层）
- **集成测试**：启动 mihomo + clash-verge-rev，验证 toggle 开关能正确调用 PATCH API
- **边界情况**：
  - 核心未启动时 toggle → 显示错误通知
  - output 为空字符串 → mihomo 输出到 stdout
  - output 设非法路径 → mihomo 返回错误
  - 快速反复 toggle → 无竞态（React Query 缓存 + ahooks useLockFn）

## 8. 改动文件清单

| 文件 | 操作 | 估计行数 |
|------|------|----------|
| `src-tauri/src/cmd/tracing.rs` | 新增 | ~50 |
| `src-tauri/src/cmd/mod.rs` | 修改 (+1 行) | ~1 |
| `src-tauri/src/lib.rs` | 修改 (+2 行) | ~2 |
| `src/services/cmds.ts` | 修改 (+12 行) | ~12 |
| `src/hooks/use-tracing.ts` | 新增 | ~25 |
| `src/components/setting/setting-clash.tsx` | 修改 (+35 行) | ~35 |
| `src/types/*.d.ts` | 修改 (+10 行) | ~10 |
| `locales/{en,zh}.json` | 修改 (+6 行 × N 语言) | ~12 |

**总计：~150 行新增/修改代码，分布在 8 个文件中。**
