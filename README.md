# kv-store - спрощене сховище ключ-значення на зразок DynamoDB

Capstone-проєкт курсу Cloud Architectures 2026/27. Поточний етап: **M2 - шардування**. Coordinator розкладає записи по шардах через consistent hashing.

## Що де лежить
```
openspec/                      специфікації та change proposals, я пишу їх до коду
  changes/m1-infrastructure/   proposal, задачі й специфікації для M1
  changes/m2-sharding/         proposal, задачі й специфікації для M2
api/openapi.yaml               контракт API
cmd/coordinator, cmd/shard     програми на Go
internal/                      store (записи в пам'яті), ring (кільце), coordinator, shard (внутрішній API)
infra/
  network.yaml                 VPC, 2 публічні та 2 приватні підмережі, IGW, S3 endpoint, таблиці маршрутів, security groups
  iam-readonly.yaml            політика тільки на перегляд і роль, лише як код, sandbox її не створює
  artifacts.yaml               S3-бакет для збірок, M2
  app.yaml                     coordinator і 2 шарди на EC2, M2
deploy/bootstrap.sh            перший запуск сервера: ставить службу systemd
.github/workflows/ci.yaml      CI/CD
scripts/                       деплой, видалення, перевірка, пересоздання, збір доказів
docs/
  adr/                         мої архітектурні рішення
  labrole.md                   що дозволяє і забороняє sandbox та як я це з'ясувала
  iam-readonly.md              що зробив би IAM-шаблон і чому sandbox його не приймає
  quotas.md                    квоти, які можуть заважати в M2
  budget.md                    бюджет M2 порівняно з кредитом sandbox
  recreatability.md            доказ, що мережу можна знести й відбудувати з коду
  ai-critique.md               критика архітектури від AI і мої відповіді
  evidence/                    сирі виводи команд і скріншоти
```

## Архітектура M1
```
                    Інтернет
                        │
                 Internet Gateway
                        │
 VPC 10.20.0.0/16 ──────┼─────────────────────────────
   AZ #1                │              AZ #2
   public-a 10.20.0.0/24 ─┴─ public-rt ─ public-b 10.20.1.0/24
        0.0.0.0/0 → IGW, local
   private-a 10.20.10.0/24 ── private-rt ── private-b 10.20.11.0/24
        лише local, маршруту в інтернет немає
```

Security groups, кожна пускає лише попередню ланку:
```
інтернет --80--> edge-sg --8080--> coordinator-sg --9090--> shard-sg        SSH 22 закритий скрізь
```

## M2: як запустити локально
```bash
go test -race ./...                 # усі тести, зокрема інтеграційний: coordinator і 2 шарди
docker compose up --build           # або без Docker: див. змінні середовища в cmd/*/main.go
curl -X POST localhost:8080/tables -d '{"name":"users","partitionKey":"id"}'
curl -X PUT  localhost:8080/tables/users/items/u1 -d '{"name":"Ann"}'
curl -I      localhost:8080/tables/users/items/u1     # EXISTS
curl         localhost:8080/tables/users/items/u1     # READ
curl -X DELETE localhost:8080/tables/users/items/u1   # DELETE
curl         localhost:8080/internal/distribution     # скільки записів на кожному шарді
curl         localhost:8080/internal/shards           # шарди, bootId, перезапуски
```
Coordinator слухає два порти: 8080 для клієнтів і 8081 лише для реєстрації шардів.
З `EXPECTED_SHARDS=2` він не приймає записів, поки не зареєструються обидва шарди.

Бенчмарк у sandbox, коли все задеплоєно:
```bash
bash scripts/bench.sh                  # YCSB A і C по 3 рази, результати в bench/results/<run>/
bash scripts/bench-metrics.sh <run>    # через 10 хв: CPU і CPU-кредити всіх серверів
```

## M2: як підняти в AWS
```bash
bash scripts/deploy-all.sh        # kv-network → kv-artifacts → збірка в S3 → kv-app
bash scripts/verify-network.sh    # мережа, зокрема приватна таблиця лише з local і S3 endpoint
bash scripts/verify-app.sh        # 2 шарди зареєстровані, версія збігається, CRUD працює, правила coordinator-sg
bash scripts/teardown-all.sh      # у кінці сесії
```
CI/CD: на початку сесії лаби `bash scripts/set-ci-creds.sh` кладе ключі сесії в секрети GitHub.
Після цього кожен push у `main` проходить тести і деплоїть. Без дійсних ключів проходять лише тести й збірка.

