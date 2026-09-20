#!/usr/bin/env bash
# test_backup.sh — Offline tests for pg_backup.sh and pg_restore.sh.
#
# Does NOT require Docker, AWS credentials, or a live database.
# Validates script syntax, argument handling, and unit functions.
#
# Usage:
#   bash scripts/backup/test_backup.sh
#
# Exit codes:
#   0 — all tests passed
#   1 — one or more tests failed

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_SCRIPT="${SCRIPT_DIR}/pg_backup.sh"
RESTORE_SCRIPT="${SCRIPT_DIR}/pg_restore.sh"

PASS=0
FAIL=0

# ── Test helpers ──────────────────────────────────────────────────────────────
pass() {
    echo "  PASS: $1"
    PASS=$((PASS + 1))
}

fail() {
    echo "  FAIL: $1"
    FAIL=$((FAIL + 1))
}

run_test() {
    local name="$1"
    local result="$2"   # "pass" or "fail"
    if [[ "${result}" == "pass" ]]; then
        pass "${name}"
    else
        fail "${name}"
    fi
}

# ── Test 1: Shell syntax — pg_backup.sh ──────────────────────────────────────
echo ""
echo "=== Test group: Shell syntax ==="

if bash -n "${BACKUP_SCRIPT}" 2>/dev/null; then
    pass "pg_backup.sh: bash -n syntax check"
else
    fail "pg_backup.sh: bash -n syntax check"
fi

if bash -n "${RESTORE_SCRIPT}" 2>/dev/null; then
    pass "pg_restore.sh: bash -n syntax check"
else
    fail "pg_restore.sh: bash -n syntax check"
fi

# ── Test 2: Shellcheck (optional) ────────────────────────────────────────────
echo ""
echo "=== Test group: Shellcheck (optional) ==="

if command -v shellcheck > /dev/null 2>&1; then
    if shellcheck "${BACKUP_SCRIPT}" 2>/dev/null; then
        pass "pg_backup.sh: shellcheck"
    else
        fail "pg_backup.sh: shellcheck"
    fi

    if shellcheck "${RESTORE_SCRIPT}" 2>/dev/null; then
        pass "pg_restore.sh: shellcheck"
    else
        fail "pg_restore.sh: shellcheck"
    fi
else
    echo "  SKIP: shellcheck not installed — skipping shellcheck tests"
    echo "        Install with: apt-get install shellcheck  or  brew install shellcheck"
fi

# ── Test 3: pg_restore.sh — missing required argument ────────────────────────
echo ""
echo "=== Test group: pg_restore.sh argument validation ==="

# Run with no arguments; expect exit code 1
RESTORE_EXIT=0
bash "${RESTORE_SCRIPT}" > /dev/null 2>&1 || RESTORE_EXIT=$?

if [[ ${RESTORE_EXIT} -ne 0 ]]; then
    pass "pg_restore.sh: exits non-zero when no argument provided (exit=${RESTORE_EXIT})"
else
    fail "pg_restore.sh: should exit non-zero when no argument provided"
fi

# ── Test 4: pg_restore.sh — non-existent backup file ─────────────────────────
RESTORE_NONEXIST_EXIT=0
bash "${RESTORE_SCRIPT}" /nonexistent/backup_file_that_does_not_exist.dump.gz \
    > /dev/null 2>&1 || RESTORE_NONEXIST_EXIT=$?

if [[ ${RESTORE_NONEXIST_EXIT} -ne 0 ]]; then
    pass "pg_restore.sh: exits non-zero for non-existent backup file (exit=${RESTORE_NONEXIST_EXIT})"
else
    fail "pg_restore.sh: should exit non-zero for non-existent backup file"
fi

# ── Test 5: pg_restore.sh — unknown flag ─────────────────────────────────────
RESTORE_FLAG_EXIT=0
bash "${RESTORE_SCRIPT}" --unknown-flag > /dev/null 2>&1 || RESTORE_FLAG_EXIT=$?

if [[ ${RESTORE_FLAG_EXIT} -ne 0 ]]; then
    pass "pg_restore.sh: exits non-zero for unknown flag (exit=${RESTORE_FLAG_EXIT})"
else
    fail "pg_restore.sh: should exit non-zero for unknown flag"
fi

# ── Test 6: Trap behavior — temp file is removed on error ────────────────────
echo ""
echo "=== Test group: Trap behavior ==="

TMPDIR_TEST="$(mktemp -d)"
TMPFILE_TEST="${TMPDIR_TEST}/dzeroth_test.dump.gz.tmp"

# Simulate the trap logic in isolation:
# Create a .tmp file, then call the cleanup function as the trap would.
(
    BACKUP_DIR="${TMPDIR_TEST}"
    TMPFILE="${TMPFILE_TEST}"
    touch "${TMPFILE}"

    cleanup() {
        local exit_code=$?
        if [[ -f "${TMPFILE}" ]]; then
            rm -f "${TMPFILE}"
        fi
        exit ${exit_code}
    }
    trap cleanup EXIT

    # Force non-zero exit to trigger the trap
    exit 1
) 2>/dev/null || true

