# Dynamic Core Discovery — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace hardcoded core binary list with directory scanning so any `verge-*` binary placed in the sidecar directory is automatically discovered and selectable.

**Architecture:** A new Rust module `core/discovery.rs` scans the sidecar directory at startup for binaries matching `verge-*-{target_triple}`, deriving the core name. Spawning switches from `.sidecar(name)` (which requires pre-registration in `externalBin`) to `.command(full_path)`. The frontend fetches available cores dynamically via a Tauri command instead of a hardcoded array. `VALID_CLASH_CORES` constant is removed; validation checks file existence at runtime.

**Tech Stack:** Rust 1.95.0, Tauri v2, React 19, MUI 9, clash-verge-rev @ `feat/traffic-tracer`

## Global Constraints

- Rust toolchain: channel `1.95.0` from `rust-toolchain.toml`
- No new external dependencies
- Dev mode: sidecar path = `{project}/src-tauri/sidecar/`. Production: sidecar path = next to executable (`current_exe().parent()`)
- Sidecar file naming convention: `{core_name}-{target_triple}` (e.g. `verge-mihomo-tt-x86_64-unknown-linux-gnu`), Tauri convention, cannot change
- Target triple is available at compile time via `env!("TARGET")`
- Existing cores (`verge-mihomo`, `verge-mihomo-alpha`) must continue to work unchanged
- `externalBin` in `tauri.conf.json` is preserved for production bundling but no longer constrains runtime behavior

## File Map

| File | Responsibility |
|------|---------------|
| `src-tauri/src/core/discovery.rs` (new) | `sidecar_dir()`, `discover_cores()`, `resolve_core_path()`, `is_valid_core()` |
| `src-tauri/src/config/verge.rs` | Remove `VALID_CLASH_CORES`, remove startup validation of core name |
| `src-tauri/src/core/manager/state.rs:35-50` | `.sidecar()` → `.command(path)` for spawning |
| `src-tauri/src/core/validate.rs:334-347` | `.sidecar()` → `.command(path)` for validation |
| `src-tauri/src/core/service.rs:355-360` | Core name resolution for service mode |
| `src-tauri/src/core/manager/lifecycle.rs:48-51` | Remove `IVerge::VALID_CLASH_CORES` check, use `discovery::is_valid_core()` |
| `src-tauri/src/cmd/clash.rs` | Add `list_available_cores` Tauri command |
| `src-tauri/src/lib.rs:160-162` | Register `list_available_cores` |
| `src/components/setting/mods/clash-core-viewer.tsx` | Replace static `VALID_CORE` with dynamic fetch |

---

### Task 1: Create Sidecar Discovery Module

**Files:**
- Create: `src-tauri/src/core/discovery.rs`
- Modify: `src-tauri/src/config/verge.rs:288` (remove VALID_CLASH_CORES)
- Modify: `src-tauri/src/config/verge.rs:291-333` (remove startup validation of core name)
- Modify: `src-tauri/src/core/manager/mod.rs` (declare discovery module)

**Interfaces:**
- Produces: `pub fn sidecar_dir() -> PathBuf`, `pub fn discover_cores() -> Vec<String>`, `pub fn resolve_core_path(name: &str) -> Option<PathBuf>`, `pub fn is_valid_core(name: &str) -> bool`

- [ ] **Step 1.1: Create discovery.rs**

Create `src-tauri/src/core/discovery.rs`:

```rust
use std::path::{Path, PathBuf};

/// Compile-time target triple (e.g. "x86_64-unknown-linux-gnu")
const TARGET_TRIPLE: &str = env!("TARGET");

/// Returns the directory where sidecar core binaries live
pub fn sidecar_dir() -> PathBuf {
    if cfg!(debug_assertions) {
        // Dev mode: binaries at src-tauri/sidecar/
        Path::new(env!("CARGO_MANIFEST_DIR")).join("sidecar")
    } else {
        // Production: binaries next to the executable
        std::env::current_exe()
            .ok()
            .and_then(|p| p.parent().map(Path::to_path_buf))
            .unwrap_or_else(|| PathBuf::from("."))
    }
}

/// Scan the sidecar directory and return all discovered core names
pub fn discover_cores() -> Vec<String> {
    let dir = sidecar_dir();
    let mut cores: Vec<String> = Vec::new();

    let suffix = format!("-{}", TARGET_TRIPLE);
    let exe_suffix = format!("-{}.exe", TARGET_TRIPLE);

    if let Ok(entries) = std::fs::read_dir(&dir) {
        for entry in entries.flatten() {
            let name = entry.file_name();
            let name_str = name.to_string_lossy();

            let base = if name_str.ends_with(&exe_suffix) {
                &name_str[..name_str.len() - exe_suffix.len()]
            } else if name_str.ends_with(&suffix) {
                &name_str[..name_str.len() - suffix.len()]
            } else {
                continue;
            };

            // Only include verge-* binaries
            if base.starts_with("verge-") && !cores.contains(&base.to_string()) {
                cores.push(base.to_string());
            }
        }
    }

    cores.sort();
    cores
}

/// Resolve a core name to its full binary path
pub fn resolve_core_path(name: &str) -> Option<PathBuf> {
    let dir = sidecar_dir();

    #[cfg(windows)]
    let filename = format!("{}-{}.exe", name, TARGET_TRIPLE);
    #[cfg(not(windows))]
    let filename = format!("{}-{}", name, TARGET_TRIPLE);

    let path = dir.join(&filename);
    if path.exists() {
        Some(path)
    } else {
        None
    }
}

/// Check whether a core name is valid (binary exists)
pub fn is_valid_core(name: &str) -> bool {
    resolve_core_path(name).is_some()
}
```

