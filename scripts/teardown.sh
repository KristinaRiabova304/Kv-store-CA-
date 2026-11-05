#!/usr/bin/env bash
# Видаляє мережевий стек і чекає, поки його не стане.
#   bash scripts/teardown.sh
set -euo pipefail
STACK="${STACK:-kv-network}"
REGION="${AWS_REGION:-us-east-1}"

aws cloudformation delete-stack --stack-name "$STACK" --region "$REGION"
echo "Чекаю, поки $STACK видалиться ..."
aws cloudformation wait stack-delete-complete --stack-name "$STACK" --region "$REGION"
echo "Видалено."
