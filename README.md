# SoupTask

Go job queue with a gRPC API, an HTTP gateway, PostgreSQL storage, and separate worker processes. A worker simulates a 500 ms job; the committed effect is recorded in PostgreSQL as `job_effects_count`.

Очередь задач на Go с gRPC API, HTTP gateway, PostgreSQL и отдельными процессами воркеров. Воркер имитирует работу в течение 500 мс; зафиксированный эффект отражается в PostgreSQL полем `job_effects_count`.

## Navigation / Навигация

| Русский | English |
| --- | --- |
| [Быстрый старт](#ru-quick) | [Quick guide](#en-quick) |
| [Семь ручных проверок](#ru-tests) | [Seven manual tests](#en-tests) |
| [Расширенные тесты](#ru-extended) | [Extended tests](#en-extended) |
| [Структура проекта](#ru-structure) | [Project structure](#en-structure) |

<a id="ru-quick"></a>
## Русский: быстрый старт

Команды ниже выполняются в терминале WSL из корня репозитория. Нужны Docker Desktop с интеграцией WSL, Docker Compose, `curl` и Python 3. Для генерации и Go-тестов нужны Go 1.27.3+ и GNU Make. Для миграций используется [Goose](https://github.com/pressly/goose); `make tools` его не устанавливает.

```bash
mkdir -p .tools/bin
GOBIN="$PWD/.tools/bin" go install github.com/pressly/goose/v3/cmd/goose@v3.30.0
docker compose up -d --wait db
.tools/bin/goose -dir migrations postgres \
  'postgres://souptask:souptask@127.0.0.1:5433/souptask?sslmode=disable' up
docker compose up -d --build
docker compose ps
curl -i 'http://127.0.0.1:8080/v1/jobs?page_size=1'
```

Миграцию достаточно применить один раз к новой базе. При повторном вызове Goose проверяет свою таблицу версий. Не запускайте файл миграции целиком через `psql`: в нём находятся и `Up`, и `Down`. Compose по умолчанию поднимает четыре воркера. HTTP доступен на `127.0.0.1:8080`, gRPC на `127.0.0.1:9090`, PostgreSQL на `127.0.0.1:5433`. `docker compose down` останавливает контейнеры и сохраняет том БД.

Для локальной генерации из proto-контракта:

```bash
make tools
make generate
make lint
make build
```

`make generate` обновляет `pkg/api` и сгенерированный `config/config.go`. Генератор создаёт отсутствующие файлы обработчиков в `internal/app` и не перезаписывает уже написанные обработчики. В Docker Compose число процессов воркера меняется через `docker compose up -d --scale worker=8 worker`. В Kubernetes число реплик берётся из `deploy/k8s/values.yaml` и меняется при `helm upgrade`; это не горячее изменение Go-конфига.

<a id="ru-tests"></a>
## Русский: семь ручных проверок

Сервис должен быть запущен. Выполните подготовку один раз в том же терминале, где будете вводить команды проверок. Каждая проверка использует свой ключ, поэтому существующие записи в БД не мешают.

```bash
BASE='http://127.0.0.1:8080'
RUN="manual-$(date +%s%N)"
```

### 1. Создать и получить задачу

```bash
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-create\",\"payload\":{\"kind\":\"demo\"}}")
printf '%s\n' "$created"
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i "$BASE/v1/jobs/$ID"
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT status, attempt_count, job_effects_count FROM jobs WHERE id=$ID;"
```

Ожидается HTTP 200. Ответ на создание обычно содержит `QUEUED`; к моменту GET задача может уже перейти в `RUNNING` или `SUCCEEDED`. После обработки статус будет `SUCCEEDED`, а `job_effects_count` будет равен 1. При необходимости повторите последние две команды через секунду.

### 2. Повтор с тем же ключом

```bash
for attempt in 1 2; do
  curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-duplicate\",\"payload\":{}}"
  echo
done
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT count(*) FROM jobs WHERE idempotency_key='$RUN-duplicate';"
```

Ожидаются одинаковые ID и одна строка в БД. Статусы двух ответов могут различаться, если воркер успел выполнить задачу между запросами.

### 3. Ошибки входных данных

```bash
curl -i "$BASE/v1/jobs/abc"
curl -i "$BASE/v1/jobs/9223372036854775807"
curl -i -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' -d '{"payload":{}}'
```

Ожидаются соответственно HTTP 400 для неверного ID, HTTP 404 для отсутствующей задачи и HTTP 400 для отсутствующего `idempotencyKey`.

### 4. Пагинация списка

```bash
for n in 1 2 3; do
  curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-page-$n\",\"payload\":{}}" >/dev/null
done
page=$(curl -fsS "$BASE/v1/jobs?page_size=2")
printf '%s\n' "$page"
token=$(printf '%s' "$page" | python3 -c 'import json,sys; print(json.load(sys.stdin)["nextPageToken"])')
curl -iG --data-urlencode 'page_size=2' --data-urlencode "page_token=$token" "$BASE/v1/jobs"
```

Первая страница содержит две задачи и `nextPageToken`; вторая продолжает список без повторения этих ID. Список включает и задачи прошлых запусков.

### 5. Отмена задачи в очереди

```bash
docker compose stop worker
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-cancel\",\"payload\":{}}")
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i -X POST "$BASE/v1/jobs/$ID:cancel"
docker compose start worker
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT status, job_effects_count FROM jobs WHERE id=$ID;"
```

Ожидается `canceled` и ноль эффектов. Даже если проверка прервана, снова запустите воркеры командой `docker compose start worker`.

### 6. Отмена завершённой задачи

```bash
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-finished\",\"payload\":{}}")
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i "$BASE/v1/jobs/$ID"
```

Повторяйте GET, пока статус не станет `JOB_STATUS_SUCCEEDED`. Затем:

```bash
curl -i -X POST "$BASE/v1/jobs/$ID:cancel"
curl -i "$BASE/v1/jobs/$ID"
```

Оба ответа сохраняют статус `SUCCEEDED`: завершённая задача не отменяется задним числом.

### 7. Параллельное создание

```bash
for n in $(seq 1 64); do
  curl -sS -o /dev/null -w '%{http_code}\n' -X POST "$BASE/v1/jobs" \
    -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-parallel-$n\",\"payload\":{}}" &
done
wait
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT count(*) AS total,
       count(*) FILTER (WHERE status='succeeded') AS succeeded,
       count(*) FILTER (WHERE job_effects_count > 1) AS repeated_effects
FROM jobs WHERE idempotency_key LIKE '$RUN-parallel-%';"
```

Ожидаются 64 ответа HTTP 200 и 64 строки. Воркерам нужно время на выполнение: повторите SQL-запрос позже, чтобы увидеть `succeeded=64` и `repeated_effects=0`.

<a id="ru-extended"></a>
## Русский: расширенные тесты

### Автоматические Go-тесты

`internal/storage/postgres/silent_integration_test.go` обращается к реальной БД, поэтому `go test ./...` требует отдельную тестовую базу и `TEST_DATABASE_URL`. Создайте её один раз; не используйте основную БД:

```bash
docker compose exec -T db psql -U souptask -d postgres -c 'CREATE DATABASE souptask_test;'
.tools/bin/goose -dir migrations postgres \
  'postgres://souptask:souptask@127.0.0.1:5433/souptask_test?sslmode=disable' up
TEST_DATABASE_URL='postgres://souptask:souptask@127.0.0.1:5433/souptask_test?sslmode=disable' go test ./...
```

Тесты проверяют одновременный захват одной задачи, завершение с просроченной арендой и гонку отмены с завершением. При повторном запуске используйте существующую `souptask_test`, не создавая её снова.

### Chaos harness

Не запускайте chaos и нагрузочный тест одновременно. Сначала дождитесь пустой очереди и поднимите восемь воркеров:

```bash
docker compose up -d --scale worker=8 worker
docker compose ps -q worker | wc -l
docker compose exec -T db psql -U souptask -d souptask -c \
  "SELECT count(*) FROM jobs WHERE status IN ('queued','running');"
python3 chaos_test.py
```

Скрипт создаёт 10 000 задач, отправляет 1 000 повторных POST и 500 отмен. Примерно каждые две секунды он посылает `kill -9` процессу случайного воркера через ограниченный вспомогательный контейнер с `--pid=host`. `docker kill` здесь не подходит: Docker считает его ручной остановкой и не применяет `restart: unless-stopped`. Скрипт продолжает работу до `pending=0`; не прерывайте его, если нужен завершённый прогон.

Каждый запуск создаёт `chaos-runs/<run>/`. Путь к последнему запуску хранится в `chaos-runs/current`:

| Файл | Что смотреть |
| --- | --- |
| `requests.log` | Сырые ответы POST и отмен; ошибки HTTP помечены кодом или `ERROR`. |
| `kills.log` | Выбранный воркер, PID и результат `kill -9`; `exit=0` означает успешную отправку сигнала. |
| `harness_summary.txt` | `request_errors=0`, `duplicate_id_mismatches=0`, `kills>0`, `pending=0` при успешном завершении. При прерывании файл не создаётся. |
| `invariants.log`, `effects.log` | Создаются проверочными SQL-командами ниже. |

```bash
RUN=$(cat chaos-runs/current)
cat "chaos-runs/$RUN/harness_summary.txt"
tail -n 10 "chaos-runs/$RUN/kills.log"
find chaos-runs -mindepth 2 -maxdepth 2 -name harness_summary.txt | wc -l
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT count(*) AS total,
       count(*) FILTER (WHERE status IN ('queued','running')) AS pending,
       count(*) FILTER (WHERE job_effects_count > 1
         OR (status='succeeded' AND job_effects_count <> 1)
         OR (status IN ('canceled','failed') AND job_effects_count <> 0)
         OR (status='canceled' AND NOT cancel_requested)) AS inconsistent
FROM jobs WHERE idempotency_key LIKE '$RUN-%';" | tee "chaos-runs/$RUN/invariants.log"
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT status, job_effects_count, count(*) AS jobs
FROM jobs WHERE idempotency_key LIKE '$RUN-%'
GROUP BY status, job_effects_count ORDER BY status, job_effects_count;" | tee "chaos-runs/$RUN/effects.log"
```

Команда `find` показывает число завершённых прогонов с итоговым файлом; для отчёта сохраняйте `requests.log`, `kills.log`, `harness_summary.txt` и результаты SQL. Для успешного прогона ожидаются `total=10000`, `pending=0`, `inconsistent=0`; у задач `succeeded` счётчик эффекта равен 1, у `canceled` равен 0. Ключи содержат новый префикс на каждом запуске, поэтому старые строки в БД не попадают в эти запросы. Счётчик доказывает эффект внутри PostgreSQL, но не эффект во внешнем файле. Harness также не проверяет длительную работу без сбоев и нагрузку с реальной полезной работой.

### Нагрузка 100 POST/с

Скрипт `load-runs/load_test.py` отправляет 6 000 уникальных POST за 60 секунд. Для честного сравнения дождитесь, пока предыдущая очередь опустеет. Запустите его сначала с восемью, затем с 64 воркерами:

```bash
docker compose up -d --scale worker=8 worker
python3 load-runs/load_test.py
RUN=$(cat load-runs/current)
cat "load-runs/$RUN/summary.txt"
cat "load-runs/$RUN/queue.csv"

# Дождитесь нуля перед следующим прогоном:
docker compose exec -T db psql -U souptask -d souptask -c \
  "SELECT count(*) FROM jobs WHERE status IN ('queued','running');"
docker compose up -d --scale worker=64 worker
python3 load-runs/load_test.py
RUN=$(cat load-runs/current)
cat "load-runs/$RUN/summary.txt"
cat "load-runs/$RUN/queue.csv"
```

Если запрос перед вторым прогоном показывает ненулевую очередь, повторяйте его до нуля. Каждый запуск создаёт `load-runs/<run>/`:

| Файл | Что смотреть |
| --- | --- |
| `requests.csv` | Время отправки и ответа, задержка, HTTP-код и тело каждого запроса. |
| `queue.csv` | Снимки примерно каждые пять секунд: `elapsed_s,total,queued,running,succeeded,failed`. Рост `queued` означает, что воркеры не успевают за входящим потоком. |
| `summary.txt` | Фактический RPS, число ответов 200, ошибки, задержки p50/p95/p99 и последний снимок очереди. |

Последний снимок может содержать задачи `running`; это не обязательно зависание. Проверьте их позже по префиксу последнего запуска:

```bash
RUN=$(cat load-runs/current)
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT status, count(*) FROM jobs
WHERE idempotency_key LIKE '$RUN-%'
GROUP BY status ORDER BY status;"
```

Это тест 100 **новых задач** в секунду с имитацией работы по 500 мс. Он не измеряет смешанный поток GET и POST или производительность реальной задачи.

<a id="ru-structure"></a>
## Русский: структура проекта

| Путь | Назначение |
| --- | --- |
| `api/proto/jobs/v1/jobs.proto` | gRPC-контракт и типы запросов/ответов. |
| `api/http.yaml` | HTTP-маршруты для grpc-gateway. |
| `pkg/api/jobs/v1/` | Сгенерированный публичный Go API для импорта другими проектами. |
| `cmd/SoupWebApplication/`, `cmd/SoupWorker/` | Отдельные точки входа API и воркера. |
| `internal/app/` | Обработчики четырёх RPC и преобразование ошибок/ответов. |
| `internal/jobs/` | Состояния задач, модели и интерфейсы хранилища. |
| `internal/storage/postgres/` | SQL-операции с очередью и интеграционные тесты. |
| `internal/worker/` | Цикл захвата, обработки и завершения задач. |
| `migrations/` | Миграция схемы PostgreSQL для Goose. |
| `config/config.go`, `tools/config/` | Сгенерированные типизированные ключи и их генератор; число локальных worker-контейнеров задаётся Compose, а не этим Go-файлом. |
| `tools/handlergen/` | Генерация отсутствующих каркасов обработчиков в `internal/app/`. |
| `deploy/docker/`, `deploy/k8s/`, `compose.yaml` | Dockerfile, Helm chart и локальный Compose. `rtc.worker_count.value` в Helm задаёт число реплик worker Deployment. |
| `chaos_test.py`, `chaos-runs/` | Chaos harness и его результаты. |
| `load-runs/load_test.py`, `load-runs/<run>/` | Нагрузочный тест и файлы каждого прогона. |

<a id="en-quick"></a>
## English: quick guide

Run these commands in a WSL terminal from the repository root. You need Docker Desktop with WSL integration, Docker Compose, `curl`, and Python 3. Code generation and Go tests need Go 1.27.3+ and GNU Make. Migrations use [Goose](https://github.com/pressly/goose); `make tools` does not install it.

```bash
mkdir -p .tools/bin
GOBIN="$PWD/.tools/bin" go install github.com/pressly/goose/v3/cmd/goose@v3.30.0
docker compose up -d --wait db
.tools/bin/goose -dir migrations postgres \
  'postgres://souptask:souptask@127.0.0.1:5433/souptask?sslmode=disable' up
docker compose up -d --build
docker compose ps
curl -i 'http://127.0.0.1:8080/v1/jobs?page_size=1'
```

Apply the migration once to a new database. On later runs, Goose checks its version table. Do not run the entire SQL migration file through `psql`: it contains both `Up` and `Down`. Compose starts four workers by default. HTTP listens on `127.0.0.1:8080`, gRPC on `127.0.0.1:9090`, and PostgreSQL on `127.0.0.1:5433`. `docker compose down` stops containers and keeps the database volume.

For local code generation:

```bash
make tools
make generate
make lint
make build
```

`make generate` updates `pkg/api` and the generated `config/config.go`. The handler generator creates missing files in `internal/app` and does not overwrite existing handlers. In Docker Compose, change the worker process count with `docker compose up -d --scale worker=8 worker`. In Kubernetes, the replica count comes from `deploy/k8s/values.yaml` and changes on `helm upgrade`; this is not a live update of Go configuration.

<a id="en-tests"></a>
## English: seven manual tests

Start the service first. Run this setup once in the same terminal as the test commands. Each test uses a unique key, so existing database rows do not interfere.

```bash
BASE='http://127.0.0.1:8080'
RUN="manual-$(date +%s%N)"
```

### 1. Create and fetch a job

```bash
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-create\",\"payload\":{\"kind\":\"demo\"}}")
printf '%s\n' "$created"
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i "$BASE/v1/jobs/$ID"
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT status, attempt_count, job_effects_count FROM jobs WHERE id=$ID;"
```

Expect HTTP 200. The create response usually shows `QUEUED`; by the time GET runs, the job may already be `RUNNING` or `SUCCEEDED`. After processing, its status is `SUCCEEDED` and `job_effects_count=1`. Repeat the last two commands after a second if necessary.

### 2. Retry with the same key

```bash
for attempt in 1 2; do
  curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-duplicate\",\"payload\":{}}"
  echo
done
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT count(*) FROM jobs WHERE idempotency_key='$RUN-duplicate';"
```

Expect the same ID in both responses and one database row. The status may change between requests if a worker finishes the job.

### 3. Invalid inputs

```bash
curl -i "$BASE/v1/jobs/abc"
curl -i "$BASE/v1/jobs/9223372036854775807"
curl -i -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' -d '{"payload":{}}'
```

Expect HTTP 400 for an invalid ID, HTTP 404 for an unknown job, and HTTP 400 for a missing `idempotencyKey`.

### 4. List pagination

```bash
for n in 1 2 3; do
  curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-page-$n\",\"payload\":{}}" >/dev/null
done
page=$(curl -fsS "$BASE/v1/jobs?page_size=2")
printf '%s\n' "$page"
token=$(printf '%s' "$page" | python3 -c 'import json,sys; print(json.load(sys.stdin)["nextPageToken"])')
curl -iG --data-urlencode 'page_size=2' --data-urlencode "page_token=$token" "$BASE/v1/jobs"
```

The first page contains two jobs and a `nextPageToken`; the second continues without repeating those IDs. The list includes jobs from earlier runs.

### 5. Cancel a queued job

```bash
docker compose stop worker
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-cancel\",\"payload\":{}}")
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i -X POST "$BASE/v1/jobs/$ID:cancel"
docker compose start worker
docker compose exec -T db psql -U souptask -d souptask \
  -c "SELECT status, job_effects_count FROM jobs WHERE id=$ID;"
```

Expect `canceled` and zero effects. If the check is interrupted, start the workers again with `docker compose start worker`.

### 6. Cancel a completed job

```bash
created=$(curl -fsS -X POST "$BASE/v1/jobs" -H 'Content-Type: application/json' \
  -d "{\"idempotencyKey\":\"$RUN-finished\",\"payload\":{}}")
ID=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job"]["id"])')
curl -i "$BASE/v1/jobs/$ID"
```

Repeat GET until the status is `JOB_STATUS_SUCCEEDED`. Then run:

```bash
curl -i -X POST "$BASE/v1/jobs/$ID:cancel"
curl -i "$BASE/v1/jobs/$ID"
```

Both responses should keep `SUCCEEDED`: a completed job is not canceled retroactively.

### 7. Create jobs in parallel

```bash
for n in $(seq 1 64); do
  curl -sS -o /dev/null -w '%{http_code}\n' -X POST "$BASE/v1/jobs" \
    -H 'Content-Type: application/json' \
    -d "{\"idempotencyKey\":\"$RUN-parallel-$n\",\"payload\":{}}" &
done
wait
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT count(*) AS total,
       count(*) FILTER (WHERE status='succeeded') AS succeeded,
       count(*) FILTER (WHERE job_effects_count > 1) AS repeated_effects
FROM jobs WHERE idempotency_key LIKE '$RUN-parallel-%';"
```

Expect 64 HTTP 200 responses and 64 database rows. Workers need time to process them: repeat the SQL query later to see `succeeded=64` and `repeated_effects=0`.

<a id="en-extended"></a>
## English: extended tests

### Automated Go tests

`internal/storage/postgres/silent_integration_test.go` connects to a real database, so `go test ./...` needs a separate test database and `TEST_DATABASE_URL`. Create it once; do not use the main database:

```bash
docker compose exec -T db psql -U souptask -d postgres -c 'CREATE DATABASE souptask_test;'
.tools/bin/goose -dir migrations postgres \
  'postgres://souptask:souptask@127.0.0.1:5433/souptask_test?sslmode=disable' up
TEST_DATABASE_URL='postgres://souptask:souptask@127.0.0.1:5433/souptask_test?sslmode=disable' go test ./...
```

The tests cover concurrent claims of one job, stale workers after lease expiry, and cancellation versus completion. On later runs, reuse `souptask_test` without creating it again.

### Chaos harness

Do not run chaos and load tests at the same time. Wait for the queue to empty and start eight workers:

```bash
docker compose up -d --scale worker=8 worker
docker compose ps -q worker | wc -l
docker compose exec -T db psql -U souptask -d souptask -c \
  "SELECT count(*) FROM jobs WHERE status IN ('queued','running');"
python3 chaos_test.py
```

The script creates 10,000 jobs, sends 1,000 repeated POST requests and 500 cancellations. About every two seconds it sends `kill -9` to a random worker process through a restricted helper container with `--pid=host`. `docker kill` is unsuitable here because Docker treats it as a manual stop and does not apply `restart: unless-stopped`. The script continues until `pending=0`; let it finish if you need a complete run.

Each run creates `chaos-runs/<run>/`. `chaos-runs/current` points to the latest run:

| File | What to read |
| --- | --- |
| `requests.log` | Raw POST and cancellation responses; HTTP failures have a status code or `ERROR`. |
| `kills.log` | Selected worker, PID, and `kill -9` result; `exit=0` means the signal was sent successfully. |
| `harness_summary.txt` | A successful run has `request_errors=0`, `duplicate_id_mismatches=0`, `kills>0`, `pending=0`. An interrupted run does not create this file. |
| `invariants.log`, `effects.log` | Produced by the SQL checks below. |

```bash
RUN=$(cat chaos-runs/current)
cat "chaos-runs/$RUN/harness_summary.txt"
tail -n 10 "chaos-runs/$RUN/kills.log"
find chaos-runs -mindepth 2 -maxdepth 2 -name harness_summary.txt | wc -l
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT count(*) AS total,
       count(*) FILTER (WHERE status IN ('queued','running')) AS pending,
       count(*) FILTER (WHERE job_effects_count > 1
         OR (status='succeeded' AND job_effects_count <> 1)
         OR (status IN ('canceled','failed') AND job_effects_count <> 0)
         OR (status='canceled' AND NOT cancel_requested)) AS inconsistent
FROM jobs WHERE idempotency_key LIKE '$RUN-%';" | tee "chaos-runs/$RUN/invariants.log"
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT status, job_effects_count, count(*) AS jobs
FROM jobs WHERE idempotency_key LIKE '$RUN-%'
GROUP BY status, job_effects_count ORDER BY status, job_effects_count;" | tee "chaos-runs/$RUN/effects.log"
```

The `find` command counts completed runs with a summary file; keep `requests.log`, `kills.log`, `harness_summary.txt`, and the SQL results for the report. Expect `total=10000`, `pending=0`, and `inconsistent=0` in a successful run. `succeeded` jobs have one effect and `canceled` jobs have none. Each run uses a new key prefix, so these queries exclude older rows. The counter proves an effect inside PostgreSQL, not an effect in an external file. The harness also does not test long periods without faults or a real job workload.

### Load test at 100 POST/s

`load-runs/load_test.py` sends 6,000 unique POST requests in 60 seconds. For a fair comparison, wait for the previous queue to empty. Run with eight workers, then with 64:

```bash
docker compose up -d --scale worker=8 worker
python3 load-runs/load_test.py
RUN=$(cat load-runs/current)
cat "load-runs/$RUN/summary.txt"
cat "load-runs/$RUN/queue.csv"

# Wait for zero before the next run:
docker compose exec -T db psql -U souptask -d souptask -c \
  "SELECT count(*) FROM jobs WHERE status IN ('queued','running');"
docker compose up -d --scale worker=64 worker
python3 load-runs/load_test.py
RUN=$(cat load-runs/current)
cat "load-runs/$RUN/summary.txt"
cat "load-runs/$RUN/queue.csv"
```

If the query before the second run is nonzero, repeat it until it returns zero. Each run creates `load-runs/<run>/`:

| File | What to read |
| --- | --- |
| `requests.csv` | Send and completion times, latency, HTTP status, and response body for every request. |
| `queue.csv` | Snapshots about every five seconds: `elapsed_s,total,queued,running,succeeded,failed`. Growing `queued` means workers cannot keep up with new jobs. |
| `summary.txt` | Actual send rate, HTTP 200 count, errors, p50/p95/p99 latency, and final queue snapshot. |

The last snapshot may still contain `running` jobs; this does not necessarily mean they are stuck. Check them later using the latest run prefix:

```bash
RUN=$(cat load-runs/current)
docker compose exec -T db psql -U souptask -d souptask -c "
SELECT status, count(*) FROM jobs
WHERE idempotency_key LIKE '$RUN-%'
GROUP BY status ORDER BY status;"
```

This measures 100 **new jobs** per second with a simulated 500 ms job. It does not measure a mixed GET/POST workload or real job execution.

<a id="en-structure"></a>
## English: project structure

| Path | Purpose |
| --- | --- |
| `api/proto/jobs/v1/jobs.proto` | gRPC contract and request/response types. |
| `api/http.yaml` | HTTP route mappings for grpc-gateway. |
| `pkg/api/jobs/v1/` | Generated public Go API that other projects can import. |
| `cmd/SoupWebApplication/`, `cmd/SoupWorker/` | Separate API and worker entry points. |
| `internal/app/` | Four RPC handlers and response/error mapping. |
| `internal/jobs/` | Job states, models, and storage interfaces. |
| `internal/storage/postgres/` | SQL queue operations and integration tests. |
| `internal/worker/` | Claim, process, and finish loop. |
| `migrations/` | Goose migration for the PostgreSQL schema. |
| `config/config.go`, `tools/config/` | Generated typed keys and their generator; local worker container count comes from Compose, not this Go file. |
| `tools/handlergen/` | Generates missing handler skeletons in `internal/app/`. |
| `deploy/docker/`, `deploy/k8s/`, `compose.yaml` | Dockerfile, Helm chart, and local Compose setup. `rtc.worker_count.value` controls worker Deployment replicas in Helm. |
| `chaos_test.py`, `chaos-runs/` | Chaos harness and its results. |
| `load-runs/load_test.py`, `load-runs/<run>/` | Load generator and per-run output files. |
