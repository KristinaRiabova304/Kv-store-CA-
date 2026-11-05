#!/usr/bin/env bash
# Демонструє change set, тобто попередній перегляд перед застосуванням, і drift detection.
#   bash scripts/demo-changeset-drift.sh
set -euo pipefail
cd "$(dirname "$0")/.."
STACK="${STACK:-kv-network}"
REGION="${AWS_REGION:-us-east-1}"

echo "### ЧАСТИНА 1: CHANGE SET - що ЗМІНИЛОСЯ Б, якби я застосувала нову версію шаблону"
# Роблю змінену копію шаблону: міняю тег Name у VPC.
sed 's/"${ProjectName}-vpc"/"${ProjectName}-vpc-v2"/' infra/network.yaml > /tmp/network-v2.yaml
CS=demo-$(date +%s)
aws cloudformation create-change-set --stack-name "$STACK" --change-set-name "$CS" \
  --template-body file:///tmp/network-v2.yaml --region "$REGION" >/dev/null
aws cloudformation wait change-set-create-complete --stack-name "$STACK" --change-set-name "$CS" --region "$REGION"
aws cloudformation describe-change-set --stack-name "$STACK" --change-set-name "$CS" --region "$REGION" \
  --query 'Changes[].ResourceChange.[Action,LogicalResourceId,ResourceType,Replacement]' --output table
echo "Change set НЕ виконано, я лише переглянула зміни. Видаляю його."
aws cloudformation delete-change-set --stack-name "$STACK" --change-set-name "$CS" --region "$REGION"

echo
echo "### ЧАСТИНА 2: DRIFT - хтось змінив ресурс руками, в обхід CloudFormation"
VPC=$(aws cloudformation describe-stacks --stack-name "$STACK" --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='VpcId'].OutputValue" --output text)
aws ec2 create-tags --resources "$VPC" --tags Key=milestone,Value=edited-by-hand --region "$REGION"
ID=$(aws cloudformation detect-stack-drift --stack-name "$STACK" --region "$REGION" --query StackDriftDetectionId --output text)
until [ "$(aws cloudformation describe-stack-drift-detection-status --stack-drift-detection-id "$ID" \
          --region "$REGION" --query DetectionStatus --output text)" != "DETECTION_IN_PROGRESS" ]; do sleep 3; done
aws cloudformation describe-stack-resource-drifts --stack-name "$STACK" --region "$REGION" \
  --stack-resource-drift-status-filters MODIFIED \
  --query 'StackResourceDrifts[].[LogicalResourceId,StackResourceDriftStatus,PropertyDifferences[].[PropertyPath,ExpectedValue,ActualValue]]' --output json

echo "Усуваю drift: повертаю значення з шаблону"
aws ec2 create-tags --resources "$VPC" --tags Key=milestone,Value=m1 --region "$REGION"
echo "Готово. Зробити скріншоти обох частин для захисту."
