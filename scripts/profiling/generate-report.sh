#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TARGET_DIR="${1:-${REPO_ROOT}/.profile/latest}"

if [ ! -d "${TARGET_DIR}" ]; then
    echo "[profiling] Error: Session directory ${TARGET_DIR} does not exist."
    exit 1
fi

SESSION_DIR="$(readlink -f "${TARGET_DIR}")"
MANIFEST_FILE="${SESSION_DIR}/manifest.json"

if [ ! -f "${MANIFEST_FILE}" ]; then
    echo "[profiling] Warning: manifest.json not found in ${SESSION_DIR}."
fi

python3 - "${SESSION_DIR}" << 'PYEOF'
import os, json, sys, glob

session_dir = sys.argv[1]
manifest_file = os.path.join(session_dir, "manifest.json")
report_file = os.path.join(session_dir, "report.md")

manifest = {}
if os.path.exists(manifest_file):
    try:
        with open(manifest_file, "r") as f:
            manifest = json.load(f)
    except Exception as e:
        print(f"Error reading manifest: {e}", file=sys.stderr)

session_id = manifest.get("session_id", os.path.basename(session_dir))
started_at = manifest.get("started_at", "unknown")
finished_at = manifest.get("finished_at", "still active or interrupted")
duration = manifest.get("duration_seconds")
duration_str = f"{duration:.1f}s" if isinstance(duration, (int, float)) else "N/A"

git_sha = manifest.get("git_commit_sha", "unknown")
git_dirty = manifest.get("git_dirty", False)
build_profile = manifest.get("build_profile", "unknown")
platform_info = manifest.get("platform", {})

pids = manifest.get("pids", {})
tauri_pid = pids.get("tauri_desktop", "N/A")
backend_pid = pids.get("kandev_backend", "N/A")
child_pids = pids.get("child_processes", [])

endpoints = manifest.get("endpoints", {})
pprof_endpoint = endpoints.get("pprof", "N/A")

# Enumerate artifacts
all_files = sorted(os.listdir(session_dir))
artifacts = []
for f in all_files:
    if f == "report.md":
        continue
    f_path = os.path.join(session_dir, f)
    size_bytes = os.path.getsize(f_path) if os.path.isfile(f_path) else 0
    size_str = f"{size_bytes / 1024:.1f} KB" if size_bytes < 1024*1024 else f"{size_bytes / (1024*1024):.2f} MB"
    
    kind = "other"
    if f.endswith(".pprof"):
        kind = "Go pprof profile"
    elif f == "perf.data":
        kind = "Linux perf sampling trace"
    elif f.endswith(".log"):
        kind = "Application log"
    elif f.endswith(".json") and f != "manifest.json":
        kind = "Frontend trace / JSON export"
    elif f == "manifest.json":
        kind = "Session manifest"
    elif f.endswith(".endpoint") or f.endswith(".pid"):
        kind = "Runtime metadata"
    
    artifacts.append((f, kind, size_str, f_path))

report = []
report.append(f"# KanDev Desktop Profiling Report: `{session_id}`\n")
report.append(f"**Generated:** {started_at} | **Duration:** {duration_str}\n")
report.append("## 1. Environment & Build Metadata\n")
report.append(f"- **Git Commit:** `{git_sha}` {'(dirty working tree)' if git_dirty else '(clean)'}")
report.append(f"- **Build Profile:** `{build_profile}`")
report.append(f"- **Platform:** {platform_info.get('os', 'Linux')} ({platform_info.get('kernel', 'unknown')}) on {platform_info.get('arch', 'unknown')}")
report.append(f"- **CPU Model:** {platform_info.get('cpu_model', 'unknown')} ({platform_info.get('cpu_cores', 'N/A')} cores)")
report.append(f"- **Session Directory:** `{session_dir}`\n")

report.append("## 2. Process Tree & Endpoints\n")
report.append(f"- **Tauri Shell PID:** `{tauri_pid}`")
report.append(f"- **KanDev Backend PID:** `{backend_pid}`")
if child_pids:
    report.append(f"- **Child Processes:** {', '.join(f'`{p}`' for p in child_pids)}")
report.append(f"- **Backend pprof Endpoint:** `{pprof_endpoint}`\n")

report.append("## 3. Collected Artifacts\n")
report.append("| Artifact | Type | Size | Path |")
report.append("|---|---|---|---|")
for name, kind, size_str, path in artifacts:
    report.append(f"| `{name}` | {kind} | {size_str} | `{path}` |")
