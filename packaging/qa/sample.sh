#!/usr/bin/env bash
# Sample one running mumu process for a soak or a stress run.
#
# Usage:
#   packaging/qa/sample.sh <pid> <seconds> <outdir> [interval]
#
# Writes <outdir>/samples.csv and <outdir>/summary.txt. It measures the process
# only: resident memory, CPU percentage and thread count. GPU and energy are
# system-wide on macOS and need powermetrics; record those separately.

set -euo pipefail

pid="${1:?usage: sample.sh <pid> <seconds> <outdir> [interval]}"
seconds="${2:?usage: sample.sh <pid> <seconds> <outdir> [interval]}"
outdir="${3:?usage: sample.sh <pid> <seconds> <outdir> [interval]}"
interval="${4:-5}"

mkdir -p "$outdir"
csv="$outdir/samples.csv"
echo "elapsed_s,rss_mib,cpu_percent,threads" >"$csv"
: >"$outdir/summary.txt"

start="$(date +%s)"
while :; do
  now="$(date +%s)"
  elapsed=$((now - start))
  (( elapsed >= seconds )) && break

  if ! line="$(ps -p "$pid" -o rss=,%cpu= 2>/dev/null)"; then
    echo "sample: process $pid exited after ${elapsed}s" | tee -a "$outdir/summary.txt"
    break
  fi

  rss="$(echo "$line" | awk '{printf "%.1f", $1/1024}')"
  cpu="$(echo "$line" | awk '{print $2}')"
  threads="$(ps -M -p "$pid" 2>/dev/null | tail -n +2 | wc -l | tr -d ' ')"
  echo "$elapsed,$rss,$cpu,$threads" >>"$csv"
  sleep "$interval"
done

if [[ -s "$csv" ]] && [[ "$(wc -l <"$csv" | tr -d ' ')" -gt 1 ]]; then
  awk -F, 'NR>1 {
    n++; rss+=$2; cpu+=$3;
    if ($2>maxrss) maxrss=$2;
    if ($3>maxcpu) maxcpu=$3;
    last=$1;
  }
  END {
    printf "samples=%d duration_s=%d\n", n, last;
    printf "rss_mib_mean=%.1f rss_mib_max=%.1f\n", rss/n, maxrss;
    printf "cpu_percent_mean=%.1f cpu_percent_max=%.1f\n", cpu/n, maxcpu;
  }' "$csv" >>"$outdir/summary.txt"
fi

cat "$outdir/summary.txt"
