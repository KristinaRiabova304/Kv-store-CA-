#!/usr/bin/env bash
# Деплой усього M2 по черзі: мережа → бакет збірок → збірка → застосунок. Те саме робить CI.
#   bash scripts/deploy-all.sh
#   ADMIN_CIDR=1.2.3.4/32 bash scripts/deploy-all.sh   хто може звертатися до coordinator
set -euo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"

if [ -z "${ADMIN_CIDR:-}" ]; then
  [ -n "${CI:-}" ] && { echo "У CI задай ADMIN_CIDR, інакше доступ отримає IP раннера GitHub" >&2; exit 1; }
  ADMIN_CIDR="$(curl -sSf https://checkip.amazonaws.com | tr -d '[:space:]')/32"
  echo "ADMIN_CIDR не задано, беру свою IP: $ADMIN_CIDR"
fi

# ключі сесії sandbox живуть ~4 год; перевіряю їх одразу, щоб помилка була зрозумілою
if ! aws sts get-caller-identity --region "$REGION" >/dev/null 2>&1; then
  echo "Ключі AWS недійсні. Запусти лабу і онови їх: bash scripts/set-ci-creds.sh" >&2
  exit 2
fi

echo "== kv-network"
bash scripts/deploy.sh

echo "== kv-artifacts"
aws cloudformation deploy --stack-name kv-artifacts --template-file infra/artifacts.yaml \
  --region "$REGION" --tags project=kvstore milestone=m2 --no-fail-on-empty-changeset

echo "== збірка"
VERSION=$(bash scripts/release.sh)

echo "== kv-app, версія $VERSION"
aws cloudformation deploy --stack-name kv-app --template-file infra/app.yaml \
  --region "$REGION" --tags project=kvstore milestone=m2 --no-fail-on-empty-changeset \
  --parameter-overrides "ReleaseVersion=$VERSION" "AdminCidr=$ADMIN_CIDR"

aws cloudformation describe-stacks --stack-name kv-app --region "$REGION" \
  --query 'Stacks[0].Outputs' --output table
