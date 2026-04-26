#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
USERS="${USERS:-10}"
PART_SIZE_MB="${PART_SIZE_MB:-64}"
MIN_FILE_MB="${MIN_FILE_MB:-256}"
MAX_FILE_MB="${MAX_FILE_MB:-2048}"
MAX_RETRIES="${MAX_RETRIES:-3}"
FLAKE_RATE="${FLAKE_RATE:-18}"
TIMEOUT_RATE="${TIMEOUT_RATE:-12}"
CANCEL_RATE="${CANCEL_RATE:-20}"
ABANDON_RATE="${ABANDON_RATE:-8}"
POST_COMPLETE_CANCEL_RATE="${POST_COMPLETE_CANCEL_RATE:-10}"
POLL_INTERVAL_SEC="${POLL_INTERVAL_SEC:-2}"
MAX_POLL_ATTEMPTS="${MAX_POLL_ATTEMPTS:-90}"
RAMP_UP_SEC="${RAMP_UP_SEC:-10}"

usage() {
  cat <<USAGE
Usage: $(basename "$0") [options]

Options:
  --users N                     Number of concurrent users (default: $USERS)
  --api-base-url URL            API base URL (default: $API_BASE_URL)
  --min-file-mb N               Smallest source video size in MB (default: $MIN_FILE_MB)
  --max-file-mb N               Largest source video size in MB (default: $MAX_FILE_MB)
  --part-size-mb N              Multipart part size in MB, must be >= 5 (default: $PART_SIZE_MB)
  --max-retries N               Max retries per part upload (default: $MAX_RETRIES)
  --flake-rate PCT              Random flaky client failures, 0-100 (default: $FLAKE_RATE)
  --timeout-rate PCT            Random forced client timeouts, 0-100 (default: $TIMEOUT_RATE)
  --cancel-rate PCT             Random in-flight cancellation chance, 0-100 (default: $CANCEL_RATE)
  --abandon-rate PCT            Random user abandon chance, 0-100 (default: $ABANDON_RATE)
  --post-complete-cancel-rate PCT  Chance of queued->cancelled after complete, 0-100 (default: $POST_COMPLETE_CANCEL_RATE)
  --ramp-up-sec N               Spread user starts over this many seconds (default: $RAMP_UP_SEC)
  --help                        Show this help

Required tools: curl, jq, awk, sed, head
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --users) USERS="$2"; shift 2 ;;
    --api-base-url) API_BASE_URL="$2"; shift 2 ;;
    --min-file-mb) MIN_FILE_MB="$2"; shift 2 ;;
    --max-file-mb) MAX_FILE_MB="$2"; shift 2 ;;
    --part-size-mb) PART_SIZE_MB="$2"; shift 2 ;;
    --max-retries) MAX_RETRIES="$2"; shift 2 ;;
    --flake-rate) FLAKE_RATE="$2"; shift 2 ;;
    --timeout-rate) TIMEOUT_RATE="$2"; shift 2 ;;
    --cancel-rate) CANCEL_RATE="$2"; shift 2 ;;
    --abandon-rate) ABANDON_RATE="$2"; shift 2 ;;
    --post-complete-cancel-rate) POST_COMPLETE_CANCEL_RATE="$2"; shift 2 ;;
    --ramp-up-sec) RAMP_UP_SEC="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

for cmd in curl jq awk sed head; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
done

if (( PART_SIZE_MB < 5 )); then
  echo "part-size-mb must be >= 5" >&2
  exit 1
fi
if (( MIN_FILE_MB <= 0 || MAX_FILE_MB < MIN_FILE_MB )); then
  echo "invalid min/max file size" >&2
  exit 1
fi

PART_SIZE_BYTES=$((PART_SIZE_MB * 1024 * 1024))
RUN_ID="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="scripts/load/runs/$RUN_ID"
mkdir -p "$RUN_DIR/logs"
RESULTS_CSV="$RUN_DIR/results.csv"
echo "user,job_id,result,bytes,parts,uploaded_parts,duration_sec,reason" > "$RESULTS_CSV"

