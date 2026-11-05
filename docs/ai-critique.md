# AI-рев'ю архітектури - Milestone 1

## Як я його отримала
Рев'ю зробив окремий сеанс AI. Він мав доступ до всього репозиторію: шаблонів, ADR, специфікацій, доказів,
завдання і моїх заготовлених відповідей. Тобто це не сліпе рев'ю лише шаблонів, про що він сам пише наприкінці.
Нічого в AWS він не запускав, поведінку AWS описує зі своїх знань.

Номери рядків у транскрипті стосуються версії файлів на момент рев'ю. Після нього файли змінилися.

### Промпт
You are a senior AWS solutions architect reviewing a student's infrastructure for                                                                               
    a DynamoDB-like key-value store: a coordinator and sharded nodes, replication later.                                                                          
    Constraints: AWS Academy sandbox, cannot create IAM roles, small budget.                                                                                      
    Critique the network design, the CIDR plan, the security groups, the stack layout                                                                             
    and the read-only IAM policy. List concrete weaknesses, ordered by severity. Be blunt.  

## Транскрипт
Відповідь AI без змін:

```text
 Architecture review: kv-store M1

  The network will deploy, but it won't support M2. It describes one request path rather than the system the capstone requires, and the IAM policy doesn't deliver
  what ADR-0004 claims.

  I read the code, ADRs, specs and evidence, but ran nothing against AWS. AWS behaviour here comes from my own knowledge, and I've marked what you still need to
  test in the sandbox.

  Critical: blocks M2

  1. You have no working way to access or deploy to the nodes.
  - SSM: docs/adr/0005-security-groups.md:44 relies on it, but it needs an instance profile, and the simulator shows iam:PassRole denied (docs/labrole.md:48). It
    also needs a network path, and private subnets have no route or endpoints. The three interface endpoints cost about $44/month across 2 AZs if left running.
  - EC2 Instance Connect: this needs port 22 reachable, either on a public IP or from an Instance Connect Endpoint. Your spec bans port 22 everywhere, and no
    endpoint exists.
  - CI/CD: M2 requires it, but nothing can deliver code to a private node.
  - Deferred test: the check that decides all this is a single dry-run, and you've put it off until M2 (docs/labrole.md:68). If it fails, instances can't make
    signed AWS calls, which also affects S3 logs in M3 and CloudWatch metrics in M4 and M5.
  - Fix: run the dry-run now. Then test a free AWS::EC2::InstanceConnectEndpoint with port 22 open only from its own SG, and amend the spec.

  2. The SG chain blocks flows the capstone requires. infra/network.yaml:188-194 only allows edge to coordinator.

  ┌───────────┬───────────────────────────────────────────┬────────────────────────────────────────────────────┐
  │ Milestone │                   Flow                    │                       Status                       │
  ├───────────┼───────────────────────────────────────────┼────────────────────────────────────────────────────┤
  │ M2        │ Shard registers with coordinator on start │ Blocked: shards can't reach the coordinator port   │
  ├───────────┼───────────────────────────────────────────┼────────────────────────────────────────────────────┤
  │ M2        │ YCSB client to coordinator                │ No SG; the only route is through the load balancer │
  ├───────────┼───────────────────────────────────────────┼────────────────────────────────────────────────────┤
  │ M3        │ Shard to shard replication                │ Noted in ADR-0005, not built                       │
  ├───────────┼───────────────────────────────────────────┼────────────────────────────────────────────────────┤
  │ M3        │ Nodes to log broker                       │ No SG                                              │
  ├───────────┼───────────────────────────────────────────┼────────────────────────────────────────────────────┤
  │ M4        │ Metrics scraping                          │ No SG                                              │
  └───────────┴───────────────────────────────────────────┴────────────────────────────────────────────────────┘

  Self-referencing rules also need standalone AWS::EC2::SecurityGroupIngress resources, so the inline pattern won't carry over.

  3. The budget and the topology describe different systems. docs/budget.md:10 has the coordinator on a public IP with no ALB, while ADR-0003 and ADR-0005 put it
  in a private subnet behind one. In budget variant A, nothing can reach port 8080.

  Pick one and write the ADR. I'd drop the ALB for M2, since it adds cost and a latency hop in front of a single target. Put the coordinator in a public subnet,
  restrict 8080 to an AllowedClientCidr, and run YCSB inside the VPC.

  High

  4. Two AZs can't survive an AZ loss under quorum. With N=3, one AZ holds two replicas.
  - W=2, R=2: losing the AZ with two replicas loses quorum.
  - W=1, R=3 (the capstone's single-leader case): losing either AZ makes R=3 impossible, because a read needs all three replicas.

  So your spec's "replicas in the other zone keep working" and the rebuttal at docs/ai-critique.md:29 don't hold for quorum. The capstone doesn't require 2 AZs
  and empty subnets are free, so create the third AZ now and update scripts/verify-network.sh:22.

  5. The policy doesn't separate "view" from "data". ADR-0004:39 says data and secrets are protected, but these allowed actions expose them:

  ┌────────────────────────────────┬─────────────────────────────────┬────────────────────────────────────────────────────────────────────────────────────────┐
  │            Line in             │         Allowed action          │                                    What it exposes                                     │
  │    infra/iam-readonly.yaml     │                                 │                                                                                        │
  ├────────────────────────────────┼─────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ 59                             │ lambda:GetFunctionConfiguration │ Environment variables, a common place for secrets                                      │
  ├────────────────────────────────┼─────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ 47                             │ ec2:Describe*                   │ User data, through DescribeInstanceAttribute and launch template versions              │
  ├────────────────────────────────┼─────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ 50                             │ cloudformation:Get*             │ Full templates, through GetTemplate                                                    │
  ├────────────────────────────────┼─────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ 53                             │ s3:ListBucket                   │ Object keys, which are data in a key-value store; it also contradicts "bucket info     │
  │                                │                                 │ only"                                                                                  │
  ├────────────────────────────────┼─────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ 67                             │ iam:Get*                        │ The whole account's IAM layout, through GetAccountAuthorizationDetails                 │
  └────────────────────────────────┴─────────────────────────────────┴────────────────────────────────────────────────────────────────────────────────────────┘

  The Deny list also misses dynamodb:PartiQLSelect, dynamodb:GetRecords, logs:StartQuery, logs:GetQueryResults, logs:StartLiveTail, lambda:GetFunction and
  ec2:GetConsoleOutput. If someone attaches ReadOnlyAccess, several of those become available, so the claim at ADR-0004:32 fails.

  ADR-0004:34 is wrong too: nothing in the Allow list could match GetItem.

  Finally, cloudformation:DetectStack* (line 51) starts a drift operation and updates drift status, which isn't strictly view-only.

  6. Stateful shards are in a stack you treat as disposable. ADR-0002 plans to redeploy the app dozens of times, but replicas hold data, and replacing an instance
  destroys its root volume. Split data volumes into their own stack with a DeletionPolicy, and deploy code through the pipeline rather than by replacing
  instances.

  7. The security groups are in the wrong stack. ADR-0002 justifies the network stack because it rarely changes and doesn't list SGs in it, yet ADR-0005 added
  them plus the app ports.
  - Update churn: every flow change from M2 to M5 now means updating kv-network.
  - Export lock-in: exported SG IDs can't change while another stack imports them. Changing a GroupDescription forces replacement, which also collides with the
    fixed GroupName.
  - Subnet exports: comma-joined subnet lists (infra/network.yaml:223-228) are awkward for placing replicas per AZ. Export each subnet separately, with its AZ.
  - Fix: move the SGs and ports to a kv-security stack or the app stack.

  8. The data API is planned to be open to the internet over plain HTTP. infra/network.yaml:177 allows 0.0.0.0/0 on port 80. The "HTTPS in M2" plan also has no
  path yet, because a public ACM certificate needs a domain you control. Either get a domain, or use an IP allowlist and state plainly that traffic is HTTP.

  Medium

  9. The free S3 gateway endpoint is missing. Your budget recommends it, but the template doesn't have it. The private subnet spec also says "only local", which
  would forbid its route, so reword that to "no route to an IGW or NAT". Without NAT, the Datadog and Sentry options in M4 are also unreachable from private
  nodes.

  10. The IAM template has never really been tested. validate-template only checks syntax, and the deploy failed on the first resource, so the role and trust
  policy were never evaluated. Try aws accessanalyzer validate-policy and aws iam simulate-custom-policy, which create nothing. Check that the sandbox allows
  them.

  11. The trust policy defaults to the weak option. It defaults to account root with MFA off, and your spec makes that official. Having parameters doesn't help
  when the default is the loose one, so make TrustedPrincipalArn required with no root fallback.

  12. The Resource: "*" rebuttal is only partly true (docs/ai-critique.md:32). ec2:Describe* needs it, but s3:ListBucket, s3:GetBucket*, dynamodb:DescribeTable,
  lambda:GetFunctionConfiguration and IAM reads can be scoped to resources. A tutor can easily catch this one.

  13. scripts/verify-network.sh passes on broken configurations.
  - Lines 43 and 45: "only from" really means "contains this SG and not 0.0.0.0/0", so an extra rule for 10.20.0.0/16 passes.
  - Line 48: FromPort==22 misses all-traffic rules and port ranges such as 0-65535.
  - Not checked: subnet-to-route-table associations and the edge SG rules, so a private subnet attached to the public route table would pass.

  14. Public IPs are assigned by default, and open egress is justified by placement.
  - Public IPs: MapPublicIpOnLaunch: true gives every instance in those subnets a billable public IP, so assign them explicitly instead.
  - Egress: ADR-0005:47 relies on "no route anyway", which stops being true if a shard lands in a public subnet or NAT is added. At minimum, limit edge SG egress
    to the coordinator port.

  Low

  15. The CIDR plan is sound, with small gaps. The sizing and layout are fine, and I wouldn't split coordinators and shards into separate subnets.
  - Parameters: the CIDR parameters have no validation and can overlap, and the verify script hard-codes 10.20.0.0/16 anyway (line 18). Remove them or derive them
    with !Cidr.
  - Lab VPC range: ADR-0003:16 says labs "usually" use 10.0.0.0/16, but I didn't find that in your labrole or quota evidence. Run describe-vpcs and cite the
    result.

  16. Flow logs are less blocked than they look. An S3 destination doesn't need an IAM role, so docs/ai-critique.md:31 is a choice to defer, not a sandbox limit.
  The default SG is also not locked down.

  Outside your five areas, but it affects the budget

  - T3 credits: T3 defaults to unlimited credits, so sustained load can add up to about $0.09/hour per t3.micro, against a $0.0104 base rate.
  - Benchmark validity: credit state also skews the M2, M3 and M5 YCSB comparisons, so fix the credit mode and record it.
  - Cross-AZ traffic: replication traffic between AZs isn't in the budget either.

  What holds up

  Keep these: SG-to-SG references, no public SSH, no always-on NAT, the separate network stack, and the 144-second rebuild from code.

  Before you use this at your defense

  - It may not count as a fresh review: docs/ai-critique.md asks for a new chat with only the templates and ADRs. I also read your prepared rebuttals, the
    capstone and your evidence, so check whether this meets the requirement, or rerun it in a new chat if not.
  - Rewrite two rebuttals: the ones at lines 29 and 32 of docs/ai-critique.md don't hold up, as items 4 and 12 explain.
```