## M1: як підняти мережу
Що потрібно: запущена лаба AWS Academy і відкритий **CloudShell** у регіоні `us-east-1`.
Можна й звичайний AWS CLI з ключами лаби, вони є в *AWS Details → AWS CLI*.
```bash
git clone https://github.com/KristinaRiabova304/Kv-store-CA-.git kv-store && cd kv-store
bash scripts/deploy.sh            # створює або оновлює стек kv-network, приблизно 2 хв
```

## Як перевірити
```bash
bash scripts/verify-network.sh    # перевіряє CIDR, 2 AZ, маршрут на IGW, ізоляцію приватних підмереж, security groups
```
Останній рядок має бути `ALL CHECKS PASSED`.

Інші докази для M1:
```bash
bash scripts/dissect-labrole.sh        # аналіз LabRole → docs/evidence/
bash scripts/try-iam-stack.sh          # IAM-шаблон: перевірка і відмова при деплої → docs/evidence/
bash scripts/check-quotas.sh           # квоти → переношу в docs/quotas.md
bash scripts/demo-changeset-drift.sh   # демонстрація change set і drift detection
bash scripts/recreate.sh               # видалення й повторне створення із заміром часу
bash scripts/check-passrole.sh         # чи можна дати серверу наявну роль, dry-run → docs/evidence/
```

Скріншоти з консолі лежать у `docs/evidence/screenshots/`:
- `05-cfn-stack.png`: стек `kv-network`, 17 ресурсів у стані CREATE_COMPLETE;
- `06-subnets.png`: чотири підмережі 10.20.0.0/24, 10.20.1.0/24, 10.20.10.0/24, 10.20.11.0/24;
- `07a-public-rt.png`: у публічній таблиці маршрут 0.0.0.0/0 на internet gateway;
- `07b-private-rt.png`: у приватній таблиці лише local, виходу в інтернет немає;
- `08a-shard-sg.png`, `08b-coordinator-sg.png`, `08c-edge-sg.png`: ланцюжок security groups,
  9090 лише від coordinator-sg, 8080 лише від edge-sg, 80 з інтернету;
- скріншоти IAM описані в `docs/labrole.md`, кредит у `docs/budget.md`.

## Як видалити
```bash
bash scripts/teardown.sh
```

## Мої рішення
- ADR-0001, інструмент IaC: CloudFormation. Файл `docs/adr/0001-iac-tool.md`
- ADR-0002, розподіл стеків: мережа окремо від застосунку. Файл `docs/adr/0002-stack-layout.md`
- ADR-0003, схема адрес: `10.20.0.0/16`, підмережі `/24`. Файл `docs/adr/0003-cidr-scheme.md`
- ADR-0004, політика тільки на перегляд. Файл `docs/adr/0004-readonly-policy.md`
- ADR-0005, security groups: ланцюжок edge → coordinator → шарди. Файл `docs/adr/0005-security-groups.md`
- ADR-0006, протокол API: REST + JSON, OpenAPI. Файл `docs/adr/0006-api-protocol.md`
- ADR-0007, хеш xxHash64 і 128 віртуальних вузлів. Файл `docs/adr/0007-hash-and-vnodes.md`
- ADR-0008, partition key: шард вибирається за таблицею і partition key. Файл `docs/adr/0008-partition-key.md`
- ADR-0009, coordinator у публічній підмережі, шарди в приватних. Файл `docs/adr/0009-m2-placement.md`
- ADR-0010, доставка коду через S3 і CI/CD. Файл `docs/adr/0010-delivery-and-cicd.md`
- ADR-0011, Go і зберігання в пам'яті. Файл `docs/adr/0011-go-and-in-memory.md`

## Що змінилося з попереднього етапу
**M2 порівняно з M1:**
- з'явився сам застосунок: coordinator і шарди на Go, API в `api/openapi.yaml`;
- у `kv-network` додано S3 gateway endpoint і експорти підмереж; приватні підмережі досі без маршруту в інтернет;
- нові стеки `kv-artifacts` і `kv-app`;
- coordinator-sg тепер пускає шарди і `AdminCidr` на 8080, правила живуть у `kv-app`;
- CI/CD у GitHub Actions.

**M1** - перший етап. Я створила мережевий стек із security groups, IAM-шаблони, розібрала обмеження sandbox,
переглянула квоти й порахувала бюджет для M2.