echo "Run directory: $RUN_DIR"
echo "Starting simulation: users=$USERS api=$API_BASE_URL"

timestamp() { date +"%Y-%m-%dT%H:%M:%S%z"; }
rand_between() {
  local min="$1" max="$2"
  echo $((RANDOM % (max - min + 1) + min))
}
chance() {
  local pct="$1"
  (( RANDOM % 100 < pct ))
}

json_post() {
  local url="$1" payload="$2" timeout_s="$3" out_file="$4"
  local status
  status=$(curl -sS -m "$timeout_s" -o "$out_file" -w "%{http_code}" \
    -H "Content-Type: application/json" -X POST "$url" -d "$payload" || echo "000")
  echo "$status"
}

cancel_job() {
  local job_id="$1" from="$2" log_file="$3"
  local out="$RUN_DIR/tmp-cancel-$job_id.json"
  local payload
  payload=$(jq -nc --arg id "$job_id" --arg from "$from" '{id:$id, from:$from, to:"cancelled"}')
  local status
  status=$(json_post "$API_BASE_URL/status/$job_id/update" "$payload" 15 "$out")
  echo "[$(timestamp)] cancel job=$job_id from=$from status=$status" >> "$log_file"
}

upload_part() {
  local url="$1" part_bytes="$2" timeout_s="$3" headers_file="$4"
  local status
  : > "$headers_file"
  status=$(head -c "$part_bytes" /dev/urandom | curl -sS -X PUT "$url" -m "$timeout_s" \
    -H "Content-Type: application/octet-stream" --data-binary @- -D "$headers_file" -o /dev/null -w "%{http_code}" || echo "000")
  echo "$status"
}

