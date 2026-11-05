#!/usr/bin/env bash
# Виводить квоти сервісів, які можуть обмежити M2.
#   bash scripts/check-quotas.sh | tee docs/evidence/quotas-$(date +%Y%m%d).txt
set -uo pipefail
REGION="${AWS_REGION:-us-east-1}"

q() {  # код сервісу, частина назви квоти
  sleep 2   # пауза, бо Service Quotas повертає TooManyRequestsException на часті запити
  aws service-quotas list-service-quotas --service-code "$1" --region "$REGION" \
    --query "Quotas[?contains(QuotaName, '$2')].[ServiceCode,QuotaName,Value]" --output text 2>&1
}

echo "Сервіс | Квота | Значення   регіон $REGION, $(date -u)"
q ec2 "Running On-Demand Standard"
q ec2 "EC2-VPC Elastic IPs"
q vpc "VPCs per Region"
q vpc "Subnets per VPC"
q vpc "Internet gateways per Region"
q vpc "NAT gateways per Availability Zone"
q vpc "Inbound or outbound rules per security group"
q elasticloadbalancing "Application Load Balancers per Region"
q lambda "Concurrent executions"
q dynamodb "Maximum number of tables"
q cloudformation "Stack count"
echo
echo "Поточне використання:"
echo -n "VPCs: ";            aws ec2 describe-vpcs --region "$REGION" --query 'length(Vpcs)'
echo -n "Запущені сервери: "; aws ec2 describe-instances --region "$REGION" \
  --filters Name=instance-state-name,Values=running --query 'length(Reservations[].Instances[])'
echo -n "Elastic IPs: ";     aws ec2 describe-addresses --region "$REGION" --query 'length(Addresses)'
