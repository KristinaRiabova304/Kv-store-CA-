#!/usr/bin/env bash
# Перевіряє задеплоєний застосунок за specs/delivery і specs/sharding. Код виходу 0 = усе добре.
#   bash scripts/verify-app.sh
# Запускати з машини, чия IP входить в AdminCidr стека kv-app.
set -uo pipefail
REGION="${AWS_REGION:-us-east-1}"
FAIL=0
ok()  { echo "  OK   $*"; }
bad() { echo "  FAIL $*"; FAIL=1; }
out() { aws cloudformation describe-stacks --stack-name "$1" --region "$REGION" \
          --query "Stacks[0].Outputs[?OutputKey=='$2'].OutputValue" --output text; }
param() { aws cloudformation describe-stacks --stack-name "$1" --region "$REGION" \
          --query "Stacks[0].Parameters[?ParameterKey=='$2'].ParameterValue" --output text; }

URL=$(out kv-app CoordinatorUrl); WANT=$(out kv-app ReleaseVersion)
echo "Coordinator $URL, очікувана версія $WANT"

# після деплою сервери перезапускаються; чекаю до 3 хв, поки coordinator і обидва шарди готові
for i in $(seq 1 36); do
  SHARDS=$(curl -s --max-time 3 "$URL/internal/shards" | grep -o '"id"' | wc -l | tr -d ' ')
  [ "${SHARDS:-0}" -ge 2 ] && break
  sleep 5
done
[ "${SHARDS:-0}" -eq 2 ] && ok "2 shards registered" || bad "registered shards: ${SHARDS:-0}"

GOT=$(curl -s --max-time 3 "$URL/version" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
[ "$GOT" = "$WANT" ] && ok "coordinator runs $GOT" || bad "coordinator runs '$GOT', expected $WANT"

# CRUD через coordinator; ім'я таблиці щоразу нове, бо повторна назва дає 409
T="verify$(date +%s)"
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$@"; }
BODY='{"name":"'$T'","partitionKey":"id"}'
[ "$(code -X POST "$URL/tables" -d "$BODY")" = 201 ] && ok "table $T created" || bad "create table"
[ "$(code -X PUT "$URL/tables/$T/items/k1" -d '{"v":1}')" = 200 ] && ok "CREATE" || bad "CREATE"
[ "$(code -I "$URL/tables/$T/items/k1")" = 200 ] && ok "EXISTS" || bad "EXISTS"
[ "$(code "$URL/tables/$T/items/k1")" = 200 ] && ok "READ" || bad "READ"
[ "$(code -X DELETE "$URL/tables/$T/items/k1")" = 204 ] && ok "DELETE" || bad "DELETE"
[ "$(code -I "$URL/tables/$T/items/k1")" = 404 ] && ok "gone after DELETE" || bad "still exists after DELETE"

# правила coordinator-sg: рівно три, як в ADR-0009 після AI-рев'ю
COORD_SG=$(out kv-network CoordinatorSecurityGroupId); EDGE_SG=$(out kv-network EdgeSecurityGroupId)
SHARD_SG=$(out kv-network ShardSecurityGroupId); ADMIN=$(param kv-app AdminCidr)
RULES=$(aws ec2 describe-security-group-rules --region "$REGION" --filters "Name=group-id,Values=$COORD_SG" \
  --query 'SecurityGroupRules[?!IsEgress].[FromPort, ReferencedGroupInfo.GroupId || CidrIpv4]' --output text \
  | awk '{print $1, $2}' | sort | tr '\n' ';')
EXPECTED=$(printf '%s\n' "8080 $EDGE_SG" "8081 $SHARD_SG" "8080 $ADMIN" | sort | tr '\n' ';')
[ "$RULES" = "$EXPECTED" ] && ok "coordinator-sg: 8080 edge-sg and $ADMIN, 8081 shard-sg only" || bad "coordinator-sg rules: $RULES"

# реєстрація шардів недоступна з клієнтського порту (AI-рев'ю M2, пункт 4)
[ "$(code -X POST "$URL/internal/shards" -d '{"id":"x","addr":"1.2.3.4:9090","bootId":"x"}')" = 405 ] \
  && ok "shard registration refused on 8080" || bad "shard registration accepted on 8080"

[ $FAIL -eq 0 ] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED"
exit $FAIL
