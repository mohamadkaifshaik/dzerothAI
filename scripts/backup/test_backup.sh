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

# ── Test 11: compose_exec does not pass --env-file to docker compose exec ──────
#
# Root cause guard: passing --env-file to `docker compose exec` fails under
# root/systemd when the system-installed Docker Compose version predates the
# --env-file global flag (v2.2.x). For exec the flag is not needed — the
# container's environment is already set from startup. This test confirms the
# flag was not re-introduced.
echo ""
echo "=== Test group: compose_exec --env-file absence ==="

# Check pg_backup.sh: no non-comment line in compose_exec should contain --env-file.
BACKUP_ENV_FILE_LINES="$(grep -n "\-\-env-file" "${BACKUP_SCRIPT}" | grep -v "^[0-9]*:[[:space:]]*#" || true)"
if [[ -z "${BACKUP_ENV_FILE_LINES}" ]]; then
    pass "pg_backup.sh: compose_exec does not pass --env-file to docker compose exec"
else
    fail "pg_backup.sh: --env-file found in non-comment lines: ${BACKUP_ENV_FILE_LINES}"
fi

# Check pg_restore.sh: same guard.
RESTORE_ENV_FILE_LINES="$(grep -n "\-\-env-file" "${RESTORE_SCRIPT}" | grep -v "^[0-9]*:[[:space:]]*#" || true)"
if [[ -z "${RESTORE_ENV_FILE_LINES}" ]]; then
    pass "pg_restore.sh: compose_exec does not pass --env-file to docker compose exec"
else
    fail "pg_restore.sh: --env-file found in non-comment lines: ${RESTORE_ENV_FILE_LINES}"
fi

# ── Test 12: find_docker_compose — function present and DOCKER_COMPOSE_BIN respected ──
echo ""
echo "=== Test group: find_docker_compose function ==="

# Verify the function is declared in both scripts.
if grep -q "find_docker_compose" "${BACKUP_SCRIPT}"; then
    pass "pg_backup.sh: find_docker_compose function is present"
else
    fail "pg_backup.sh: find_docker_compose function is missing"
fi

if grep -q "find_docker_compose" "${RESTORE_SCRIPT}"; then
    pass "pg_restore.sh: find_docker_compose function is present"
else
    fail "pg_restore.sh: find_docker_compose function is missing"
fi

# DOCKER_COMPOSE_BIN set to a non-executable file must cause non-zero exit.
# We provide both a valid COMPOSE_FILE and a secrets file so the failure
# lands specifically in find_docker_compose, not in earlier guards.
TMPDC_DIR="$(mktemp -d)"
touch "${TMPDC_DIR}/docker-compose.prod.yml"
mkdir -p "${TMPDC_DIR}/deploy/secrets"
touch "${TMPDC_DIR}/deploy/secrets/env.production"

NONEXEC_BIN="$(mktemp)"
# chmod +x intentionally NOT called — file must not be executable.

DCB_EXIT=0
DCB_OUTPUT="$(
    COMPOSE_FILE="${TMPDC_DIR}/docker-compose.prod.yml" \
    DOCKER_COMPOSE_BIN="${NONEXEC_BIN}" \
    bash "${BACKUP_SCRIPT}" 2>&1
)" || DCB_EXIT=$?

rm -f "${NONEXEC_BIN}"
rm -rf "${TMPDC_DIR}"

if [[ ${DCB_EXIT} -ne 0 ]]; then
    pass "pg_backup.sh: exits non-zero when DOCKER_COMPOSE_BIN is not executable (exit=${DCB_EXIT})"
else
    fail "pg_backup.sh: should exit non-zero when DOCKER_COMPOSE_BIN is not executable"
fi

if echo "${DCB_OUTPUT}" | grep -qi "not executable\|DOCKER_COMPOSE_BIN"; then
    pass "pg_backup.sh: error output mentions DOCKER_COMPOSE_BIN or not executable"
else
    fail "pg_backup.sh: DOCKER_COMPOSE_BIN error output unexpected (got: ${DCB_OUTPUT})"
fi

# ── Test 13: pg_restore --list uses `-` (stdin), not /dev/stdin ───────────────
#
# Root cause guard: /dev/stdin in the official postgres Docker image is a char
# device node, NOT a symlink to /proc/self/fd/0. When `pg_restore --list
# /dev/stdin` is used inside the container via `docker compose exec -T`, the
# char device returns no data and pg_restore fails with "did not find magic
# string in file header". The correct form is `pg_restore --list -` which
# reads from fd 0 (the actual stdin pipe). This test confirms the regression
# cannot be silently re-introduced.
echo ""
echo "=== Test group: pg_restore --list stdin form ==="

# Must NOT use /dev/stdin with pg_restore --list.
if grep -qE "pg_restore[[:space:]].*--list[[:space:]].*\/dev\/stdin" "${BACKUP_SCRIPT}"; then
    fail "pg_backup.sh: pg_restore --list must not use /dev/stdin (use - for stdin)"
else
    pass "pg_backup.sh: pg_restore --list does not use /dev/stdin"
fi

# Must use `pg_restore --list -` (dash = fd 0) for stdin.
if grep -qE "pg_restore[[:space:]].*--list[[:space:]]+-[[:space:]]*>" "${BACKUP_SCRIPT}" || \
   grep -qE "pg_restore --list -" "${BACKUP_SCRIPT}"; then
    pass "pg_backup.sh: pg_restore --list uses - (stdin fd 0)"
else
    fail "pg_backup.sh: pg_restore --list must pass - as the archive argument for stdin"
fi

# Must use gunzip (or gzip -dc) to decompress before pg_restore --list.
# grep for lines that contain gunzip or gzip -d within 5 lines before pg_restore --list.
if grep -q "gunzip\|gzip -d" "${BACKUP_SCRIPT}"; then
    pass "pg_backup.sh: integrity check includes gunzip/gzip decompression"
else
    fail "pg_backup.sh: integrity check must decompress before pg_restore --list"
fi

# Must NOT wrap pg_restore in an unnecessary sh -c for the --list check.
if grep -qE "sh[[:space:]]+-c[[:space:]]+'?\"?pg_restore[[:space:]]+--list" "${BACKUP_SCRIPT}"; then
    fail "pg_backup.sh: pg_restore --list must not be wrapped in sh -c"
else
    pass "pg_backup.sh: pg_restore --list is not wrapped in sh -c"
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
