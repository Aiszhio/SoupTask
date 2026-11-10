# SoupWebApplication

Каркас Job Queue для задания Soup Backend Engineer. gRPC и HTTP через grpc-gateway используют один обработчик в internal/jobs. Пока все RPC возвращают Unimplemented (HTTP 501). PostgreSQL, очередь и воркеры появятся следующим этапом.

## Локальная разработка в WSL

Нужны Go 1.27.3+, GNU Make и доступ к Go module proxy.

~~~sh
cd /mnt/c/Users/Maxim/Desktop/SoupTask
make tools
make generate
make lint
make test
make build
./bin/SoupWebApplication
~~~

make tools устанавливает Buf и protoc-плагины в локальную .tools/bin. make generate обновляет Go-файлы в pkg/api из контракта api/proto и маршрутов api/http.yaml.

По умолчанию gRPC слушает 127.0.0.1:9090, HTTP — 127.0.0.1:8080. Локальный .env можно загрузить через set -a; . ./.env; set +a перед запуском.

| Операция | HTTP |
| --- | --- |
| Создать | POST /v1/jobs, ключ передаётся в поле JSON idempotencyKey |
| Получить статус | GET /v1/jobs/{job_id} |
| Список | GET /v1/jobs?page_size=20&page_token=... |
| Отменить | POST /v1/jobs/{job_id}:cancel |

## Docker Compose

Из корня проекта запустите docker compose up --build. Compose использует deploy/docker/Dockerfile и публикует HTTP на 127.0.0.1:8080, gRPC на 127.0.0.1:9090.

## Kubernetes

Helm chart находится в deploy/k8s. В deploy/k8s/values.yaml задаются образ, число реплик, порты и rtc.workerCount. Шаблон Deployment передаёт число воркеров контейнеру через переменную WORKER_COUNT; отдельный конфигурационный сервис и генерация Go-констант не нужны. Обработчики очереди пока не реализованы, поэтому приложение ещё не использует WORKER_COUNT.

После публикации образа укажите свой image.repository и image.tag в values-файле и выполните helm upgrade --install souptask ./deploy/k8s.
