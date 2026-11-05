# Задачі: M1 - налаштування інфраструктури

## 1. Спершу план
- [x] 1.1 Написати proposal, специфікації та ADR-0001..0003.

## 2. Розбираюся з sandbox
- [x] 2.1 Запустити `scripts/dissect-labrole.sh`, результат зберегти в `docs/evidence/`
- [x] 2.2 Перевірити кілька дій у IAM Policy Simulator у консолі, зробити скріншоти
- [x] 2.3 Заповнити `docs/labrole.md`: що дозволено, що заборонено, як я це з'ясувала

## 3. IAM як код
- [x] 3.1 Написати `infra/iam-readonly.yaml`: політика тільки на перегляд і роль
- [x] 3.2 Перевірити шаблон командою `aws cloudformation validate-template`
- [x] 3.3 Спробувати задеплоїти й зберегти помилку AccessDenied у `docs/evidence/`
- [x] 3.4 Пояснити в `docs/iam-readonly.md`, що шаблон зробив би і чому sandbox не дає

## 4. Мережа як код
- [x] 4.1 Написати `infra/network.yaml`
- [x] 4.2 Задеплоїти через `scripts/deploy.sh`
- [x] 4.3 Перевірити таблиці маршрутів, підмережі й IGW через `scripts/verify-network.sh`
- [x] 4.4 Показати change set: подивитися, що зміниться, ще до застосування
- [x] 4.5 Показати drift detection: змінити тег руками й побачити, що CloudFormation це помітив
- [x] 4.6 Описати в ADR-0005 і специфікації security groups для балансувальника, coordinator і шардів
- [x] 4.7 Додати security groups у `infra/network.yaml` і перевірки в `scripts/verify-network.sh`

## 5. Планую M2
- [x] 5.1 Запустити `scripts/check-quotas.sh`, заповнити `docs/quotas.md`
- [x] 5.2 Вписати в `docs/budget.md` реальний кредит sandbox

## 6. Відтворюваність
- [x] 6.1 Запустити `scripts/recreate.sh`, записати час і проблеми в `docs/recreatability.md`

## 7. Захист
- [x] 7.1 README: як підняти, як перевірити, що змінилося
- [x] 7.2 Транскрипт AI-критики в `docs/ai-critique.md`
- [x] 7.3 Підготувати відповіді на контрольні питання

## 8. Правки після AI-рев'ю
- [x] 8.1 Звузити `infra/iam-readonly.yaml` і виправити ADR-0004
- [x] 8.2 Виправити твердження про зони в специфікації та про адреси в ADR-0003
- [x] 8.3 Записати відкриті питання M2 в ADR-0005 і `docs/budget.md`
- [x] 8.4 Посилити перевірки в `scripts/verify-network.sh`, додати `scripts/check-passrole.sh`
- [ ] 8.5 Запустити нові перевірки в sandbox і зберегти вивід у `docs/evidence/`
