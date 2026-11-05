#!/usr/bin/env bash
# Збирає докази про ідентичність sandbox і LabRole.
# Запускаю в AWS CloudShell або будь-де, де є ключі sandbox:
#   bash scripts/dissect-labrole.sh
# Результат зберігається в docs/evidence/labrole-<час>.txt
set -u
OUT_DIR="$(dirname "$0")/../docs/evidence"
mkdir -p "$OUT_DIR"
OUT="$OUT_DIR/labrole-$(date +%Y%m%d-%H%M%S).txt"
REGION="${AWS_REGION:-us-east-1}"

run() {   # пише команду і її вивід або помилку у файл доказів
  echo "=================================================="
  echo "\$ $*"
  echo "--------------------------------------------------"
  "$@" 2>&1
  echo "код виходу: $?"
  echo
}

{
  echo "# Докази щодо LabRole, $(date -u)"
  echo

  echo "## 1. Хто я? Ідентичність, яку використовують консоль і CLI"
  run aws sts get-caller-identity

  echo "## 2. Ролі акаунта і сама LabRole"
  run aws iam list-roles --query 'Roles[].[RoleName,Path]' --output table
  run aws iam get-role --role-name LabRole
  run aws iam list-attached-role-policies --role-name LabRole
  run aws iam list-role-policies --role-name LabRole
  run aws iam list-instance-profiles-for-role --role-name LabRole

  echo "## 3. Вміст кожної прикріпленої managed policy"
  for arn in $(aws iam list-attached-role-policies --role-name LabRole \
                 --query 'AttachedPolicies[].PolicyArn' --output text 2>/dev/null); do
    ver=$(aws iam get-policy --policy-arn "$arn" \
            --query 'Policy.DefaultVersionId' --output text 2>/dev/null)
    run aws iam get-policy-version --policy-arn "$arn" --version-id "$ver"
  done

  echo "## 4. Inline-політики"
  for name in $(aws iam list-role-policies --role-name LabRole \
                  --query 'PolicyNames[]' --output text 2>/dev/null); do
    run aws iam get-role-policy --role-name LabRole --policy-name "$name"
  done

  echo "## 5. Policy simulator"
  # Якщо LabRole немає, симулюю роль, під якою працюю сама, наприклад voclabs
  ROLE_ARN=$(aws iam get-role --role-name LabRole --query Role.Arn --output text 2>/dev/null)
  if [ -z "$ROLE_ARN" ]; then
    ROLE_ARN=$(aws sts get-caller-identity --query Arn --output text \
      | sed -E 's#arn:aws:sts::([0-9]+):assumed-role/([^/]+)/.*#arn:aws:iam::\1:role/\2#')
    echo "LabRole не знайдено, симулюю $ROLE_ARN"
  fi
  run aws iam simulate-principal-policy --policy-source-arn "$ROLE_ARN" \
      --action-names \
        ec2:CreateVpc ec2:RunInstances ec2:CreateNatGateway \
        s3:CreateBucket dynamodb:CreateTable lambda:CreateFunction \
        cloudformation:CreateStack \
        iam:CreateRole iam:CreatePolicy iam:AttachRolePolicy iam:CreateUser iam:PassRole \
        organizations:CreateAccount ce:GetCostAndUsage \
      --query 'EvaluationResults[].[EvalActionName,EvalDecision]' --output table

  echo "## 6. Спроби й помилки з ПОТОЧНОЮ ідентичністю, нешкідливі dry-run"
  run aws ec2 run-instances --dry-run --image-id resolve:ssm:/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
      --instance-type t3.micro --region "$REGION"
  run aws ec2 run-instances --dry-run --image-id resolve:ssm:/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
      --instance-type m5.4xlarge --region "$REGION"
  run aws iam create-policy --policy-name kvstore-probe \
      --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:DescribeVpcs","Resource":"*"}]}'
  run aws iam create-role --role-name kvstore-probe \
      --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
  run aws iam create-user --user-name kvstore-probe
  run aws ec2 describe-regions --query 'Regions[].RegionName' --output text
  run aws ec2 describe-vpcs --region us-west-2 --query 'Vpcs[].VpcId'
  run aws ec2 describe-vpcs --region eu-central-1 --query 'Vpcs[].VpcId'
} > "$OUT"

echo "Докази збережено в $OUT"
echo "Якщо якась спроба випадково ВДАЛАСЯ, треба прибрати за собою:"
echo "  aws iam delete-policy --policy-arn arn:aws:iam::<account>:policy/kvstore-probe"
echo "  aws iam delete-role --role-name kvstore-probe ; aws iam delete-user --user-name kvstore-probe"
