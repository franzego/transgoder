# Realistic Load Simulation Scripts

These scripts simulate messy real-world usage against the transcoder API:

- concurrent users (10 or 50)
- large multipart uploads
- random flaky client behavior
- random upload timeouts
- random user-abandon flows
- random job cancellations mid-upload and after queueing

## Files

- `scripts/load/simulate_load.sh`: main scenario runner
- `scripts/load/run_10_users.sh`: convenience wrapper for 10 users
- `scripts/load/run_50_users.sh`: convenience wrapper for 50 users

## Requirements

- `curl`
- `jq`
- `awk`
- `sed`
- `head`

## Quick Start

```bash
# 10 users
scripts/load/run_10_users.sh

# 50 users
scripts/load/run_50_users.sh
```

## Useful Overrides

```bash
API_BASE_URL=http://localhost:8080 \
MIN_FILE_MB=512 \
MAX_FILE_MB=4096 \
PART_SIZE_MB=64 \
FLAKE_RATE=20 \
TIMEOUT_RATE=15 \
CANCEL_RATE=25 \
ABANDON_RATE=10 \
POST_COMPLETE_CANCEL_RATE=12 \
scripts/load/run_50_users.sh
```

## Notes

- Default file sizes are intentionally large to create realistic pressure.
- Results are written to `scripts/load/runs/<timestamp>/results.csv`.
- Per-user logs are written to `scripts/load/runs/<timestamp>/logs/`.
- Summary metrics are in `scripts/load/runs/<timestamp>/summary.txt`.
