# AI-рев'ю архітектури - Milestone 2

## Як я його отримала
Рев'ю зробив той самий сеанс AI, який допомагав мені писати код M2. Це слабкість: рецензент оцінює
власну роботу. Тому в промпті я прямо попросила шукати баги і місця, де документація обіцяє більше, ніж робить код.
Рев'юер читав код, шаблони, ADR, специфікації і результати бенчмарку `docs/benchmark.md`.
Перед рев'ю система була задеплоєна в sandbox і пройшла `verify-app.sh`, але сам рев'юер поведінку відмов у AWS не перевіряв.

### Промпт
You are a senior distributed-systems engineer reviewing a student's M2 of a DynamoDB-like key-value store:
a Go coordinator with a consistent-hashing ring (xxHash64, 128 vnodes) routing to 2 in-memory shards on EC2,
deployed with CloudFormation from GitHub Actions in an AWS Academy sandbox (no IAM roles, 4-hour sessions).
Critique correctness under failures and restarts, the routing and registration protocol, security, the
deploy pipeline and the validity of the YCSB baseline. You helped write this code, so look hardest for real bugs
and for places where the docs promise more than the code does. Order by severity. Be blunt.

## Транскрипт
Відповідь AI без змін:

```text
Architecture review: kv-store M2

The happy path works and the ring is sound: the distribution test and the YCSB run agree (51/49).
But the coordinator's view of the world is fragile. Several ordinary events - a restart, a deploy,
shards starting in a different order - lose data silently, and the client gets a 404 instead of an error.
A store that answers "not found" for data it has lost is worse than one that answers "unavailable".

Critical: silent data loss

1. The coordinator forgets every table when it restarts.
   Table definitions live only in Server.tables (internal/coordinator/server.go). Shards re-register
   within 10 s, but tables do not. After a coordinator restart every request returns 404
   table_not_found, although the shards still hold all the data. The spec's "coordinator restarted"
   scenario only covers shards. Clients will re-create tables and carry on, so the outage looks like
   empty tables rather than a failure.

2. Data written before the last shard registers becomes unreachable.
   In single-node mode the coordinator keeps items in its local store. When the first shard registers,
   backendFor() routes everything to the ring and the local items are never read again. The same thing
   happens between the first and the second shard: keys written while only shard-1 was in the ring now
   hash to shard-2 for about half of them. On EC2 the shards boot in parallel, so the window is real.
   No migration, no warning, no 503: reads simply return 404.

3. A restarted shard comes back empty, and nobody notices.
   Heartbeats are received but never used: there is no "last seen", so a dead shard stays in the ring
   forever (good: requests fail with 503) - but a shard that restarts with the same ID and address is
   accepted as if nothing happened, with an empty map. Every deploy is exactly this: CloudFormation
   stops and starts all three instances, so every release wipes the store. ADR-0010 says so, but the
   API does not: after a deploy, reads return 404, not an error. At minimum, a shard should register
   with a boot ID so the coordinator can tell "same shard" from "same shard, lost its data".

High

4. The control plane is on the same port as the data plane, without authentication.
   POST /internal/shards is reachable by anyone who can reach 8080: the shards, and the AdminCidr.
   Anyone from that address can register a fake shard "shard-3" and receive about a third of all
   writes, or re-register "shard-1" at their own address and take over half of the keyspace. The
   security group cannot help, because it filters ports, not paths. Put /internal/* on a separate
   port that only kvstore-shard-sg can reach.

5. A deploy is a full outage and can turn into a replacement.
   All three instances stop and start at the same time; CloudFormation reports success before the
   service is healthy, and the pipeline does not run verify-app.sh, so a broken release is "green".
   Also, AmiId is an SSM parameter that resolves to the latest Amazon Linux 2023 image on every
   deploy. When AWS publishes a new AMI, the next deploy replaces all instances instead of
   restarting them; the coordinator gets a new private IP, and the shards are updated too only because
   their user data references it. That is luck, not design. Pin the AMI ID and change it on purpose.

6. Session credentials in a public repository's secrets.
   While a lab session is active, the GitHub secrets hold the full voclabs permissions. Any change
   to the workflow merged into main can print or use them. The actions are pinned by tag, not by
   commit SHA, and cfn-lint is installed unpinned at run time - both are supply-chain paths to those
   credentials. The 4-hour expiry limits the damage; it does not prevent it.

Medium

7. The baseline is a single run with an unexplained tail.
   Workload C has p99.9 of 18.5 ms and a max of 72 ms, and the tail appeared in the second half of the run.
   The client is a t3.micro with 16 threads, so the baseline may partly measure the client and the
   T3 CPU-credit balance, not the store. One run gives no variance. Before M3 you need at least three
   runs per workload, and CPU credits for all four instances recorded alongside.

8. The coordinator is the throughput ceiling, and the doc says it without measuring it.
   docs/benchmark.md says the ceiling is "most likely" the coordinator. There is no CPU measurement
   of the coordinator versus the shards during the run, so that is an assumption.

Low

9. HEAD is implemented as a full GET on the shard: up to 400 KB is read, encoded and discarded.
10. Inserting into a partition with a sort key is O(n) (slice insert). Fine for M2, a problem for
    large partitions.
11. A single RWMutex per shard serialises all writes on that shard.

What is good
- Routing by (table, partition key) is the right choice and it is implemented correctly, including the \x00
  separator; the ring tests check both balance and movement against hash % N.
- No NAT, no SSH, no IAM roles: private subnets really stay private (verify-network.sh proves it), and
  the S3-endpoint delivery is a clean answer to the sandbox constraints.
- Shard failure gives 503 within 2 s instead of hanging.
```

