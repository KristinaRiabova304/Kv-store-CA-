#!/usr/bin/env bash
# Збирає coordinator і шард для Linux і кладе їх разом з bootstrap.sh у бакет збірок.
#   bash scripts/release.sh            версія = поточний коміт
# Друкує версію останнім рядком.
set -euo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
VERSION="${VERSION:-$(git rev-parse --short=12 HEAD)}"
BUCKET=$(aws cloudformation describe-stacks --stack-name kv-artifacts --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='BucketName'].OutputValue" --output text)

rm -rf dist && mkdir -p dist
# t3.micro - x86_64; CGO вимкнено, щоб бінарник не залежав від бібліотек сервера
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-s -w -X kvstore/internal/buildinfo.Version=$VERSION" -o dist/ ./cmd/...
cp deploy/bootstrap.sh dist/
# клієнт YCSB і сценарії для стека kv-bench; bench/ycsb - окремий Go-модуль
mkdir -p dist/bench
(cd bench/ycsb && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o ../../dist/bench/kvstore-ycsb .)
cp bench/run-bench.sh bench/workloads/workloada bench/workloads/workloadc dist/bench/

aws s3 cp dist/ "s3://$BUCKET/releases/$VERSION/" --recursive --region "$REGION" --only-show-errors >&2
echo "Збірка $VERSION у s3://$BUCKET/releases/$VERSION/" >&2
echo "$VERSION"
