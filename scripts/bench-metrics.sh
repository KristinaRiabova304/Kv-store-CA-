#!/usr/bin/env bash
# CPU і CPU-кредити всіх серверів за час прогону бенчмарку, з CloudWatch (AI-рев'ю M2, пункти 7 і 8).
# Показує, хто був вузьким місцем: coordinator, шарди чи сам клієнт.
#   bash scripts/bench-metrics.sh <run-id>
# Запускати через 10 хв після прогону: базовий моніторинг EC2 публікує дані раз на 5 хв і з затримкою.
set -euo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
RUN="$1"; DIR="bench/results/$RUN"
START=$(sed -n 's/^started: *//p' "$DIR/info.txt"); END=$(sed -n 's/^finished: *//p' "$DIR/info.txt")
# вікно розширюю на 5 хв з кожного боку, бо точки CloudWatch - п'ятихвилинні
FROM=$(date -u -j -v-5M -f %Y-%m-%dT%H:%M:%SZ "$START" +%FT%TZ 2>/dev/null || date -u -d "$START - 5 min" +%FT%TZ)
TO=$(date -u -j -v+5M -f %Y-%m-%dT%H:%M:%SZ "$END" +%FT%TZ 2>/dev/null || date -u -d "$END + 5 min" +%FT%TZ)

metric() {  # $1 = id сервера, $2 = метрика, $3 = статистика
  aws cloudwatch get-metric-statistics --region "$REGION" --namespace AWS/EC2 --metric-name "$2" \
    --dimensions Name=InstanceId,Value="$1" --start-time "$FROM" --end-time "$TO" --period 300 \
    --statistics "$3" --query "sort_by(Datapoints,&Timestamp)[].[Timestamp,$3]" --output text \
    | awk '{printf "%s %.1f; ", substr($1,12,5), $2}'
}

{
  echo "Прогін $RUN, $START - $END, вікно CloudWatch $FROM - $TO, точки по 5 хв (UTC)"
  while read -r name id; do
    echo "$name ($id)"
    echo "  CPUUtilization max, %: $(metric "$id" CPUUtilization Maximum)"
    echo "  CPUCreditBalance min:   $(metric "$id" CPUCreditBalance Minimum)"
  done < "$DIR/instances.txt"
} | tee "$DIR/cpu.txt"
