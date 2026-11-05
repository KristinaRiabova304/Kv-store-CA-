#!/usr/bin/env bash
# Валідує IAM-шаблон і НАМАГАЄТЬСЯ його задеплоїти. У sandbox деплой має
# ВПАСТИ з AccessDenied - саме ця помилка і є потрібним доказом.
#   bash scripts/try-iam-stack.sh
set -uo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
OUT="docs/evidence/iam-deploy-$(date +%Y%m%d-%H%M%S).txt"
mkdir -p docs/evidence
{
  echo "\$ aws cloudformation validate-template --template-body file://infra/iam-readonly.yaml"
  aws cloudformation validate-template --template-body file://infra/iam-readonly.yaml --region "$REGION" 2>&1
  echo
  echo "\$ aws cloudformation deploy --stack-name kv-iam-readonly --capabilities CAPABILITY_NAMED_IAM ..."
  aws cloudformation deploy --stack-name kv-iam-readonly --template-file infra/iam-readonly.yaml \
    --capabilities CAPABILITY_NAMED_IAM --region "$REGION" 2>&1
  echo
  echo "\$ aws cloudformation describe-stack-events, чому впало"
  aws cloudformation describe-stack-events --stack-name kv-iam-readonly --region "$REGION" \
    --query 'StackEvents[?contains(ResourceStatus,`FAILED`)].[LogicalResourceId,ResourceStatusReason]' --output table 2>&1
} | tee "$OUT"
echo "Збережено в $OUT"
echo "Прибрати невдалий стек:  aws cloudformation delete-stack --stack-name kv-iam-readonly"