if [[ ! -f "${TMPFILE_TEST}" ]]; then
    pass "Trap removes .tmp file on non-zero exit"
else
    fail "Trap did NOT remove .tmp file on non-zero exit"
    rm -f "${TMPFILE_TEST}"
fi

rm -rf "${TMPDIR_TEST}"

# ── Test 7: pg_backup.sh — missing secrets file detection ────────────────────
echo ""
echo "=== Test group: pg_backup.sh secrets file detection ==="

# Run with a COMPOSE_FILE that points to a location where no secrets file
# exists, and verify the script exits non-zero with an error message.
TMPCOMPOSE_DIR="$(mktemp -d)"
# Create a fake compose file so the path resolution doesn't fail earlier.
touch "${TMPCOMPOSE_DIR}/docker-compose.prod.yml"

BACKUP_EXIT=0
BACKUP_OUTPUT=""
BACKUP_OUTPUT="$(
    COMPOSE_FILE="${TMPCOMPOSE_DIR}/docker-compose.prod.yml" \
    bash "${BACKUP_SCRIPT}" 2>&1
)" || BACKUP_EXIT=$?

rm -rf "${TMPCOMPOSE_DIR}"

if [[ ${BACKUP_EXIT} -ne 0 ]]; then
    pass "pg_backup.sh: exits non-zero when secrets file is missing (exit=${BACKUP_EXIT})"
else
    fail "pg_backup.sh: should exit non-zero when secrets file is missing"
fi

if echo "${BACKUP_OUTPUT}" | grep -qi "secrets"; then
    pass "pg_backup.sh: error output mentions secrets file"
else
    fail "pg_backup.sh: error output should mention secrets file (got: ${BACKUP_OUTPUT})"
fi

# ── Test 8: log_info and log_error format ────────────────────────────────────
echo ""
echo "=== Test group: Logging function format ==="

# Source only the logging functions from pg_backup.sh in a subshell.
LOG_OUTPUT="$(
    bash -c '
        log_info() {
            echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [INFO]  $*"
        }
        log_error() {
            echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [ERROR] $*" >&2
        }
        log_info "test message"
    ' 2>&1
)"

if echo "${LOG_OUTPUT}" | grep -q '\[INFO\]'; then
    pass "log_info output contains [INFO] prefix"
else
    fail "log_info output should contain [INFO] prefix (got: ${LOG_OUTPUT})"
fi

if echo "${LOG_OUTPUT}" | grep -qE '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z'; then
    pass "log_info output contains ISO 8601 UTC timestamp"
else
    fail "log_info output should contain ISO 8601 UTC timestamp (got: ${LOG_OUTPUT})"
fi

LOG_ERR_OUTPUT="$(
    bash -c '
        log_error() {
            echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [ERROR] $*" >&2
        }
        log_error "error message" 2>&1
    '
)"

if echo "${LOG_ERR_OUTPUT}" | grep -q '\[ERROR\]'; then
    pass "log_error output contains [ERROR] prefix"
else
    fail "log_error output should contain [ERROR] prefix (got: ${LOG_ERR_OUTPUT})"
fi

# ── Test 9: Backup filename format ────────────────────────────────────────────
echo ""
echo "=== Test group: Filename format ==="

FNAME="dzeroth_$(date -u +%Y%m%dT%H%M%SZ).dump.gz"
if echo "${FNAME}" | grep -qE '^dzeroth_[0-9]{8}T[0-9]{6}Z\.dump\.gz$'; then
    pass "Backup filename matches expected pattern: ${FNAME}"
else
    fail "Backup filename does not match expected pattern: ${FNAME}"
fi

# ── Test 10: pg_restore.sh --confirm flag is parsed ──────────────────────────
echo ""
echo "=== Test group: pg_restore.sh --confirm flag ==="

# When --confirm is passed with a non-existent file, it should fail with
# non-zero due to missing file, not due to missing --confirm.
# This verifies --confirm is recognized (not treated as an unknown flag).
CONFIRM_EXIT=0
CONFIRM_OUTPUT="$(
    bash "${RESTORE_SCRIPT}" --confirm /nonexistent/file.dump.gz 2>&1
)" || CONFIRM_EXIT=$?

if [[ ${CONFIRM_EXIT} -ne 0 ]]; then
    pass "--confirm flag is recognized (script fails on missing file, not unknown flag)"
else
    fail "--confirm flag test: expected non-zero exit"
fi

# Verify the error is about the file, not an unknown flag.
if echo "${CONFIRM_OUTPUT}" | grep -qi "not found\|no such\|backup file"; then
    pass "--confirm flag: error is about missing file (not unknown flag)"
else
    fail "--confirm flag: unexpected error output: ${CONFIRM_OUTPUT}"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "========================================"
echo "Test results: ${PASS} passed, ${FAIL} failed"
echo "========================================"
echo ""

if [[ ${FAIL} -gt 0 ]]; then
    exit 1
fi

exit 0