- [ ] **Step 1.2: Register discovery module in core/manager/mod.rs**

Open `src-tauri/src/core/manager/mod.rs` and add `pub mod discovery;` among the module declarations. Check the existing structure first:

```bash
grep "^pub mod\|^mod " /data/ytluo/projects/clash-verge-rev/src-tauri/src/core/manager/mod.rs
```

Then add `pub mod discovery;` alongside the other module declarations.

- [ ] **Step 1.3: Remove VALID_CLASH_CORES from verge.rs**

In `src-tauri/src/config/verge.rs`, remove line 288 (the `VALID_CLASH_CORES` constant):

```rust
// REMOVE this line:
pub const VALID_CLASH_CORES: &'static [&'static str] = &["verge-mihomo", "verge-mihomo-alpha"];
```

- [ ] **Step 1.4: Update get_valid_clash_core to use discovery**

In `src-tauri/src/config/verge.rs:354-356`, keep `get_valid_clash_core()` unchanged — it still returns the `clash_core` field or default. But update the comment to note that validation is now done at spawn-time via `is_valid_core()`. No code change needed for this method.

- [ ] **Step 1.5: Remove startup auto-fix validation**

In `src-tauri/src/config/verge.rs:291-333`, remove or simplify the `validate_and_fix_config()` method — it no longer needs to auto-fix invalid core names since invalidity means "binary not found" rather than "not in static list". The method can keep the empty-string check but remove the `VALID_CLASH_CORES.contains()` check. Replace the validation block (lines 300-312) with:

```rust
// Was: if core_str.is_empty() || !Self::VALID_CLASH_CORES.contains(&core_str)
// Now: only guard against empty string; missing binary is runtime-checked
if core_str.is_empty() {
    logging!(warn, Type::Config,
        "启动时发现clash_core配置为空, 将自动修正为 'verge-mihomo'");
    config.clash_core = Some("verge-mihomo".into());
    needs_fix = true;
}
```

- [ ] **Step 1.6: Commit**

```bash
git add src-tauri/src/core/discovery.rs src-tauri/src/core/manager/mod.rs src-tauri/src/config/verge.rs
git commit -m "feat: add sidecar directory scanning, remove hardcoded VALID_CLASH_CORES"
```

---

### Task 2: Replace .sidecar() with .command(full_path) in Spawning

**Files:**
- Modify: `src-tauri/src/core/manager/state.rs:30-50`
- Modify: `src-tauri/src/core/validate.rs:334-347`
- Modify: `src-tauri/src/core/service.rs:355-360`

**Interfaces:**
- Consumes: `crate::core::manager::discovery::{resolve_core_path, sidecar_dir}` from Task 1
- Modifies: 3 existing spawn locations to use `.command(path)` instead of `.sidecar(name)`

- [ ] **Step 2.1: Modify state.rs — sidecar spawn path**

In `src-tauri/src/core/manager/state.rs`, add import at top (after line 8):
```rust
use crate::core::manager::discovery;
```

Replace `start_core_by_sidecar` lines 30-37 (the core name resolution and spawn):

```rust
// BEFORE (lines 30-37):
let clash_core = Config::verge().await.latest_arc().get_valid_clash_core();
// ...
let (mut rx, child) = app_handle
    .shell()
    .sidecar(clash_core.as_str())?
    .args([...])
    .spawn()?;

// AFTER:
let clash_core = Config::verge().await.latest_arc().get_valid_clash_core();
let core_path = discovery::resolve_core_path(&clash_core)
    .ok_or_else(|| anyhow::anyhow!("Core binary not found: {}", clash_core))?;
let (mut rx, child) = app_handle
    .shell()
    .command(core_path)
    .args([...])
    .spawn()?;
```

