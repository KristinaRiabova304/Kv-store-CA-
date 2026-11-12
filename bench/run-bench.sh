#!/bin/bash
# Запускається на клієнтському сервері з user data стека kv-bench, див. infra/bench.yaml.
#   run-bench.sh <s3-base-url> <version> <coordinator-url> <run-id> <recordcount> <operationcount> <threads> [repeats]
# SSH немає, тому результати їдуть у S3 через gateway endpoint, а scripts/bench.sh їх забирає.
set -uo pipefail
B="$1"; VERSION="$2"; URL="$3"; RUN="$4"; RC="$5"; OC="$6"; TH="$7"; REPEATS="${8:-3}"
W=/opt/bench; mkdir -p "$W/out"; cd "$W"

dl() { curl -sSf --retry 20 --retry-all-errors -o "$W/$1" "$B/releases/$VERSION/bench/$1"; }
up() { curl -sSf --retry 5 -T "$1" "$B/bench-results/$RUN/$(basename "$1")"; }
dl kvstore-ycsb && dl workloada && dl workloadc && chmod +x kvstore-ycsb

# чекаю, поки coordinator відповідає і бачить обидва шарди
for i in $(seq 1 60); do
  [ "$(curl -s --max-time 3 "$URL/internal/shards" | grep -o '"id"' | wc -l)" -ge 2 ] && break
  sleep 5
done

TOKEN=$(curl -sSf -X PUT http://169.254.169.254/latest/api/token -H "X-aws-ec2-metadata-token-ttl-seconds: 300")
md() { curl -sSf -H "X-aws-ec2-metadata-token: $TOKEN" "http://169.254.169.254/latest/meta-data/$1"; }
{
  echo "run:            $RUN"
  echo "started:        $(date -u +%FT%TZ)"
  echo "release:        $VERSION"
  echo "client:         $(md instance-type) in $(md placement/availability-zone)"
  echo "coordinator:    $URL"
  echo "recordcount:    $RC"
  echo "operationcount: $OC"
  echo "threads:        $TH"
  echo "repeats:        $REPEATS"
  echo "shards:         $(curl -s "$URL/internal/shards")"
} > out/info.txt

ycsb() { ./kvstore-ycsb "$1" -P "$2" -p kvstore.url="$URL" -p recordcount="$RC" -p operationcount="$OC" -threads "$TH"; }

# 1. load: записую recordcount записів; 2. сценарії A і C на тих самих даних, кожен REPEATS разів,
# щоб бачити розкид між прогонами (AI-рев'ю M2, пункт 7)
ycsb load workloada > out/load.txt 2>&1
curl -s "$URL/internal/distribution" > out/distribution.json
for i in $(seq 1 "$REPEATS"); do
  ycsb run workloada > "out/run-a-$i.txt" 2>&1
  ycsb run workloadc > "out/run-c-$i.txt" 2>&1
done
echo "finished:       $(date -u +%FT%TZ)" >> out/info.txt

for f in out/*; do up "$f"; done
echo done > out/DONE && up out/DONE
