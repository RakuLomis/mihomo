# TrafficTracer UI Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a TrafficTracer toggle + output path field to clash-verge-rev's Settings > Clash section, backed by a Rust Tauri command that calls mihomo's REST API at `/experimental/tracing`.

**Architecture:** A new Rust module `cmd/tracing.rs` wraps reqwest calls to mihomo's REST API, exposed as two Tauri commands. The frontend adds a React Query hook (`use-tracing.ts`) and UI controls (Switch + TextField) in the existing `setting-clash.tsx` component, following the same GuardState + invoke pattern used by all other settings.

**Tech Stack:** Rust 1.95.0, Tauri v2, React 19, MUI 9, @tanstack/react-query, pnpm 11, clash-verge-rev @ `feat/traffic-tracer`

## Global Constraints

- Rust toolchain: channel `1.95.0` from `rust-toolchain.toml`
- Frontend package manager: pnpm 11 (from `package.json` `packageManager`)
- No new external dependencies — `reqwest` already in `Cargo.toml`
- Follow existing code patterns: `GuardState` for toggles, `CmdResult` for Rust commands, `useQuery` + `invoke` for frontend
- Branch: `feat/traffic-tracer` in `clash-verge-rev` (already created, based on `dev`)

---

### Task 0: Environment Setup

**Files:**
- None (system configuration only)

**Prerequisites:**
- [ ] **Step 0.1: Install Rust toolchain**

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
source "$HOME/.cargo/env"
rustup install 1.95.0
rustup default 1.95.0
rustc --version  # should print 1.95.0
```

- [ ] **Step 0.2: Install Tauri system dependencies (Linux)**

```bash
# Ubuntu/Debian
sudo apt-get update && sudo apt-get install -y \
  libwebkit2gtk-4.1-dev libappindicator3-dev librsvg2-dev \
  patchelf libssl-dev libsoup-3.0-dev libjavascriptcoregtk-4.1-dev
```

- [ ] **Step 0.3: Install frontend dependencies**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm install
```

Expected: `pnpm install` completes without error, `node_modules/` populated.

- [ ] **Step 0.4: Fetch Rust dependencies**

```bash
cd /data/ytluo/projects/clash-verge-rev
cargo fetch
```

Expected: All crate dependencies fetched, `Cargo.lock` generated.

---

### Task 1: Create Rust Tracing Module

**Files:**
- Create: `src-tauri/src/cmd/tracing.rs`

**Interfaces:**
- Produces: `get_tracing_state() -> CmdResult<TracingState>`, `patch_tracing_state(payload: TracingPatch) -> CmdResult<TracingState>`

- [ ] **Step 1.1: Write the tracing module**

```rust
use super::CmdResult;
use crate::config::Config;
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TracingState {
    pub enabled: bool,
    #[serde(default)]
    pub output: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct TracingPatch {
    pub enabled: Option<bool>,
    pub output: Option<String>,
}

async fn tracing_url() -> Result<(String, Option<String>), String> {
    let clash_info = Config::clash().await.data_arc().get_client_info();
    let url = format!("http://{}/experimental/tracing", clash_info.server);
    Ok((url, clash_info.secret))
}

#[tauri::command]
pub async fn get_tracing_state() -> CmdResult<TracingState> {
    let (url, secret) = tracing_url().await.map_err(|e| e.to_string())?;
    let client = reqwest::Client::new();
    let mut req = client.get(&url);
    if let Some(ref s) = secret {
        req = req.header("Authorization", format!("Bearer {}", s));
    }
    let resp = req.send().await.map_err(|e| e.to_string())?;
    if !resp.status().is_success() {
        return Err(format!("mihomo returned {}", resp.status()));
    }
    let state: TracingState = resp.json().await.map_err(|e| e.to_string())?;
    Ok(state)
}

#[tauri::command]
pub async fn patch_tracing_state(payload: TracingPatch) -> CmdResult<TracingState> {
    let (url, secret) = tracing_url().await.map_err(|e| e.to_string())?;
    let client = reqwest::Client::new();
    let mut req = client.patch(&url);
    if let Some(ref s) = secret {
        req = req.header("Authorization", format!("Bearer {}", s));
    }
    let body = serde_json::to_value(&payload).map_err(|e| e.to_string())?;
    let resp = req.json(&body).send().await.map_err(|e| e.to_string())?;
    if !resp.status().is_success() {
        return Err(format!("mihomo returned {}", resp.status()));
    }
    let state: TracingState = resp.json().await.map_err(|e| e.to_string())?;
    Ok(state)
}
```

- [ ] **Step 1.2: Verify the file compiles** (after Task 2 registration)

---

### Task 2: Register Tracing Commands

**Files:**
- Modify: `src-tauri/src/cmd/mod.rs:22` (add module)
- Modify: `src-tauri/src/lib.rs:161` (register commands)