The rest of the function (lines 38-80, args and async handler) remains unchanged.

- [ ] **Step 2.2: Modify validate.rs — validation spawn path**

In `src-tauri/src/core/validate.rs`, add import:
```rust
use crate::core::manager::discovery;
```

Replace lines 334-347 (the validation spawn):

```rust
// BEFORE:
let clash_core = Config::verge().await.latest_arc().get_valid_clash_core();
// ...
let command = app_handle
    .shell()
    .sidecar(clash_core.as_str())?
    .args([...]);

// AFTER:
let clash_core = Config::verge().await.latest_arc().get_valid_clash_core();
let core_path = discovery::resolve_core_path(&clash_core)
    .ok_or_else(|| anyhow::anyhow!("Core binary not found: {}", clash_core))?;
let command = app_handle
    .shell()
    .command(core_path)
    .args([...]);
```

- [ ] **Step 2.3: Modify service.rs — service mode core name**

In `src-tauri/src/core/service.rs:355-360`, the service mode also uses `get_valid_clash_core()` to construct the binary path. This code currently does:
```rust
let bin_path = current_exe()?.with_file_name(format!("{}{}", clash_core, bin_ext));
```

This already uses direct path construction (not `.sidecar()`), so it should continue to work as long as the binary is at `current_exe().parent()/{core_name}` with the right naming. Add use of `discovery::resolve_core_path()` as a fallback, or leave it as-is (it works for production mode).

Add import: `use crate::core::manager::discovery;`

Replace the binary path construction:
```rust
// BEFORE:
let bin_path = current_exe()?.with_file_name(format!("{clash_core}{bin_ext}"));

// AFTER: try path-relative first, then discovery
let bin_path = current_exe()?.with_file_name(format!("{clash_core}{bin_ext}"));
let bin_path = if bin_path.exists() {
    bin_path
} else {
    discovery::resolve_core_path(&clash_core)
        .ok_or_else(|| anyhow::anyhow!("Core binary not found: {clash_core}"))?
};
```

- [ ] **Step 2.4: Commit**

```bash
git add src-tauri/src/core/manager/state.rs src-tauri/src/core/validate.rs src-tauri/src/core/service.rs
git commit -m "feat: spawn core via command(path) instead of sidecar(name)"
```

---

### Task 3: Update Validation + Register New Tauri Commands

**Files:**
- Modify: `src-tauri/src/core/manager/lifecycle.rs:48-51`
- Modify: `src-tauri/src/cmd/clash.rs` (add `list_available_cores`)
- Modify: `src-tauri/src/lib.rs:161-162` (register command)

**Interfaces:**
- Consumes: `discovery::discover_cores()`, `discovery::is_valid_core()` from Task 1
- Produces: Tauri command `list_available_cores() -> CmdResult<Vec<String>>`

- [ ] **Step 3.1: Update change_core validation in lifecycle.rs**

In `src-tauri/src/core/manager/lifecycle.rs`, add import:
```rust
use crate::core::manager::discovery;
```

Replace lines 48-51 (the `change_core` validation):

```rust
// BEFORE:
pub async fn change_core(&self, clash_core: &String) -> Result<(), String> {
    if !IVerge::VALID_CLASH_CORES.contains(&clash_core.as_str()) {

// AFTER:
pub async fn change_core(&self, clash_core: &String) -> Result<(), String> {
    if !discovery::is_valid_core(clash_core.as_str()) {
```

The error message is updated to be more descriptive:

```rust
        return Err(format!(
            "Core binary not found: '{clash_core}'. \
             Place the binary in the sidecar directory and restart."
        ).into());
```

- [ ] **Step 3.2: Add list_available_cores command**

In `src-tauri/src/cmd/clash.rs`, add after the existing `get_clash_logs` function (after line 249):

```rust
use crate::core::manager::discovery;

/// 列出所有可用的核心
#[tauri::command]
pub async fn list_available_cores() -> CmdResult<Vec<String>> {
    Ok(discovery::discover_cores())
}
```

- [ ] **Step 3.3: Register the new command in lib.rs**

In `src-tauri/src/lib.rs`, add the new command to `generate_handlers()` after line 161 (`cmd::patch_clash_config,`):

```rust
cmd::list_available_cores,
```

- [ ] **Step 3.4: Commit**

```bash
git add src-tauri/src/core/manager/lifecycle.rs src-tauri/src/cmd/clash.rs src-tauri/src/lib.rs
git commit -m "feat: runtime core validation + list_available_cores command"
```

---

