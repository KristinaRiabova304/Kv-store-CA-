#!/usr/bin/env bash
# Деплой мережевого стеку: створення або оновлення.
#   bash scripts/deploy.sh
set -euo pipefail
cd "$(dirname "$0")/.."
STACK="${STACK:-kv-network}"
REGION="${AWS_REGION:-us-east-1}"

aws cloudformation validate-template --template-body file://infra/network.yaml --region "$REGION" >/dev/null
echo "Шаблон валідний. Деплою $STACK ..."

aws cloudformation deploy \
  --stack-name "$STACK" \
  --template-file infra/network.yaml \
  --region "$REGION" \
  --tags project=kvstore milestone=m1 \
  --no-fail-on-empty-changeset

aws cloudformation describe-stacks --stack-name "$STACK" --region "$REGION" \
  --query 'Stacks[0].Outputs' --output table
