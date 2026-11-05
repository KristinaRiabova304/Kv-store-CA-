#!/usr/bin/env bash
# Доказ відтворюваності: видалити стек, задеплоїти знову з IaC, перевірити. Із заміром часу.
#   bash scripts/recreate.sh | tee docs/evidence/recreate-$(date +%Y%m%d-%H%M).txt
set -uo pipefail
cd "$(dirname "$0")/.."
t0=$(date +%s)
echo "== Видалення почато  $(date -u)"
bash scripts/teardown.sh
t1=$(date +%s)
echo "== Видалення тривало $((t1 - t0)) с"

echo "== Деплой почато     $(date -u)"
bash scripts/deploy.sh
t2=$(date +%s)
echo "== Деплой тривав $((t2 - t1)) с"

echo "== Перевірка"
bash scripts/verify-network.sh
t3=$(date +%s)
echo "== РАЗОМ $((t3 - t0)) с: видалення $((t1 - t0)) с, деплой $((t2 - t1)) с"
