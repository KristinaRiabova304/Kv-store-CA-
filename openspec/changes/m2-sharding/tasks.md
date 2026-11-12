# Задачі: M2 - шардування навантаження

## 0. Хвости M1
- [ ] 0.1 Запустити `verify-network.sh`, `check-passrole.sh`, `try-iam-stack.sh` у sandbox, зберегти вивід (задача 8.5 з M1)
- [ ] 0.2 Закомітити спец M2 окремим комітом **до** першого рядка коду

## 1. Спершу план
- [x] 1.1 Proposal і специфікації `api`, `sharding`, `delivery`
- [x] 1.2 Контракт API `api/openapi.yaml`
- [x] 1.3 ADR-0006..0011 у статусі «запропоновано»
- [ ] 1.4 Перечитати, виправити під себе, змінити статус ADR на «прийнято»

## 2. Перевірити ризики в sandbox
- [x] 2.1 Чи можна створити S3-бакет з bucket policy за `aws:SourceVpce`
- [x] 2.2 Чи можна створити S3 gateway endpoint
- [ ] 2.3 Результат `check-passrole.sh`: чи можна дати серверу наявний instance profile
- [ ] 2.4 Якщо щось заборонено, оновити ADR-0010 до коду інфраструктури

## 3. Сховище і кільце
- [x] 3.1 `internal/store`: інтерфейс `Store` і реалізація в пам'яті, тести
- [x] 3.2 `internal/ring`: кільце xxHash64 зі 128 віртуальними вузлами, тести рівномірності та переїзду ключів
- [x] 3.3 Вписати реальні цифри з тестів кільця в ADR-0007

## 4. Coordinator і шард
- [x] 4.1 `cmd/shard`: внутрішній API на 9090, реєстрація і повтор кожні 10 с
- [x] 4.2 `cmd/coordinator`: API за OpenAPI, режим одного вузла, маршрутизація через кільце, тайм-аут 2 с → 503
- [x] 4.3 `/health` і `/version` на обох
- [x] 4.4 Інтеграційний тест: coordinator і 2 шарди локально, усі сценарії зі специфікації `api`
- [ ] 4.5 `docker-compose.yaml` для локального запуску (написано, не перевірено: Docker не був запущений)

## 5. Інфраструктура
- [x] 5.1 `kv-network`: S3 gateway endpoint для обох таблиць маршрутів, експорт підмереж і endpoint
- [x] 5.2 `infra/artifacts.yaml`: бакет і bucket policy
- [x] 5.3 `infra/app.yaml`: coordinator, 2 шарди, правила `*Ingress`, параметри `ReleaseVersion` і `AdminCidr`
- [x] 5.4 Оновити `scripts/verify-network.sh`: у приватній таблиці лише `local` і S3 endpoint
- [x] 5.5 `scripts/deploy-all.sh`, `release.sh`, `verify-app.sh`, `teardown-all.sh`, `deploy/bootstrap.sh`
- [x] 5.6 Перший деплой у sandbox, вивід `verify-network.sh` і `verify-app.sh` у `docs/evidence/`

## 6. CI/CD
- [x] 6.1 `.github/workflows/ci.yaml`: тести → збірка → S3 → оновлення стеків
- [x] 6.2 `scripts/set-ci-creds.sh`
- [ ] 6.3 Показати повний прогін: push → нова версія на `/version`

## 7. Бенчмарк
- [x] 7.1 YCSB `rest` binding відтворює записані запити і не підходить; узяла go-ycsb з власним драйвером `bench/ycsb`
- [x] 7.2 Клієнтський сервер і прогін workload A і C, вивід у `bench/results/`
- [x] 7.3 `docs/benchmark.md`: що запускала, цифри, чому ці workloads

## 8. Захист
- [ ] 8.1 README: як підняти, як перевірити, що змінилося з M1
- [ ] 8.2 Оновити `docs/budget.md` під фінальну схему
- [x] 8.3 AI-критика архітектури M2 і мої відповіді в `docs/ai-critique-m2.md`
- [x] 8.4 Відповіді на контрольні питання M2

## 9. Правки після AI-рев'ю M2
- [x] 9.1 Спершу оновити специфікації `sharding` і `delivery` під правки нижче, потім код
- [x] 9.2 `bootId` у реєстрації шарда: coordinator бачить, що шард перезапустився і втратив дані
- [x] 9.3 `EXPECTED_SHARDS`: поки кільце неповне, записи отримують 503
- [x] 9.4 `/internal/*` на окремому порту, відкритому лише для `kvstore-shard-sg`
- [x] 9.5 Закріпити actions за SHA коміту, версію cfn-lint і `AmiId`
- [ ] 9.6 Бенчмарк: 3 прогони на сценарій, CPU і CPU-кредити всіх серверів (код готовий, прогін - у наступній сесії лаби)