run_user() {
  local user_idx="$1"
  local log_file="$RUN_DIR/logs/user-$(printf "%03d" "$user_idx").log"
  local started_at
  started_at=$(date +%s)

  local file_mb file_bytes total_parts
  file_mb=$(rand_between "$MIN_FILE_MB" "$MAX_FILE_MB")
  file_bytes=$((file_mb * 1024 * 1024))
  total_parts=$(((file_bytes + PART_SIZE_BYTES - 1) / PART_SIZE_BYTES))

  echo "[$(timestamp)] user=$user_idx start file_mb=$file_mb parts=$total_parts" > "$log_file"

  local init_payload init_resp init_status
  init_payload=$(jq -nc --arg fn "camera_user_${user_idx}_$(date +%s).mp4" --argjson fs "$file_bytes" --argjson ps "$PART_SIZE_BYTES" \
    '{file_name:$fn, file_size:$fs, part_size:$ps}')
  init_resp="$RUN_DIR/tmp-init-$user_idx.json"
  init_status=$(json_post "$API_BASE_URL/upload/initiate" "$init_payload" 30 "$init_resp")

  if [[ "$init_status" -lt 200 || "$init_status" -ge 300 ]]; then
    echo "[$(timestamp)] initiate failed status=$init_status body=$(cat "$init_resp" 2>/dev/null || true)" >> "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,,init_failed,$file_bytes,$total_parts,0,$dur,http_$init_status" >> "$RESULTS_CSV"
    return
  fi

  local job_id upload_id
  job_id=$(jq -r '.metadata.job_id // empty' "$init_resp")
  upload_id=$(jq -r '.metadata.upload_id // empty' "$init_resp")
  if [[ -z "$job_id" || -z "$upload_id" ]]; then
    echo "[$(timestamp)] initiate invalid response $(cat "$init_resp")" >> "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,,init_parse_failed,$file_bytes,$total_parts,0,$dur,missing_job_or_upload_id" >> "$RESULTS_CSV"
    return
  fi

  echo "[$(timestamp)] initiated job=$job_id upload=$upload_id" >> "$log_file"

  if chance "$ABANDON_RATE"; then
    echo "[$(timestamp)] user abandoned session before upload" >> "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,abandoned,$file_bytes,$total_parts,0,$dur,user_left" >> "$RESULTS_CSV"
    return
  fi

  mapfile -t part_lines < <(jq -r '.metadata.parts[] | "\(.part_number)|\(.url)"' "$init_resp")
  if [[ ${#part_lines[@]} -eq 0 ]]; then
    echo "[$(timestamp)] no parts in initiate response" >> "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,no_parts,$file_bytes,$total_parts,0,$dur,empty_parts" >> "$RESULTS_CSV"
    return
  fi

  local -a completed_parts_json=()
  local uploaded_parts=0
  local cancelled_mid_upload=0
  local upload_failed=0

  local part_line part_no part_url part_size_this headers_file etag attempt max_time status
  for part_line in "${part_lines[@]}"; do
    part_no="${part_line%%|*}"
    part_url="${part_line#*|}"

    part_size_this="$PART_SIZE_BYTES"
    if (( part_no == total_parts )); then
      local consumed=$(( (total_parts - 1) * PART_SIZE_BYTES ))
      part_size_this=$(( file_bytes - consumed ))
    fi

    if chance "$CANCEL_RATE"; then
      cancel_job "$job_id" "pending" "$log_file"
      cancelled_mid_upload=1
      break
    fi

    if chance "$FLAKE_RATE"; then
      echo "[$(timestamp)] flaky_disconnect job=$job_id part=$part_no" >> "$log_file"
      upload_failed=1
      break
    fi

    for (( attempt=1; attempt<=MAX_RETRIES; attempt++ )); do
      max_time=120
      if chance "$TIMEOUT_RATE"; then
        max_time=1
      fi

      headers_file="$RUN_DIR/tmp-headers-$job_id-$part_no-$attempt.txt"
      status=$(upload_part "$part_url" "$part_size_this" "$max_time" "$headers_file")
      etag=$(awk 'BEGIN{IGNORECASE=1}/^ETag:/{gsub("\r",""); print $2}' "$headers_file" 2>/dev/null | tail -n1 || true)

      if [[ "$status" -ge 200 && "$status" -lt 300 && -n "$etag" ]]; then
        completed_parts_json+=("$(jq -nc --argjson p "$part_no" --arg e "$etag" '{part_number:$p, etag:$e}')")
        uploaded_parts=$((uploaded_parts + 1))
        echo "[$(timestamp)] uploaded job=$job_id part=$part_no/$total_parts" >> "$log_file"
        break
      fi

      echo "[$(timestamp)] upload_retry job=$job_id part=$part_no attempt=$attempt status=$status" >> "$log_file"
      sleep "0.$((RANDOM % 8 + 2))"

      if (( attempt == MAX_RETRIES )); then
        upload_failed=1
      fi
    done

    if (( upload_failed == 1 )); then
      break
    fi
  done

  if (( cancelled_mid_upload == 1 )); then
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,cancelled_mid_upload,$file_bytes,$total_parts,$uploaded_parts,$dur,client_cancelled" >> "$RESULTS_CSV"
    return
  fi

  if (( upload_failed == 1 )); then
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,upload_failed,$file_bytes,$total_parts,$uploaded_parts,$dur,retries_exhausted_or_flaky" >> "$RESULTS_CSV"
    return
  fi

  local parts_json
  parts_json=$(printf '%s\n' "${completed_parts_json[@]}" | jq -s '.')

  local fmt codec framerate duration
  if chance 50; then fmt="mp4"; else fmt="mov"; fi
  if chance 65; then codec="h264"; else codec="h265"; fi
  framerate=$(rand_between 24 60)
  duration=$(rand_between 30 7200)

  local complete_payload complete_resp complete_status
  complete_payload=$(jq -nc \
    --arg job_id "$job_id" \
    --arg upload_id "$upload_id" \
    --arg video_name "camera_${user_idx}_$(date +%s).$fmt" \
    --arg description "simulated real-world upload from user $user_idx" \
    --arg format "$fmt" \
    --arg codec "$codec" \
    --argjson framerate "$framerate" \
    --argjson duration "$duration" \
    --argjson parts "$parts_json" \
    '{job_id:$job_id, upload_id:$upload_id, parts:$parts, video_name:$video_name, description:$description, format:$format, codec:$codec, framerate:$framerate, duration:$duration}')

  complete_resp="$RUN_DIR/tmp-complete-$job_id.json"
  complete_status=$(json_post "$API_BASE_URL/upload/complete" "$complete_payload" 45 "$complete_resp")
  if [[ "$complete_status" -lt 200 || "$complete_status" -ge 300 ]]; then
    echo "[$(timestamp)] complete failed status=$complete_status body=$(cat "$complete_resp" 2>/dev/null || true)" >> "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,complete_failed,$file_bytes,$total_parts,$uploaded_parts,$dur,http_$complete_status" >> "$RESULTS_CSV"
    return
  fi

  if chance "$POST_COMPLETE_CANCEL_RATE"; then
    cancel_job "$job_id" "queued" "$log_file"
    local dur=$(( $(date +%s) - started_at ))
    echo "$user_idx,$job_id,cancelled_after_queue,$file_bytes,$total_parts,$uploaded_parts,$dur,user_cancelled" >> "$RESULTS_CSV"
    return
  fi

  local poll status_body job_status attempt_p
  for (( attempt_p=1; attempt_p<=MAX_POLL_ATTEMPTS; attempt_p++ )); do
    status_body="$RUN_DIR/tmp-status-$job_id.json"
    poll=$(curl -sS -m 10 -o "$status_body" -w "%{http_code}" "$API_BASE_URL/status/$job_id/update" || echo "000")

    if [[ "$poll" -ge 200 && "$poll" -lt 300 ]]; then
      job_status=$(jq -r '.metadata.status // empty' "$status_body")
      echo "[$(timestamp)] poll job=$job_id attempt=$attempt_p status=$job_status" >> "$log_file"

      if [[ "$job_status" == "completed" ]]; then
        local dur=$(( $(date +%s) - started_at ))
        echo "$user_idx,$job_id,completed,$file_bytes,$total_parts,$uploaded_parts,$dur,ok" >> "$RESULTS_CSV"
        return
      fi
      if [[ "$job_status" == "failed" || "$job_status" == "cancelled" ]]; then
        local dur=$(( $(date +%s) - started_at ))
        echo "$user_idx,$job_id,$job_status,$file_bytes,$total_parts,$uploaded_parts,$dur,server_terminal_state" >> "$RESULTS_CSV"
        return
      fi
    else
      echo "[$(timestamp)] poll_http_error job=$job_id code=$poll" >> "$log_file"
    fi

    sleep "$POLL_INTERVAL_SEC"
  done

  local dur=$(( $(date +%s) - started_at ))
  echo "$user_idx,$job_id,poll_timeout,$file_bytes,$total_parts,$uploaded_parts,$dur,max_poll_attempts_reached" >> "$RESULTS_CSV"
}

pids=()
for (( u=1; u<=USERS; u++ )); do
  run_user "$u" &
  pids+=("$!")
  if (( RAMP_UP_SEC > 0 )); then
    sleep "$(awk -v r="$RANDOM" -v ramp="$RAMP_UP_SEC" -v users="$USERS" \
      'BEGIN { printf "%.3f", (r / 32767.0) * (ramp / users) }')"
  fi
done

for pid in "${pids[@]}"; do
  wait "$pid"
done

summary="$RUN_DIR/summary.txt"
{
  echo "Simulation Summary"
  echo "Run ID: $RUN_ID"
  echo "API Base URL: $API_BASE_URL"
  echo "Users: $USERS"
  echo
  awk -F, 'NR>1 {count[$3]++} END {for (k in count) printf "%s,%d\n", k, count[k]}' "$RESULTS_CSV" | sort
  echo
  awk -F, 'NR>1 {sum+=$7; n++} END {if(n>0) printf "avg_duration_sec,%.2f\n", sum/n; else print "avg_duration_sec,0"}' "$RESULTS_CSV"
} | tee "$summary"

echo
printf "Results: %s\nLogs: %s\nSummary: %s\n" "$RESULTS_CSV" "$RUN_DIR/logs" "$summary"