## Мої відповіді
Чернетку цих відповідей підготовлено разом з AI, Claude Code. Рішення і перевірка аргументів мої.

### Пункт 5, політика не відділяє перегляд від даних: приймаю
Я вважала, що достатньо заборонити `GetObject` чи `GetItem`. Але метадані теж можуть розкривати дані:
`s3:ListBucket` віддає ключі, а в сховищі ключ-значення ключ уже є даними. User data і змінні середовища
часто містять паролі. Твердження в ADR-0004, що дані й секрети захищені, було завищеним,
а приклад із `GetItem` неправильним, бо жоден мій Allow під нього не підходив.

Що я зробила: прибрала зі списку Allow `s3:ListBucket`, Lambda, `cloudformation:Get*`, `cloudformation:DetectStack*`
і широкий `iam:Get*`, розширила список Deny і переписала ADR-0004. Там тепер прямо сказано,
що список Deny не гарантує повного захисту, а AWS перевірив лише синтаксис шаблону.

### Пункт 4, дві зони не тримають кворум: приймаю, рішення в M3
Рахую для N=3 у двох зонах. Одна зона тримає дві репліки. Якщо вона падає, лишається одна репліка,
і кворум W=2, R=2 недосяжний. Тому фраза в специфікації, що репліки в іншій зоні продовжать працювати,
була неправильною, і я її виправила.

