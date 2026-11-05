# kv-store - спрощене сховище ключ-значення на зразок DynamoDB

Capstone-проєкт курсу Cloud Architectures 2026/27. Поточний етап: **M1 - налаштування інфраструктури**.

## Що де лежить
```
openspec/                      специфікації та change proposals, я пишу їх до коду
  changes/m1-infrastructure/   proposal, задачі й специфікації для M1
infra/
  network.yaml                 VPC, 2 публічні та 2 приватні підмережі, IGW, таблиці маршрутів, security groups
  iam-readonly.yaml            політика тільки на перегляд і роль, лише як код, sandbox її не створює
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

## Як підняти
Що потрібно: запущена лаба AWS Academy і відкритий **CloudShell** у регіоні `us-east-1`.
Можна й звичайний AWS CLI з ключами лаби, вони є в *AWS Details → AWS CLI*.
```bash
git clone <цей репозиторій> && cd kv-store
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

## Що змінилося з попереднього етапу
M1 - перший етап. Я створила мережевий стек із security groups, IAM-шаблони, розібрала обмеження sandbox,
переглянула квоти й порахувала бюджет для M2.
