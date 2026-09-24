#!/usr/bin/env bash
set -euo pipefail

PROFILE_TYPE="${1:-cpu}"
DURATION="${2:-30}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LATEST_LINK="${REPO_ROOT}/.profile/latest"

if [ ! -d "${LATEST_LINK}" ]; then
    echo "[profiling] Error: No active or prior profiling session found at ${LATEST_LINK}."
    echo "[profiling] Start a session first with: karto run profiling/desktop"
    exit 1
fi

SESSION_DIR="$(readlink -f "${LATEST_LINK}")"
ENDPOINT_FILE="${SESSION_DIR}/backend-pprof.endpoint"

if [ ! -f "${ENDPOINT_FILE}" ]; then
    echo "[profiling] Error: Backend pprof endpoint file not found at ${ENDPOINT_FILE}."
    echo "[profiling] Ensure the desktop app is currently running in profiling mode."
    exit 1
fi

PPROF_ENDPOINT="$(head -n 1 "${ENDPOINT_FILE}" | tr -d '[:space:]')"

if [ -z "${PPROF_ENDPOINT}" ]; then
    echo "[profiling] Error: Empty pprof endpoint in ${ENDPOINT_FILE}."
    exit 1
fi

TIMESTAMP="$(date +%Y%m%d_%H%M%S)"

case "${PROFILE_TYPE}" in
    cpu)
        OUTPUT_FILE="${SESSION_DIR}/backend-cpu-${TIMESTAMP}.pprof"
        echo "[profiling] Collecting ${DURATION}s CPU profile from ${PPROF_ENDPOINT}/debug/pprof/profile?seconds=${DURATION}..."
        curl -sS "${PPROF_ENDPOINT}/debug/pprof/profile?seconds=${DURATION}" -o "${OUTPUT_FILE}"
        echo "[profiling] ✓ Saved CPU profile to: ${OUTPUT_FILE}"
        echo "[profiling] Inspect with:"
        echo "  go tool pprof ${OUTPUT_FILE}"
        echo "  go tool pprof -http=127.0.0.1:8080 ${OUTPUT_FILE}"
        ;;
    heap)
        OUTPUT_FILE="${SESSION_DIR}/backend-heap-${TIMESTAMP}.pprof"
        echo "[profiling] Collecting Heap profile from ${PPROF_ENDPOINT}/debug/pprof/heap..."
        curl -sS "${PPROF_ENDPOINT}/debug/pprof/heap" -o "${OUTPUT_FILE}"
        echo "[profiling] ✓ Saved Heap profile to: ${OUTPUT_FILE}"
        echo "[profiling] Inspect with:"
        echo "  go tool pprof ${OUTPUT_FILE}"
        echo "  go tool pprof -http=127.0.0.1:8080 ${OUTPUT_FILE}"
        ;;
    goroutine)
        OUTPUT_FILE="${SESSION_DIR}/backend-goroutine-${TIMESTAMP}.pprof"
        echo "[profiling] Collecting Goroutine profile from ${PPROF_ENDPOINT}/debug/pprof/goroutine..."
        curl -sS "${PPROF_ENDPOINT}/debug/pprof/goroutine" -o "${OUTPUT_FILE}"
        echo "[profiling] ✓ Saved Goroutine profile to: ${OUTPUT_FILE}"
        ;;
    allocs)
        OUTPUT_FILE="${SESSION_DIR}/backend-allocs-${TIMESTAMP}.pprof"
        echo "[profiling] Collecting Allocations profile from ${PPROF_ENDPOINT}/debug/pprof/allocs..."
        curl -sS "${PPROF_ENDPOINT}/debug/pprof/allocs" -o "${OUTPUT_FILE}"
        echo "[profiling] ✓ Saved Allocations profile to: ${OUTPUT_FILE}"
        ;;
    *)
        echo "[profiling] Unknown profile type '${PROFILE_TYPE}'. Supported: cpu, heap, goroutine, allocs."
        exit 1
        ;;
esac
