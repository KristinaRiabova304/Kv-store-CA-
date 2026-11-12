#!/usr/bin/env bash
# Видаляє все з M2 у зворотному порядку: застосунок → бакет збірок → мережа.
# Запускати в кінці кожної робочої сесії, див. docs/budget.md.
#   bash scripts/teardown-all.sh
set -euo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"

delete() {
  aws cloudformation describe-stacks --stack-name "$1" --region "$REGION" >/dev/null 2>&1 || { echo "$1: немає"; return; }
  echo "Видаляю $1 ..."
  aws cloudformation delete-stack --stack-name "$1" --region "$REGION"
  aws cloudformation wait stack-delete-complete --stack-name "$1" --region "$REGION"
}

delete kv-bench
delete kv-app

# CloudFormation не видаляє бакет, у якому щось лежить, тому спершу очищую його
BUCKET=$(aws cloudformation describe-stacks --stack-name kv-artifacts --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='BucketName'].OutputValue" --output text 2>/dev/null || true)
[ -n "$BUCKET" ] && [ "$BUCKET" != "None" ] && aws s3 rm "s3://$BUCKET" --recursive --region "$REGION" --only-show-errors
delete kv-artifacts

STACK=kv-network bash scripts/teardown.sh
