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


Skills:
---
name: go-service-core
description: Create or improve the foundation of a Go service repository, including package boundaries, entry points, API generation, local tooling, build workflow, and idiomatic code style. Use for a service core or repository scaffold, not for implementing domain-specific business logic, database migrations, or runtime configuration generators.
---

# Go service core

Create a small, runnable foundation for a Go service. Adapt to the repository's existing architecture and the user's chosen protocols and deployment tools. Establish clear boundaries without adding packages or tooling that have no current caller.

## Start with the repository

- Inspect the module path, Go version, entry points, API contracts, generators, build commands, deployment files, and Git state. Preserve useful conventions already present.
- Confirm what the user means by core for this task. A discussion request calls for a proposed structure. An implementation request calls for working files and verification. Respect requests to leave editing to the user.
- Record the intended request path through the service and the responsibility of each layer before creating directories.

## Package boundaries

Use these responsibilities as a guide, not a mandatory directory template.

- cmd/<service> owns process startup, dependency wiring, signal handling, and shutdown. Keep business rules out of main.
- internal/transport/<protocol> converts requests and responses, validates protocol-level input, and maps application errors to protocol errors. A gateway is an adapter to the same application operations.
- internal/app owns use cases and coordinates domain behavior through narrow interfaces. Define interfaces where they are consumed when this keeps dependencies pointing inward.
- internal/domain holds shared entities, states, and transition rules when they are substantial enough to justify a package. Keep transport types out of it.
- internal/storage/<driver> implements persistence contracts and owns transactions. Add a storage abstraction when there is a real consumer or a planned implementation boundary.
- pkg is for code deliberately importable by other modules, such as a public generated API. Keep service-private code under internal.

Generated API code contains data types, clients, server interfaces, and gateway bindings. Handwritten handlers implement those interfaces in the service. Configure the generator's output path explicitly. Do not place generated files in an application package merely because handlers call that package.

## Go style

- Use small, named types and ordinary constructors. Pass dependencies explicitly. Avoid global mutable service state and interfaces created only to imitate a layered diagram.
- Propagate context across request and storage boundaries. Return errors with useful context and map them at the transport boundary. Keep validation at the layer that knows the rule.
- Make lifecycle behavior clear. Startup failures should fail visibly; servers and workers should stop cleanly when cancellation is requested.
- Apply gofmt. Use descriptive package and identifier names. Prefer direct control flow and readable code over generic helpers, unnecessary wrappers, and speculative extension points.
- Implement a coherent vertical slice through the real entry point before expanding the skeleton. A compiling repository full of unimplemented handlers is only a scaffold, so describe it as such.

## Tooling and repository hygiene

- Pin generator and plugin versions so regeneration is repeatable. Document the exact generation command and where its outputs belong.
- Provide a small build interface such as Make targets for tools, generate, lint, test, and build when it helps the repository. Keep targets independent where possible so one missing generator does not block unrelated work.
- Keep downloaded tools and compiled binaries outside tracked source. Use Git ignore rules for local artifacts. Keep build-context exclusions in a tracked Docker ignore file when Docker is used.
- Add Docker, Compose, Helm, CI, or protocol tooling only when requested or needed to run the service. Keep local addresses and container addresses appropriate to their environments.
- Document prerequisites and one reliable local run path. Do not claim a command works until it has been run successfully in the available environment.

## Verify the core

Check generated-code reproducibility, formatting, build, and the normal request path. Run relevant tests or focused smoke checks for implemented behavior. Report which commands passed, which could not run, and whether endpoints perform work or still return placeholders.

