#!/usr/bin/env bash
# Демо M2 на захисті. Адресу coordinator беру зі стека kv-app, перед кожним кроком чекаю Enter.
#   bash scripts/demo.sh                              в AWS
#   URL=http://localhost:8080 bash scripts/demo.sh    локально через docker compose
# Перед цим: bash scripts/deploy-all.sh з тієї ж мережі, звідки показую (доступ лише з моєї IP).
set -uo pipefail
cd "$(dirname "$0")/.."
REGION="${AWS_REGION:-us-east-1}"
# локально: URL=http://localhost:8080 bash scripts/demo.sh (після docker compose up)
URL="${URL:-$(aws cloudformation describe-stacks --stack-name kv-app --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='CoordinatorUrl'].OutputValue" --output text)}"
T="orders$(date +%H%M%S)"   # нова таблиця щоразу, щоб повторний запуск не давав 409

step() {
  echo
  echo "------------------------------------------------------------"
  echo "# $1"
  echo "\$ $2"
  read -r -p "[Enter] " _
  eval "$2"
  echo
}

echo "Coordinator: $URL"
step "Шарди: обидва зареєстровані, bootId, перезапуски" "curl -s $URL/internal/shards"
step "Реєструю таблицю з partition key і sort key" \
  "curl -s -X POST $URL/tables -d '{\"name\":\"$T\",\"partitionKey\":\"userId\",\"sortKey\":\"date\"}'"
step "CREATE: три замовлення користувача u1" \
  "for d in 2026-10-31 2026-11-10 2026-11-20; do curl -s -X PUT $URL/tables/$T/items/u1/\$d -d '{\"total\":42}'; echo; done"
step "EXISTS: HEAD, є запис - 200" "curl -s -o /dev/null -w '%{http_code}\n' -I $URL/tables/$T/items/u1/2026-11-10"
step "READ одного запису" "curl -s $URL/tables/$T/items/u1/2026-11-10"
step "Діапазон за sort key: лише листопад, відсортовано, з одного шарда" \
  "curl -s '$URL/tables/$T/items/u1?from=2026-11-01&to=2026-11-30'"
step "DELETE і повторний EXISTS - 404" \
  "curl -s -o /dev/null -w 'DELETE %{http_code}\n' -X DELETE $URL/tables/$T/items/u1/2026-11-10; curl -s -o /dev/null -w 'HEAD %{http_code}\n' -I $URL/tables/$T/items/u1/2026-11-10"
step "Пишу 1000 ключів і дивлюся, як вони розійшлися по шардах" \
  "curl -s -o /dev/null -X POST $URL/tables -d '{\"name\":\"users$T\",\"partitionKey\":\"id\"}'; for i in \$(seq 1 1000); do curl -s -o /dev/null -X PUT $URL/tables/users$T/items/user\$i -d '{\"x\":1}'; done; curl -s $URL/internal/distribution"
step "Захист: реєстрація шарда з клієнтського порту відхиляється - 405" \
  "curl -s -o /dev/null -w '%{http_code}\n' -X POST $URL/internal/shards -d '{\"id\":\"evil\",\"addr\":\"1.2.3.4:9090\",\"bootId\":\"x\"}'"
step "Тести кільця: розподіл 51/49% і 36% переїзду проти 67% у hash % N" \
  "go test -v -run 'TestBalance|TestMovementOnAdd' ./internal/ring/ | grep -E 'shard-|переїхало|PASS|FAIL'"
echo "Кінець демо."