## Мої відповіді
Чернетка. Я перечитаю й перепишу її своїми словами до захисту.

Що вже зроблено, 2026-11-12. Спершу я оновила специфікації `sharding`, `api`, `delivery` і `api/openapi.yaml`, потім код:
- `bootId` і лічильник `restarts`;
- `EXPECTED_SHARDS` і помилка `503 cluster_not_ready`;
- внутрішній порт 8081;
- AMI, actions і cfn-lint закріплені;
- бенчмарк робить 3 повтори, а CPU збирає `scripts/bench-metrics.sh`.
Кожну правку перевіряє тест у `internal/coordinator/server_test.go`. У sandbox правки ще не деплоїлися.

### Пункти 1 і 3: coordinator забуває таблиці, а шард після перезапуску повертається порожнім. Приймаю
Це правда, і це гірше, ніж я описала в ADR-0010. Там я написала лише, що «дані зникають при деплої»,
але не подумала, що клієнт отримає `404` замість помилки.

Що змінюю:
- **у M2:** шард при реєстрації надсилає `bootId`, випадковий ID, створений при старті процесу.
  Якщо `bootId` відомого шарда змінився, coordinator пише це в лог і показує в `/internal/shards`.
- **у M3:** таблиці зберігаються на шардах або в окремому сховищі, а репліки відновлюють дані після перезапуску.

### Пункт 2: дані до реєстрації останнього шарда. Приймаю
Режим одного вузла вимагає завдання, тому прибрати його я не можу.

Що змінюю: coordinator приймає записи лише в одному з двох станів. Або шардів немає взагалі, або зареєстровано
очікувану кількість `EXPECTED_SHARDS`. Поки кільце неповне, записи отримують `503`.
Справжнє рішення - перенесення ключів при зміні кільця. Це завдання M5, і там воно знадобиться так само.

### Пункт 4: реєстрація шардів доступна без автентифікації. Приймаю частково
Ризик менший, ніж описує AI. Порт 8080 відкритий лише для моєї IP-адреси `/32` і для групи шардів, тобто
зловмисник мав би бути на моїй машині. Але аргумент «security group фільтрує порти, а не шляхи» правильний.
Винести `/internal/*` на окремий порт, відкритий лише для `kvstore-shard-sg`, коштує кілька рядків, тому зроблю це в M2.
Автентифікацію я відкладаю до M4, де вона і так потрібна для MCP.

### Пункт 6: ключі в секретах GitHub. Спростовую частково
Альтернативи в sandbox немає: OIDC і CodePipeline потребують IAM-ролей, а створювати їх sandbox забороняє,
див. `docs/labrole.md`. Ризик обмежений трьома речами:
- ключі живуть до 4 годин;
- secrets не передаються в запуски з форків;
- змінювати `main` можу лише я.

Що з пункту беру: закріплю actions за SHA коміту, а версію cfn-lint вкажу явно. Це закриває шлях через ланцюжок постачання.

## Що з рештою пунктів
- **5, деплой - повний простій, і AMI не закріплений.** Закріплю `AmiId` конкретним ID.
  Додати `verify-app.sh` у CI просто так не вийде: раннер GitHub не входить в `AdminCidr`, тож до coordinator він не дістанеться.
  Поки що після кожного деплою я запускаю перевірку вручну. Простій під час деплою лишається до M3,
  бо з даними в пам'яті поступовий деплой нічого не зберігає.
- **7 і 8, бенчмарк.** Приймаю. Перед M3 зроблю по 3 прогони на сценарій і запишу CPU coordinator, шардів, клієнта та CPU-кредити.
- **9-11.** Приймаю як відомі обмеження, виправлятиму, якщо бенчмарк покаже, що вони заважають.