- [ ] **Step 2.1: Add module declaration to cmd/mod.rs**

At line 22 (after `pub mod webdav;`), add:
```rust
pub mod tracing;
```

At line 40 (after `pub use webdav::*;`), add:
```rust
pub use tracing::*;
```

- [ ] **Step 2.2: Register commands in lib.rs**

In `generate_handlers()`, at line 161 (after `cmd::patch_clash_config,`), add:
```rust
cmd::get_tracing_state,
cmd::patch_tracing_state,
```

- [ ] **Step 2.3: Verify compilation**

```bash
cd /data/ytluo/projects/clash-verge-rev
cargo check 2>&1 | head -20
```

Expected: `cargo check` completes without errors. If there are compilation errors, fix them before proceeding.

- [ ] **Step 2.4: Commit**

```bash
git add src-tauri/src/cmd/tracing.rs src-tauri/src/cmd/mod.rs src-tauri/src/lib.rs
git commit -m "feat: add tracing commands to Rust backend"
```

---

### Task 3: Add TypeScript Types

**Files:**
- Modify: Find or create the types file. Check: `src/types/` directory for existing type declarations.

- [ ] **Step 3.1: Check existing type location**

```bash
ls /data/ytluo/projects/clash-verge-rev/src/types/ 2>/dev/null || echo "NO_TYPES_DIR"
grep -rn "IConfigData\|IVergeConfig" /data/ytluo/projects/clash-verge-rev/src --include="*.d.ts" --include="*.ts" -l | head -5
```

Use the output to determine where to add types. If a `src/types/index.d.ts` or similar exists, add there. Otherwise add inline to `src/types/index.d.ts` (create if needed).

- [ ] **Step 3.2: Add type declarations**

If `src/types/` directory exists with a `.d.ts` file, append to it. Otherwise create `src/types/index.d.ts`:

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

- [ ] **Step 3.3: Verify typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No new type errors (pre-existing ones unrelated to this change are acceptable).

- [ ] **Step 3.4: Commit**

```bash
git add src/types/
git commit -m "feat: add TrafficTracer TypeScript types"
```

---

### Task 4: Add cmds.ts API Functions

**Files:**
- Modify: `src/services/cmds.ts` (append at end)

- [ ] **Step 4.1: Add tracing API functions to cmds.ts**

Append at the end of `src/services/cmds.ts` (after line 587, the `isPortInUse` function):

```typescript
export async function getTracingState() {
  return invoke<ITracingState>('get_tracing_state')
}

export async function patchTracingState(payload: ITracingPatch) {
  return invoke<ITracingState>('patch_tracing_state', { payload })
}
```

- [ ] **Step 4.2: Verify typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No errors related to `getTracingState`/`patchTracingState` or `ITracingState`/`ITracingPatch`.

- [ ] **Step 4.3: Commit**

```bash
git add src/services/cmds.ts
git commit -m "feat: add getTracingState/patchTracingState to cmds.ts"
```

---

### Task 5: Create use-tracing.ts Hook

**Files:**
- Create: `src/hooks/use-tracing.ts`

- [ ] **Step 5.1: Write the hook**

```typescript
import { useQuery } from '@tanstack/react-query'

import { getTracingState, patchTracingState } from '@/services/cmds'

export const useTracing = () => {
  const { data: tracing, refetch: mutateTracing } = useQuery({
    queryKey: ['getTracingState'],
    queryFn: getTracingState,
    staleTime: 5000,
  })

  const patchTracing = async (payload: ITracingPatch) => {
    await patchTracingState(payload)
    await mutateTracing()
  }

  return { tracing, mutateTracing, patchTracing }
}
```

- [ ] **Step 5.2: Verify typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No errors related to `use-tracing.ts`.

- [ ] **Step 5.3: Commit**

```bash
git add src/hooks/use-tracing.ts
git commit -m "feat: add useTracing React Query hook"
```

---

### Task 6: Add UI Controls to setting-clash.tsx

**Files:**
- Modify: `src/components/setting/setting-clash.tsx`

- [ ] **Step 6.1: Add imports**

Add to the import block at the top of the file (alongside other hook/mod imports):

```typescript
import { useTracing } from '@/hooks/use-tracing'
```

Ensure `TextField` is already imported from `@mui/material` (check line 2 — it includes `TextField`).

- [ ] **Step 6.2: Add hook call to component**

Inside the `SettingClash` component function, after the existing hook calls (`useVerge`, `useClash`, etc.), add:

```typescript
const { tracing, patchTracing } = useTracing()
```

- [ ] **Step 6.3: Add UI elements**

Insert the following JSX inside `<SettingList>`, after the last existing `<SettingItem>` (the tunnels entry at line 284-287) and before `</SettingList>`:

```tsx
<SettingItem
  label={t('settings.sections.clash.form.fields.tracing')}
>
  <GuardState
    value={tracing?.enabled ?? false}
    valueProps="checked"
    onCatch={onError}
    onFormat={onSwitchFormat}
    onChange={() => {}}
    onGuard={(e) => patchTracing({ enabled: e })}
  >
    <Switch edge="end" />
  </GuardState>
</SettingItem>

{tracing?.enabled && (
  <SettingItem
    label={t('settings.sections.clash.form.fields.tracingOutput')}
    extra={
      <TooltipIcon
        title={t('settings.sections.clash.form.tooltips.tracingOutput')}
        sx={{ opacity: '0.7' }}
      />
    }
  >
    <TextField
      autoComplete="new-password"
      size="small"
      value={tracing?.output ?? ''}
      placeholder="stdout"
      sx={{ width: 250, input: { py: '7.5px' } }}
      onBlur={(e) => {
        if (e.target.value !== tracing?.output) {
          patchTracing({ output: e.target.value || '' })
        }
      }}
    />
  </SettingItem>
)}
```

- [ ] **Step 6.4: Verify typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No errors from setting-clash.tsx.

- [ ] **Step 6.5: Commit**

```bash
git add src/components/setting/setting-clash.tsx
git commit -m "feat: add TrafficTracer toggle and output path to Settings > Clash"
```

---

### Task 7: Add i18n Translation Keys

**Files:**
- Modify: `src/locales/en.json`
- Modify: `src/locales/zh.json`

- [ ] **Step 7.1: Check locale file structure**

```bash
ls /data/ytluo/projects/clash-verge-rev/src/locales/
```

Note which locale files exist. At minimum, add to `en.json` and `zh.json`.

- [ ] **Step 7.2: Find the insertion point in en.json**

The existing keys are nested under `settings.sections.clash.form.fields` and `settings.sections.clash.form.tooltips`. Open `src/locales/en.json` and locate the relevant section (search for `"tunnels"` as it's the last existing key under `fields`).

- [ ] **Step 7.3: Add English keys**

After the last existing key under `settings.sections.clash.form.fields` (currently `"tunnels"`), add:

```json
"tracing": "Traffic Tracer",
"tracingOutput": "Output Path"
```

Under `settings.sections.clash.form.tooltips`, add:

```json
"tracingOutput": "Empty = stdout"
```

- [ ] **Step 7.4: Add Chinese keys**

In `src/locales/zh.json`, add corresponding translations:

Under `settings.sections.clash.form.fields`:
```json
"tracing": "流量追踪",
"tracingOutput": "输出路径"
```

Under `settings.sections.clash.form.tooltips`:
```json
"tracingOutput": "留空即输出到标准输出"
```

- [ ] **Step 7.5: Verify**

Check the JSON is valid and the keys are correctly nested. The `settings.sections.clash.form.fields` and `settings.sections.clash.form.tooltips` paths must match the `t()` calls in the UI code.

- [ ] **Step 7.6: Commit**

```bash
git add src/locales/
git commit -m "feat: add TrafficTracer i18n keys (en + zh)"
```

---

### Task 8: Build Verification

**Files:**
- None (verification only)

- [ ] **Step 8.1: Full Rust build check**

```bash
cd /data/ytluo/projects/clash-verge-rev/src-tauri
cargo check 2>&1 | tail -30
```

Expected: `cargo check` completes without errors from `cmd/tracing.rs` or `lib.rs`.

- [ ] **Step 8.2: Frontend typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No TypeScript errors introduced by our changes.

- [ ] **Step 8.3: Build the application (optional, requires full Tauri env)**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm dev
```

This launches clash-verge-rev in development mode. Verify:
1. Open Settings > Clash section
2. See "Traffic Tracer" switch and "Output Path" field (when enabled)
3. Toggle the switch — check mihomo response (requires mihomo running on localhost)
4. Verify the API call goes through correctly

- [ ] **Step 8.4: Commit any fixes from verification**

If any issues were found and fixed during verification:

```bash
git add -A
git commit -m "fix: corrections from build verification"
```

---

## Execution Summary

| Task | Files | Est. Time |
|------|-------|-----------|
| Task 0 | Environment setup | 15 min |
| Task 1 | `cmd/tracing.rs` (create) | 10 min |
| Task 2 | `cmd/mod.rs`, `lib.rs` (modify) | 5 min |
| Task 3 | `src/types/` (modify) | 10 min |
| Task 4 | `src/services/cmds.ts` (modify) | 5 min |
| Task 5 | `src/hooks/use-tracing.ts` (create) | 5 min |
| Task 6 | `src/components/setting/setting-clash.tsx` (modify) | 10 min |
| Task 7 | `src/locales/*.json` (modify) | 10 min |
| Task 8 | Build verification | 10 min |

**Total: ~80 min, ~150 lines of code across 8 files.**