Третю зону я зараз не додаю. Завдання M1 зон не вимагає, а скільки реплік і який кворум я оберу, вирішиться в M3.
Адреси `10.20.2.0/24` і `10.20.12.0/24` вже зарезервовані в ADR-0003, тож це буде зміна шаблону, а не переробка мережі.

### Пункт 2, ланцюжок security groups блокує потрібні потоки: приймаю частково
**Приймаю** про реєстрацію шарда. Завдання M2 прямо вимагає, щоб шарди реєструвалися в coordinator при старті,
а зараз шард не може дістатися порту coordinator. Це записано в ADR-0005 як перше відкрите питання для M2.

**Спростовую** потребу вже зараз робити групи для брокера логів і збору метрик. Цих компонентів ще немає,
я не вибрала ні продукт, ні порти, тому такі правила були б здогадками. Правило без реального споживача лише
відкриває зайвий доступ. Я додам їх разом із самими компонентами в M3 і M4.

## Що з рештою пунктів
| Пункт | Що я зробила |
|---|---|
| 1, доступ до серверів | Написала `scripts/check-passrole.sh`, у ADR-0005 записала, що робочого способу доступу поки немає |
| 3, бюджет і мережа розходяться | Записала в `docs/budget.md`, вибір окремим ADR на початку M2 |
| 6, дані шардів у стеку застосунку | Вирішу разом зі стеком застосунку в M2 |
| 7, групи в мережевому стеку | Записала в ADR-0005, перенесу до появи стека застосунку |
| 8, HTTP з інтернету | Записала в ADR-0005 |
| 9, S3 endpoint | Вирішу в M2, коли стане зрозуміло, як сервери отримують код |
| 10, шаблон IAM реально не перевірявся | Записала це обмеження в `docs/iam-readonly.md` і ADR-0004 |
| 11, trust policy за замовчуванням | Поки лишаю, у реальному акаунті передала б `TrustedPrincipalArn` |
| 12, `Resource: *` | Приймаю: стару відповідь я прибрала, обмеження записала в ADR-0004 |
| 13, слабкі перевірки | Виправила `scripts/verify-network.sh` |
| 14, публічні IP і вихідний трафік | Вихідний трафік записала в ADR-0005, публічні IP перегляну в M2 |
| 15, адреси | Виправила ADR-0003: окремої VPC з лаб у мене немає. Параметри CIDR поки лишаю |
| 16, flow logs | Це мій вибір відкласти, а не обмеження sandbox. Повернуся в M4 |
| CPU-кредити і трафік між зонами | Записала в `docs/budget.md` |
