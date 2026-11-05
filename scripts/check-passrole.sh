#!/usr/bin/env bash
# Перевіряє dry-run, чи можна запустити сервер з наявним instance profile. Нічого не створює.
# DryRunOperation означає, що запит дозволено; UnauthorizedOperation або AccessDenied, що ні.
#   bash scripts/check-passrole.sh
set -uo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
OUT="docs/evidence/passrole-dry-run.txt"
AMI="resolve:ssm:/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
mkdir -p docs/evidence
{
  for PROFILE in myS3Role EMR_EC2_DefaultRole; do
    echo "\$ aws ec2 run-instances --dry-run --instance-type t3.micro --iam-instance-profile Name=$PROFILE"
    aws ec2 run-instances --dry-run --image-id "$AMI" --instance-type t3.micro \
      --iam-instance-profile "Name=$PROFILE" --region "$REGION" 2>&1
    echo
  done
} | tee "$OUT"
echo "Збережено в $OUT"
