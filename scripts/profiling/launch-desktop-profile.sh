#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mkdir -p "${REPO_ROOT}/.profile"

SESSION_ID="$(date +%Y%m%d_%H%M%S)"
SESSION_DIR="${REPO_ROOT}/.profile/${SESSION_ID}"
mkdir -p "${SESSION_DIR}"

# Update relative latest symlink
(cd "${REPO_ROOT}/.profile" && ln -sfn "${SESSION_ID}" latest)

DESKTOP_BIN="${REPO_ROOT}/apps/desktop/src-tauri/target/release/kandev-desktop"
if [ ! -f "${DESKTOP_BIN}" ]; then
    echo "[profiling] Error: Profiling desktop executable not found at ${DESKTOP_BIN}."
    echo "[profiling] Please build it first with: karto run profiling/build"
    exit 1
fi

# Export runtime environment
export KANDEV_PROFILE=1
export KANDEV_DESKTOP_PORT="${KANDEV_DESKTOP_PORT:-38430}"
export KANDEV_PROFILE_PPROF_FILE="${SESSION_DIR}/backend-pprof.endpoint"
export KANDEV_BACKEND_PID_FILE="${SESSION_DIR}/backend.pid"
export KANDEV_DESKTOP_RUNTIME_DIR="${REPO_ROOT}/apps/desktop/src-tauri/resources/kandev"

# Platform metadata
GIT_SHA="$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null || echo "unknown")"
GIT_DIRTY="$(git -C "${REPO_ROOT}" status --porcelain 2>/dev/null | grep -q . && echo "true" || echo "false")"
OS_NAME="$(uname -s)"
KERNEL_VER="$(uname -r)"
ARCH_NAME="$(uname -m)"
CPU_MODEL="$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^[ \t]*//' || echo "unknown")"
CPU_CORES="$(nproc 2>/dev/null || echo 1)"
START_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
START_EPOCH="$(date +%s)"

# Check native Linux perf capability
PERF_REQUESTED="${KANDEV_PROFILE_PERF:-0}"
for arg in "$@"; do
    if [ "$arg" = "--perf" ]; then
        PERF_REQUESTED=1
    fi
done

USE_PERF=false
if [ "${PERF_REQUESTED}" = "1" ]; then
    if command -v perf >/dev/null 2>&1; then
        PARANOID="$(cat /proc/sys/kernel/perf_event_paranoid 2>/dev/null || echo 2)"
        if [ "${PARANOID}" -le 1 ] || [ "$(id -u)" -eq 0 ]; then
            USE_PERF=true
            echo "[profiling] Native 'perf' recording enabled (perf_event_paranoid=${PARANOID})."
        else
            echo "[profiling] Notice: 'perf' requested, but /proc/sys/kernel/perf_event_paranoid=${PARANOID} restricts unprivileged profiling."
            echo "[profiling] To enable: sudo sysctl -w kernel.perf_event_paranoid=1"
            echo "[profiling] Proceeding with standard execution..."
        fi
    else
        echo "[profiling] Notice: 'perf' command not found in PATH. Install linux-tools to enable."
        echo "[profiling] Proceeding with standard execution..."
    fi
else
    echo "[profiling] Native sampling: perf inactive (run with --perf or KANDEV_PROFILE_PERF=1 if desired)."
fi

# Initial manifest
cat << JSONEOF > "${SESSION_DIR}/manifest.json"
{
  "session_id": "${SESSION_ID}",
  "started_at": "${START_TIME}",
  "git_commit_sha": "${GIT_SHA}",
  "git_dirty": ${GIT_DIRTY},
  "build_profile": "profiling (opt-level=3, debug=true, devtools=enabled)",
  "executable_path": "${DESKTOP_BIN}",
  "log_path": "${SESSION_DIR}/desktop.log",
  "platform": {
    "os": "${OS_NAME}",
    "kernel": "${KERNEL_VER}",
    "arch": "${ARCH_NAME}",
    "cpu_model": "${CPU_MODEL}",
    "cpu_cores": ${CPU_CORES}
  },
  "pids": {
    "tauri_desktop": null,
    "kandev_backend": null,
    "child_processes": []
  },
  "endpoints": {
    "frontend": "http://127.0.0.1:${KANDEV_DESKTOP_PORT}/",
    "pprof": null
  }
}
JSONEOF

echo "[profiling] Launching KanDev Desktop..."
echo "[profiling] Session logs: ${SESSION_DIR}/desktop.log"

LOG_FILE="${SESSION_DIR}/desktop.log"

if [ "${USE_PERF}" = "true" ]; then
    perf record -F 99 -g --call-graph dwarf -o "${SESSION_DIR}/perf.data" "${DESKTOP_BIN}" 2>&1 | tee "${LOG_FILE}" &
    APP_PID=$!
else
    "${DESKTOP_BIN}" 2>&1 | tee "${LOG_FILE}" &
    APP_PID=$!
fi

cleanup_on_exit() {
    EXIT_CODE=$?
    END_EPOCH="$(date +%s)"
    END_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    DURATION=$((END_EPOCH - START_EPOCH))
    
    echo ""
    echo "[profiling] KanDev Desktop session ended (duration: ${DURATION}s, exit code: ${EXIT_CODE})."
    
    # If desktop.log is empty, preserve backend logs
    if [ ! -s "${LOG_FILE}" ] && [ -f "${HOME}/.kandev/logs/backend-logs.log" ]; then
        tail -n 1000 "${HOME}/.kandev/logs/backend-logs.log" > "${SESSION_DIR}/backend.log" 2>/dev/null || true
    fi

    # Update manifest with final metadata
    python3 - "${SESSION_DIR}/manifest.json" "${END_TIME}" "${DURATION}" "${EXIT_CODE}" << 'PYEOF' 2>/dev/null || true
import json, os, sys

manifest_path = sys.argv[1]
end_time = sys.argv[2]
duration = float(sys.argv[3]) if sys.argv[3].replace('.', '', 1).isdigit() else 0
exit_code = int(sys.argv[4]) if sys.argv[4].isdigit() else 0

if os.path.exists(manifest_path):
    try:
        with open(manifest_path, "r") as f:
            data = json.load(f)
        data["finished_at"] = end_time
        data["duration_seconds"] = duration
        data["exit_code"] = exit_code
        with open(manifest_path, "w") as f:
            json.dump(data, f, indent=2)
    except Exception:
        pass
PYEOF

    "${REPO_ROOT}/scripts/profiling/generate-report.sh" "${SESSION_DIR}" || true
}