### Task 4: Dynamic Frontend Core Selection

**Files:**
- Modify: `src/components/setting/mods/clash-core-viewer.tsx`

- [ ] **Step 4.1: Add invoke import and cmds function**

First, add `invoke` import to `clash-core-viewer.tsx` (line 1 area):
```typescript
import { invoke } from '@tauri-apps/api/core'
```

- [ ] **Step 4.2: Remove static VALID_CORE array**

Remove lines 26-37 (the entire `VALID_CORE` array):

```typescript
// REMOVE this block:
const VALID_CORE = [
  {
    name: 'Mihomo',
    core: 'verge-mihomo',
    chipKey: 'settings.modals.clashCore.variants.release',
  },
  {
    name: 'Mihomo Alpha',
    core: 'verge-mihomo-alpha',
    chipKey: 'settings.modals.clashCore.variants.alpha',
  },
]
```

- [ ] **Step 4.3: Fetch cores dynamically on dialog open**

Inside the `useImperativeHandle` callback (around line 51-54), update the `open` handler to fetch available cores:

```typescript
useImperativeHandle(ref, () => ({
  open: async () => {
    try {
      const cores = await invoke<string[]>('list_available_cores')
      setAvailableCores(cores)
    } catch {
      // Fallback to known defaults if discovery fails
      setAvailableCores(['verge-mihomo', 'verge-mihomo-alpha'])
    }
    setOpen(true)
  },
  close: () => setOpen(false),
}))
```

Add the state variable before the `useImperativeHandle`:

```typescript
const [availableCores, setAvailableCores] = useState<string[]>([])
```

- [ ] **Step 4.4: Replace static core references with dynamic list**

Find the JSX section that renders core options (search for `VALID_CORE.map` around line 130-170). Replace with `availableCores.map`. Since we no longer have the `chipKey` for i18n, display the raw core name. Each core gets a `ListItemButton`:

```tsx
{availableCores.map((core) => (
  <ListItemButton
    key={core}
    selected={clash_core === core}
    disabled={changingCore === core || upgrading}
    onClick={() => onCoreChange(core)}
  >
    <ListItemText
      primary={core}
      secondary={
        clash_core === core
          ? t('settings.modals.clashCore.currentCore')
          : undefined
      }
    />
    {clash_core === core && (
      <Chip
        size="small"
        label={t('settings.modals.clashCore.variants.current')}
        color="primary"
      />
    )}
  </ListItemButton>
))}
```

- [ ] **Step 4.5: Verify typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No errors in `clash-core-viewer.tsx`. Pre-existing errors in unrelated files are acceptable.

- [ ] **Step 4.6: Commit**

```bash
git add src/components/setting/mods/clash-core-viewer.tsx
git commit -m "feat: fetch available cores dynamically via list_available_cores"
```

---

### Task 5: Build Verification

**Files:**
- None (verification only)

- [ ] **Step 5.1: Frontend typecheck**

```bash
cd /data/ytluo/projects/clash-verge-rev
pnpm typecheck
```

Expected: No new errors introduced.

- [ ] **Step 5.2: Rust build check**

```bash
cd /data/ytluo/projects/clash-verge-rev
source "$HOME/.cargo/env"
cargo check 2>&1 | grep -E "error|warning|Checking"
```

Expected: No `error` lines from our changed modules (`discovery`, `state`, `lifecycle`, `validate`, `service`, `verge`, `clash/cmd`, `lib`). Warnings from pre-existing code are acceptable.

- [ ] **Step 5.3: Manual verification**

If `cargo check` passes and the app can be launched (`pnpm dev`), verify:
1. Place a custom binary at `src-tauri/sidecar/verge-mihomo-tt-x86_64-unknown-linux-gnu`
2. Open Settings > Clash Core
3. Verify "verge-mihomo-tt" appears in the list
4. Select it → verify the core switches and restarts
5. Remove the binary, restart → verify it disappears from the list

- [ ] **Step 5.4: Commit any verification fixes**

```bash
git add -A
git commit -m "fix: corrections from build verification"
```

---

## Execution Summary

| Task | Files | Est. Time |
|------|-------|-----------|
| Task 1 | `discovery.rs` (create), `verge.rs` (modify), `mod.rs` | 15 min |
| Task 2 | `state.rs`, `validate.rs`, `service.rs` (modify) | 10 min |
| Task 3 | `lifecycle.rs`, `clash.rs`, `lib.rs` (modify) | 10 min |
| Task 4 | `clash-core-viewer.tsx` (modify) | 15 min |
| Task 5 | Build verification | 10 min |

**Total: ~60 min, ~120 lines of code across 10 files.**
