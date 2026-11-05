# LabRole: що дозволяє, що забороняє, як я це з'ясувала

## Головне відкриття: LabRole у моєму sandbox немає
Завдання просить розібрати LabRole. Але в моєму акаунті такої ролі не існує:
`aws iam get-role --role-name LabRole` повертає `NoSuchEntity`.
Список ролей акаунта, `aws iam list-roles`, показує інші ролі: `voclabs`, `vocareum`,
`lambda-run-role`, `myBasicLambdaRole`, `myS3Role`, `EMR_EC2_DefaultRole`, `EMR_DefaultRole`
і службові ролі AWS з префіксом `AWSServiceRoleFor`.

Тому я розбираю роль, під якою справді працюю, - **`voclabs`**. Саме вона визначає,
що мені дозволено в консолі й CLI. Докази: `docs/evidence/labrole-20261101-131731.txt`
`docs/evidence/labrole-extra.txt` і `docs/evidence/labrole-extra2.txt`.

## Хто я
`aws sts get-caller-identity` показує:
`arn:aws:sts::891377215247:assumed-role/voclabs/user5460708=kriabova@kse.org.ua`.
Тобто я не IAM-користувач, а тимчасово прийняла роль `voclabs`.

Що до неї прикріплено, `aws iam list-attached-role-policies` і `list-role-policies`:
- AWS managed політика **`ReadOnlyAccess`** - дозволяє читати майже все;
- inline-політика **`Pvoclabs`** - її вміст я побачити не можу: і `iam:GetRole`, і `iam:GetRolePolicy`
  на `voclabs` заборонено явним Deny. Роль приховує сама себе, щоб студенти не бачили точних правил.

## Як я це з'ясовувала
1. **CLI**: `get-caller-identity`, `list-roles`, `list-attached-role-policies`, `list-role-policies`.
2. **Policy simulator**: `aws iam simulate-principal-policy` для ролі `voclabs`. Він відповідає,
   чи буде дія дозволена, і нічого не виконує.
3. **Спроби й помилки**: `run-instances --dry-run`, спроби `create-policy`, `create-role`, `create-user`,
   `describe-vpcs` у різних регіонах. Текст помилки `AccessDenied` показує, яка саме політика заборонила.
4. **Консоль**: IAM → Roles, IAM Policy Simulator. Скріншоти в `docs/evidence/screenshots/`.
   Консольний симулятор для `voclabs` не запрацював: він одразу показує *Access denied to iam:GetRolePolicy*,
   бо не може прочитати приховану `Pvoclabs`, і кнопка Simulate лишається неактивною.
   Тому всі результати симулятора в таблиці нижче отримані через CLI `simulate-principal-policy`.

Чому кілька способів: вміст `Pvoclabs` прихований, а SCP організації AWS Academy
в самій ролі взагалі не видно. Справжні межі показують лише реальні виклики.

## Результати
| Область | Дія | Результат | Тип відмови | Як з'ясувала |
|---|---|---|---|---|
| Мережа | `ec2:CreateVpc`, `ec2:CreateNatGateway` | Дозволено | | симулятор |
| Сервери | `ec2:RunInstances` для `t3.micro` | Дозволено | | dry-run повернув `DryRunOperation` |
| Сервери | `ec2:RunInstances` для `m5.4xlarge` | **Заборонено** | явний Deny в identity-based політиці | dry-run повернув `UnauthorizedOperation` |
| Регіони | `us-east-1`, `us-west-2` | Дозволено | | `describe-vpcs` спрацював |
| Регіони | `us-east-2`, `us-west-1`, `eu-west-1`, `eu-central-1` | **Заборонено** | явний Deny у **SCP** організації AWS Academy | `describe-vpcs` у кожному регіоні |
| Сервіси | S3, DynamoDB, Lambda, CloudFormation: створення | Дозволено | | симулятор |
| IAM | `iam:CreatePolicy`, `CreateRole`, `CreateUser` | **Заборонено** | неявна відмова: жодна політика не дозволяє | реальні спроби |
| IAM | `iam:PassRole` без конкретної ролі і для `lambda-run-role`, `myS3Role`, `EMR_EC2_DefaultRole` | Заборонено за симулятором | неявна відмова | симулятор; перевіряю dry-run, `scripts/check-passrole.sh` |
| IAM | `iam:ListRoles`, `ListAttachedRolePolicies` | Дозволено | | спрацювали, це дає `ReadOnlyAccess` |
| IAM | `iam:GetRole` і `iam:GetRolePolicy` на саму `voclabs` | **Заборонено** | явний Deny в identity-based політиці | реальні спроби |
| Організації | `organizations:CreateAccount` | **Заборонено** | явний Deny | симулятор |
| Білінг | `ce:GetCostAndUsage` | Дозволено | | симулятор і реальний виклик: витрати за жовтень $0 |

## Три види відмови, які я побачила
1. **Неявна відмова, implicit deny.** Текст помилки: *no identity-based policy allows*.
   Дії ніхто не дозволив, отже вона заборонена. Так заборонено створювати IAM-ресурси.
2. **Явний Deny у політиці ролі.** Текст: *with an explicit deny in an identity-based policy*.
   Хтось навмисно заборонив. Так заборонено великі сервери й читання самої `voclabs`.
   Найімовірніше, ці Deny сидять у прихованій `Pvoclabs`.
3. **Явний Deny у SCP.** Текст: *with an explicit deny in a service control policy*.
   Це обмеження на рівні всієї організації AWS Academy. Його не видно в ролі, і навіть
   адміністратор акаунта не може його обійти. Так заборонено інші регіони.

## Що це означає для проєкту
- Я **не можу створювати власні ролі** → мої ролі з мінімальними правами існують лише як код у `infra/iam-readonly.yaml`.
- LabRole немає → серверам у M2 треба буде дати одну з наявних ролей. Instance profiles є лише два:
  `myS3Role` і `EMR_EC2_DefaultRole`. Але симулятор каже, що `iam:PassRole` на них неявно заборонено.
  Симулятор не бачить усього, тому я перевіряю це вже зараз: `scripts/check-passrole.sh` робить dry-run
  з обома профілями і зберігає вивід у `docs/evidence/passrole-dry-run.txt`.
  **Результат: ще не запускала.**
  Якщо не вийде, сервери працюватимуть без ролі, і доступ до AWS API з них доведеться будувати інакше:
  це зачепить логи в S3 у M3 і метрики CloudWatch у M4.
- Лише малі сервери → проєктую під `t3.micro` і подібні.
- Лише `us-east-1` і `us-west-2` → один регіон, `us-east-1`.
- Ключі спливають разом із сесією → жодних довгих пайплайнів на ключах sandbox.

## Скріншоти
Усі в `docs/evidence/screenshots/`, зроблені 4 листопада 2026.

| Файл | Що видно |
|---|---|
| `01-whoami.png` | меню акаунта: я працюю як `voclabs`, акаунт 8913-7721-5247 |
| `02-iam-roles.png` | IAM → Roles: 23 ролі, серед них `myS3Role`, `EMR_EC2_DefaultRole` |
| `02b-labrole-search.png` | пошук `LabRole`: 0 збігів, ролі немає |
| `03-voclabs-getrole-denied.png` | відкриваю `voclabs`: *iam:GetRole* заборонено явним Deny в identity-based політиці |
| `04-policy-simulator.png` | консольний симулятор: *iam:GetRolePolicy* заборонено, симуляція не запускається |