report.append("")

# Frontend trace analysis
frontend_traces = [f for f in all_files if f.endswith(".json") and f != "manifest.json"]
if frontend_traces:
    report.append("## 4. Frontend / WebView Trace Analysis\n")
    for ft in frontend_traces:
        ft_path = os.path.join(session_dir, ft)
        try:
            with open(ft_path, "r", errors="ignore") as f:
                data = json.load(f)
            
            events = data.get("traceEvents", data if isinstance(data, list) else None)
            if isinstance(events, list):
                long_tasks = []
                total_duration_us = 0
                cat_times = {}
                for ev in events:
                    dur_us = ev.get("dur", 0)
                    name = ev.get("name", "")
                    cat = ev.get("cat", "other")
                    if dur_us > 0:
                        total_duration_us += dur_us
                        cat_times[cat] = cat_times.get(cat, 0) + (dur_us / 1000.0)
                    if dur_us >= 50000:  # >= 50ms Long Task
                        long_tasks.append((dur_us / 1000.0, name, cat))
                
                long_tasks.sort(key=lambda x: x[0], reverse=True)
                report.append(f"### Chrome / WebKit Performance Trace: `{ft}`\n")
                report.append(f"- **Total Captured Events:** {len(events):,}")
                report.append(f"- **Long Tasks (>50ms):** {len(long_tasks)}")
                if long_tasks:
                    report.append(f"- **Longest Task:** {long_tasks[0][0]:.2f} ms (`{long_tasks[0][1]}` in category `{long_tasks[0][2]}`)")
                    report.append("\n**Top 5 Longest Tasks (Main Thread Blockers):**")
                    report.append("| Duration | Category | Event / Task Name |")
                    report.append("|---|---|---|")
                    for d_ms, t_name, c_name in long_tasks[:5]:
                        report.append(f"| {d_ms:.2f} ms | `{c_name}` | `{t_name}` |")
                    report.append("")
                
                if cat_times:
                    top_cats = sorted(cat_times.items(), key=lambda x: x[1], reverse=True)[:5]
                    report.append("\n**Main Thread Time Breakdown:**")
                    report.append("| Category | Total Time (ms) |")
                    report.append("|---|---|")
                    for c_name, c_ms in top_cats:
                        report.append(f"| `{c_name}` | {c_ms:.2f} ms |")
                    report.append("")
            elif isinstance(data, dict) and any(k in data for k in ["actualDuration", "commitFilter"]):
                report.append(f"### React Profiler Recording: `{ft}`\n")
                report.append("- Valid React DevTools component profiler session detected.")
            else:
                report.append(f"### Frontend JSON Export: `{ft}`\n")
                report.append(f"- Saved {size_str} artifact.")
        except Exception as e:
            report.append(f"### Frontend Trace: `{ft}`\n")
            report.append(f"- Note: Could not parse trace data ({e}).")

report.append("## 5. Inspection & Analysis Guide\n")
report.append("### Frontend / WebView Traces")
report.append("Exported Chrome DevTools traces (`.json`) can be loaded in:")
report.append("- Chrome / Chromium: `chrome://tracing` or DevTools Performance panel (Load profile button)")
report.append("- Web app: [https://ui.perfetto.dev](https://ui.perfetto.dev)")
report.append("- React DevTools: In Chromium browser -> DevTools -> Profiler -> Load profile button\n")

report.append("### Go Backend Profiling (`pprof`)")
report.append("Analyze captured `.pprof` files with Go tool:")
report.append("```bash")
report.append(f"# Interactive CLI:")
report.append(f"go tool pprof {session_dir}/<profile-name>.pprof")
report.append("")
report.append(f"# Interactive Web UI (flamegraph, call tree, source):")
report.append(f"go tool pprof -http=127.0.0.1:8080 {session_dir}/<profile-name>.pprof")
report.append("```\n")

if any(f == "perf.data" for f, _, _, _ in artifacts):
    report.append("### Native Linux Profiling (`perf`)")
    report.append("Inspect system and native thread CPU attribution:")
    report.append("```bash")
    report.append(f"perf report -i {session_dir}/perf.data --stdio")
    report.append("```\n")

content = "\n".join(report) + "\n"
with open(report_file, "w") as f:
    f.write(content)

print(content)
print(f"[profiling] ✓ Report saved to: {report_file}")
PYEOF
