# Політика тільки на перегляд і роль, лише як код

Шаблон: `infra/iam-readonly.yaml`. Рішення: `docs/adr/0004-readonly-policy.md`.

## Що б він зробив, якби його можна було задеплоїти
1. Створює власну політику `kvstore-read-only`:
   - **Allow** на перегляд для сервісів мого проєкту:
     EC2 і VPC, CloudFormation, інформація про бакети S3, інформація про таблиці DynamoDB,
     ELB, Auto Scaling, CloudWatch, Logs, ролі й політики IAM, Service Quotas.
   - **Явний Deny** на читання даних і секретів: `s3:GetObject`, `s3:ListBucket`,
     `dynamodb:GetItem/Query/Scan`, `secretsmanager:GetSecretValue`, user data серверів,
     змінні середовища Lambda та інші.
   - Усе інше, тобто будь-який запис, заборонено **неявно**: для цього немає жодного Allow.
2. Створює роль `kvstore-read-only-role` з двома політиками:
   - **Trust policy**, вона ж `AssumeRolePolicyDocument`, визначає, *хто* може стати цією роллю.
     За замовчуванням це root акаунта, тобто будь-хто з цього акаунта, кому
     самому дозволено `sts:AssumeRole`. Можна ввімкнути обов'язковий MFA.
   - **Permissions policy** визначає, *що* роль може робити. Це політика вище.
3. Хтось, наприклад аудитор, CI-задача чи інструмент моніторингу, викликає `sts:AssumeRole`,
   отримує тимчасові ключі максимум на 1 годину і може лише дивитися.

## Як IAM перевіряє запит, по порядку
1. Є явний **Deny** будь-де → заборонено.
2. Інакше є відповідний **Allow** → дозволено.
3. Інакше → заборонено за замовчуванням, це implicit deny.

Ще SCP організації та permission boundaries можуть права тільки звузити, але не розширити.

Приклади: `s3:GetBucketPolicy` → підходить під Allow `s3:GetBucket*`, Deny немає → дозволено.
`s3:GetObject` → підходить під Deny → заборонено. `ec2:CreateVpc` → немає Allow → заборонено.

## Чому sandbox відмовляє
Моя ідентичність у sandbox, `voclabs`, і організація AWS Academy не дозволяють
`iam:CreatePolicy` і `iam:CreateRole`. Створення ролей - це шлях до підвищення
прав: студент міг би створити роль з `AdministratorAccess`, дозволити EC2
її прийняти й обійти всі обмеження лаби: регіони, типи серверів, бюджет.
Тому AWS Academy дає лише готові ролі, які створила сама. У моєму sandbox це `voclabs` для мене
і кілька ролей для сервісів, наприклад `lambda-run-role`. LabRole, про яку пише завдання, у моєму акаунті немає.

## Що сталося при спробі деплою
Докази: `docs/evidence/iam-deploy-20261101-133412.txt`, його створив `scripts/try-iam-stack.sh`.
Цей запуск стосується першої версії шаблону. Після AI-рев'ю я звузила політику, тому шаблон треба перевірити
ще раз тим самим скриптом. Новий вивід ляже поруч окремим файлом.
1. `validate-template` пройшов успішно. Синтаксис шаблону правильний, і AWS попередив, що йому потрібна
   `CAPABILITY_NAMED_IAM`, бо він створює IAM-ресурси з фіксованими іменами.
2. Деплой упав на ресурсі `ReadOnlyPolicy` з помилкою:
   *not authorized to perform: iam:CreatePolicy [...] because no identity-based policy allows the iam:CreatePolicy action*.
   Це неявна відмова: жодна політика `voclabs` не дозволяє створювати політики.
3. До ролі `ReadOnlyRole` CloudFormation навіть не дійшов. Роль посилається на політику через
   `!Ref ReadOnlyPolicy`, тому створюється після неї. Політика не створилась, і стек відкотився.
4. Що `iam:CreateRole` теж заборонено, я довела окремо: пряма спроба `aws iam create-role`
   повернула `AccessDenied`, див. `docs/evidence/labrole-20261101-131731.txt`.

Після цього я видалила невдалий стек командою `aws cloudformation delete-stack --stack-name kv-iam-readonly`.

## Чого цей доказ не показує
`validate-template` перевіряє лише синтаксис, а деплой упав на першому ресурсі. Отже, AWS не оцінював
ні зміст політики, ні trust policy ролі. Те, що політика працює як задумано, тут не доведено.