trap cleanup_on_exit EXIT INT TERM

# Poll for backend startup and PID discovery (up to 15 seconds)
BACKEND_PID=""
PPROF_ENDPOINT=""
for _ in $(seq 1 30); do
    if [ ! -d "/proc/${APP_PID}" ] 2>/dev/null; then
        break
    fi
    if [ -f "${SESSION_DIR}/backend.pid" ] && [ -z "${BACKEND_PID}" ]; then
        BACKEND_PID="$(head -n 1 "${SESSION_DIR}/backend.pid" 2>/dev/null | tr -d '[:space:]')"
    fi
    if [ -f "${SESSION_DIR}/backend-pprof.endpoint" ] && [ -z "${PPROF_ENDPOINT}" ]; then
        PPROF_ENDPOINT="$(head -n 1 "${SESSION_DIR}/backend-pprof.endpoint" 2>/dev/null | tr -d '[:space:]')"
    fi
    if [ -n "${BACKEND_PID}" ] && [ -n "${PPROF_ENDPOINT}" ]; then
        break
    fi
    sleep 0.5
done

# Collect child process PIDs
CHILD_PIDS=()
if [ -n "${BACKEND_PID}" ] && command -v pgrep >/dev/null 2>&1; then
    while IFS= read -r c_pid; do
        if [ -n "$c_pid" ]; then
            CHILD_PIDS+=("$c_pid")
        fi
    done < <(pgrep -P "${BACKEND_PID}" 2>/dev/null || true)
fi

# Update manifest with discovered PIDs and endpoint
python3 - "${SESSION_DIR}/manifest.json" "${APP_PID}" "${BACKEND_PID}" "${PPROF_ENDPOINT}" "${CHILD_PIDS[*]:-}" << 'PYEOF' 2>/dev/null || true
import json, os, sys

manifest_path = sys.argv[1]
app_pid = int(sys.argv[2]) if sys.argv[2].isdigit() else None
backend_pid = int(sys.argv[3]) if sys.argv[3].isdigit() else None
pprof_endpoint = sys.argv[4] if sys.argv[4] else None
child_pids_raw = sys.argv[5] if len(sys.argv) > 5 else ""
child_pids = [int(p) for p in child_pids_raw.split() if p.isdigit()]

if os.path.exists(manifest_path):
    try:
        with open(manifest_path, "r") as f:
            data = json.load(f)
        data["pids"]["tauri_desktop"] = app_pid
        data["pids"]["kandev_backend"] = backend_pid
        data["pids"]["child_processes"] = child_pids
        data["endpoints"]["frontend"] = f"http://127.0.0.1:{os.environ.get('KANDEV_DESKTOP_PORT', '38430')}/"
        data["endpoints"]["pprof"] = pprof_endpoint
        with open(manifest_path, "w") as f:
            json.dump(data, f, indent=2)
    except Exception:
        pass
PYEOF

echo ""
echo "================================================================================"
echo " KanDev Desktop Profiling Session Active"
echo " Session ID:         ${SESSION_ID}"
echo " Session Directory:  ${SESSION_DIR}"
echo " Tauri PID:          ${APP_PID}"
echo " Backend PID:        ${BACKEND_PID:-pending/detected in log}"
echo " Go pprof Endpoint:  ${PPROF_ENDPOINT:-pending/detected in log}"
echo " Local Web App URL:  http://127.0.0.1:${KANDEV_DESKTOP_PORT}/"
echo ""
echo " ─── Frontend / WebView Profiling Guide ──────────────────────────────────────"
echo " 1. Native WebKit Inspector (in Tauri Desktop window):"
echo "    • Press F12 (or menu: View -> Toggle Developer Tools)"
echo "    • Click 'Timelines' tab -> click Record (circle icon in top-left)"
echo "    • Interact with the Task Page (open tasks, scroll messages, stream tokens)"
echo "    • Click Stop (circle icon) -> click Export icon to save JSON trace"
echo "    • Save JSON file to: ${SESSION_DIR}/"
echo ""
echo " 2. Chrome / Chromium DevTools (Recommended for React Profiler & Deep Flamechart):"
echo "    • Open http://127.0.0.1:${KANDEV_DESKTOP_PORT}/ in Google Chrome, Chromium, or Brave"
echo "    • Open DevTools (F12) -> 'Performance' tab -> Record -> exercise -> Stop"
echo "    • Click 'Save profile...' and save JSON to: ${SESSION_DIR}/"
echo "    • For React components: Open 'Profiler' tab -> Record -> Stop -> Export"
echo ""
echo " ─── Profiling Commands (run in another terminal) ─────────────────────────────"
echo " Capture Heap:       karto run profiling/backend-heap"
echo " Capture CPU (30s):  karto run profiling/backend-cpu"
echo " Generate Report:    karto run profiling/report"
echo "================================================================================"
echo ""
echo "[profiling] Manually exercise KanDev now. The session will remain active until"
echo "[profiling] you close the application or press Ctrl+C."

wait "${APP_PID}"
