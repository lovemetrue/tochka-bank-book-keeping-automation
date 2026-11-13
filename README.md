# tochka-bank-book-keeping-automation

Автоматизация налогового учёта ИП на УСН «Доходы» 6% (без сотрудников) через API Точка Банка.
Раз в сутки забирает выписку, считает авансы и взносы и напоминает о сроках в Telegram.

## Что делает
- `sync`: Init Statement → опрос Get Statement → дописывает входящие поступления в `income.jsonl` (дедуп по `paymentId`). Подозрительные на «не доход» (возвраты, займы, свои переводы) идут в `review.jsonl` и в расчёт не попадают, пока вы их не разберёте.
- `calc`: авансы нарастающим итогом (`доход × 6% − взносы − прошлые авансы`), взнос 1% свыше 300 000 ₽, сроки.
- `remind`: сводка в Telegram за 7 дней до любого срока из `config.json`.

Платежей программа **не отправляет**. Платёжка-черновик «на подпись» в ФНС запланирована, но не реализована.

## Запуск
```bash
cp .env.example .env            # TOCHKA_JWT=...  (ИБ Точки → Интеграции и API → JWT-ключ)
go test ./...
TOCHKA_CA=ca.pem go run . sync --dry
go run . calc
```
Сборка для сервера: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w' -o tochka-tax .`

Cron (06:00 UTC = 09:00 МСК):
```
0 6 * * * cd /opt/tochka-tax && TOCHKA_CA=/opt/tochka-tax/ca.pem ./tochka-tax sync >> sync.log 2>&1 && ./tochka-tax remind >> sync.log 2>&1
```

## Конфигурация
`config.json`: ставка, фиксированный взнос, порог 1%, сроки, ключевые слова «не доход», список уже уплаченных взносов (`paid`). Всё, что меняется по закону, лежит только здесь; значения на 2026 год нужно сверять с ФНС.

## Особенности
- API Точки подписан корнем Минцифры, которого нет в системных сертификатах. `ca.pem` (публичный корень с gu-st.ru) подключается через `TOCHKA_CA` только для этой программы; проверка TLS не отключается.
- `remind` вызывает Python-скрипт `notify.py` из соседнего проекта (Telegram с обходом DPI); `dotenv.py` заменяет `python-dotenv` на хосте.
- Поля транзакций сверены с документацией, не с живым ответом.

## Источники
[API Точки: выписки](https://developers.tochka.com/docs/tochka-api/opisanie-metodov/vypiski), [вебхуки](https://developers.tochka.com/docs/tochka-api/opisanie-metodov/vebhuki), [авансы УСН](https://tochka.com/knowledge/buhgalteriya/kak-rasschitat-i-uplatit-avansovye-platezhi-po-usn/)
