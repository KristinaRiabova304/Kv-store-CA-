#!/bin/bash
# Перший запуск сервера. Його завантажує user data з S3, див. infra/app.yaml і ADR-0010.
#   bootstrap.sh coordinator <bucket> <region> <expected-shards>
#   bootstrap.sh shard <bucket> <region> <shard-id> <coordinator-internal-url>
# Ставить службу systemd kvstore. Вона при КОЖНОМУ старті сервера читає версію з поточної
# user data і завантажує збірку цієї версії. Тому деплой - це зміна user data і перезапуск сервера.
set -euo pipefail
ROLE="$1"; BUCKET="$2"; REGION="$3"
EXPECTED_SHARDS=0; SHARD_ID=""; COORDINATOR_URL=""
if [ "$ROLE" = coordinator ]; then EXPECTED_SHARDS="${4:-0}"; else SHARD_ID="$4"; COORDINATOR_URL="$5"; fi

mkdir -p /etc/kvstore
cat > /etc/kvstore/static.env <<EOF
ROLE=$ROLE
BUCKET=$BUCKET
REGION=$REGION
SHARD_ID=$SHARD_ID
COORDINATOR_URL=$COORDINATOR_URL
EXPECTED_SHARDS=$EXPECTED_SHARDS
EOF

cat > /usr/local/bin/kvstore-fetch <<'EOF'
#!/bin/bash
# Читає KV_VERSION з user data через IMDSv2 і завантажує бінарник цієї версії з S3.
set -euo pipefail
. /etc/kvstore/static.env
TOKEN=$(curl -sSf -X PUT http://169.254.169.254/latest/api/token -H "X-aws-ec2-metadata-token-ttl-seconds: 300")
md() { curl -sSf -H "X-aws-ec2-metadata-token: $TOKEN" "http://169.254.169.254/latest/$1"; }
VERSION=$(md user-data | sed -n 's/^# KV_VERSION=//p')
LOCAL_IP=$(md meta-data/local-ipv4)
curl -sSf --retry 20 --retry-all-errors -o /usr/local/bin/kvstore.new \
  "https://$BUCKET.s3.$REGION.amazonaws.com/releases/$VERSION/$ROLE"
chmod +x /usr/local/bin/kvstore.new
mv /usr/local/bin/kvstore.new /usr/local/bin/kvstore
mkdir -p /run/kvstore
echo "ADVERTISE_ADDR=$LOCAL_IP:9090" > /run/kvstore/dynamic.env
echo "kvstore-fetch: $ROLE version $VERSION"
EOF
chmod +x /usr/local/bin/kvstore-fetch

cat > /etc/systemd/system/kvstore.service <<'EOF'
[Unit]
Description=kv-store node
Wants=network-online.target
After=network-online.target

[Service]
# EnvironmentFile читається перед кожним процесом, тому ExecStart уже бачить файл, записаний ExecStartPre
EnvironmentFile=/etc/kvstore/static.env
EnvironmentFile=-/run/kvstore/dynamic.env
# "+" - завантаження з правами root, сам сервер - від тимчасового користувача без прав (DynamicUser)
ExecStartPre=+/usr/local/bin/kvstore-fetch
ExecStart=/usr/local/bin/kvstore
Restart=always
RestartSec=3
DynamicUser=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now kvstore.service
