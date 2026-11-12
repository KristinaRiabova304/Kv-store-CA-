#!/usr/bin/env bash
# Кладе ключі поточної сесії sandbox у секрети GitHub, щоб CI міг деплоїти. Див. ADR-0010.
# Запускати на початку кожної сесії лаби: ключі спливають приблизно за 4 години.
#   bash scripts/set-ci-creds.sh
# Ключі беру з ~/.aws/credentials (профіль default), куди їх вставляю з AWS Details → AWS CLI.
# Потрібен gh, у який виконано вхід: gh auth login.
set -euo pipefail
cd "$(dirname "$0")/.."

key()  { aws configure get "$1" --profile "${AWS_PROFILE:-default}"; }
AK=$(key aws_access_key_id); SK=$(key aws_secret_access_key); ST=$(key aws_session_token)
[ -n "$AK" ] && [ -n "$SK" ] && [ -n "$ST" ] || { echo "Немає ключів у профілі ${AWS_PROFILE:-default}" >&2; exit 1; }

# спершу переконуюсь, що ключі дійсні, щоб не класти в GitHub мертві
aws sts get-caller-identity --query Arn --output text

# значення передаю через stdin, а не аргументом, щоб їх не було видно у списку процесів
printf '%s' "$AK" | gh secret set AWS_ACCESS_KEY_ID
printf '%s' "$SK" | gh secret set AWS_SECRET_ACCESS_KEY
printf '%s' "$ST" | gh secret set AWS_SESSION_TOKEN

if [ -n "${ADMIN_CIDR:-}" ]; then
  gh variable set ADMIN_CIDR --body "$ADMIN_CIDR"
else
  MY="$(curl -sSf https://checkip.amazonaws.com | tr -d '[:space:]')/32"
  gh variable set ADMIN_CIDR --body "$MY"
  echo "ADMIN_CIDR = $MY"
fi
echo "Секрети оновлено. Наступний push у main задеплоїть застосунок."
