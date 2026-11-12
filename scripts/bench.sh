#!/usr/bin/env bash
# Бенчмарк YCSB у sandbox: створює стек kv-bench з клієнтом, чекає на результати в S3,
# завантажує їх у bench/results/<run>/ і видаляє стек.
#   bash scripts/bench.sh
#   RECORDS=100000 OPS=200000 THREADS=16 REPEATS=3 bash scripts/bench.sh
# Потрібні задеплоєні kv-network, kv-artifacts і kv-app, а збірка має містити bench/.
set -euo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
RUN="${RUN:-$(date -u +%Y%m%d-%H%M%S)}"
out() { aws cloudformation describe-stacks --stack-name "$1" --region "$REGION" \
          --query "Stacks[0].Outputs[?OutputKey=='$2'].OutputValue" --output text; }

VERSION=$(out kv-app ReleaseVersion)
COORD="http://$(out kv-app CoordinatorPrivateIp):8080"
BUCKET=$(out kv-artifacts BucketName)
echo "Прогін $RUN: версія $VERSION, coordinator $COORD"

aws cloudformation deploy --stack-name kv-bench --template-file infra/bench.yaml \
  --region "$REGION" --tags project=kvstore milestone=m2 \
  --parameter-overrides "ReleaseVersion=$VERSION" "CoordinatorUrl=$COORD" "RunId=$RUN" \
    "RecordCount=${RECORDS:-100000}" "OperationCount=${OPS:-200000}" "Threads=${THREADS:-16}" "Repeats=${REPEATS:-3}"

# ID серверів зберігаю одразу: клієнт після прогону видаляється, а метрики CloudWatch про нього лишаються
mkdir -p "bench/results/$RUN"
res() { aws cloudformation describe-stack-resource --stack-name "$1" --logical-resource-id "$2" --region "$REGION" \
          --query StackResourceDetail.PhysicalResourceId --output text; }
{ echo "coordinator $(res kv-app Coordinator)"; echo "shard-1 $(res kv-app Shard1)"
  echo "shard-2 $(res kv-app Shard2)"; echo "client $(res kv-bench BenchClient)"; } > "bench/results/$RUN/instances.txt"

# стек видаляю за будь-якого результату, щоб клієнт не лишився працювати
trap 'echo "Видаляю kv-bench ..."; aws cloudformation delete-stack --stack-name kv-bench --region "$REGION"' EXIT

echo "Чекаю на результати, до 30 хв ..."
for i in $(seq 1 120); do
  aws s3 ls "s3://$BUCKET/bench-results/$RUN/DONE" --region "$REGION" >/dev/null 2>&1 && break
  sleep 15
done
aws s3 cp "s3://$BUCKET/bench-results/$RUN/" "bench/results/$RUN/" --recursive --region "$REGION" --only-show-errors
[ -f "bench/results/$RUN/DONE" ] || { echo "Результатів немає за 30 хв" >&2; exit 1; }
rm "bench/results/$RUN/DONE"

echo; cat "bench/results/$RUN/info.txt"
# у кожному файлі беру лише підсумкові рядки: вони останні для кожної операції
for f in "bench/results/$RUN"/load.txt "bench/results/$RUN"/run-*.txt; do
  echo "== $(basename "$f" .txt)"
  grep -E '^(INSERT|READ|UPDATE|TOTAL|[A-Z]+-ERROR)' "$f" | awk '{a[$1]=$0} END {for (k in a) print a[k]}' | sort || true
done
echo "== розподіл після load: $(cat "bench/results/$RUN/distribution.json")"
echo
echo "Метрики CPU з'являються в CloudWatch із затримкою до 10 хв. Потім: bash scripts/bench-metrics.sh $RUN"
