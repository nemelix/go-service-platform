[English](README.md) | **Русский**

# go-service-platform

[![CI/CD](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml)

Небольшой сервис на Go, на котором я отрабатываю DevOps-практики: контейнеризацию, CI/CD, деплой на собственный сервер, HTTPS и наблюдаемость.

Работает здесь: https://64-188-81-197.sslip.io/healthz

## Архитектура

```mermaid
flowchart LR
    dev(Разработчик) -->|Pull Request| checks

    subgraph checks[Проверки, параллельно]
        test[go test<br/>с PostgreSQL]
        lint[golangci-lint]
        hado[hadolint]
        sc[shellcheck]
        gl[gitleaks]
    end

    checks --> build[docker build]
    build --> trivy{Trivy<br/>HIGH/CRITICAL?}
    trivy -->|найдено| stop((стоп))
    trivy -->|"чисто (только main)"| reg[(ghcr.io<br/>тег = sha)]
    reg --> deploy[Деплой<br/>SSH, forced command]
    deploy -->|docker compose pull| vps
    reg -.->|ежедневно| scan[Плановый<br/>скан Trivy]
    user(Пользователь) -->|HTTPS| nginx

    subgraph vps[VPS]
        nginx[Nginx<br/>TLS, reverse proxy] --> app[Go-приложение]
        app --> db[(PostgreSQL)]
    end
```

## Приложение

Счетчик посещений, который хранится в PostgreSQL.

| Эндпоинт | Назначение |
|---|---|
| `GET /healthz` | liveness: процесс жив (в базу не ходит) |
| `GET /readyz` | readiness: проверяет базу, при ее недоступности отвечает 503 |
| `GET /api/visits` | текущее число посещений |
| `POST /api/visits` | добавляет посещение и возвращает новое число |
| `GET /metrics` | метрики Prometheus (закрыт в Nginx, доступен только внутри сети Docker) |

Настройки приходят из переменных окружения ([12-factor](https://12factor.net/ru/config): настройки живут вне кода), поэтому один и тот же образ работает на ноутбуке, на VPS и позже в Kubernetes:

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `APP_ADDR` | `:8080` | адрес, на котором слушает HTTP-сервер |
| `DATABASE_URL` | нет, обязательна | строка подключения к PostgreSQL; если база недоступна, приложение при старте завершается с ошибкой |

Как оно себя ведет:

- **Логи** в структурированном JSON (`log/slog`) в stdout: одна строка на событие и на запрос, поэтому сборщик логов разбирает поля, а не регулярки.
- **Graceful shutdown:** по SIGTERM или SIGINT сервер перестает принимать новые соединения, дорабатывает текущие запросы (до 10 секунд) и выходит с кодом 0.
- **Миграции** это пронумерованные SQL-файлы (migrations/, goose), которые шаг за шагом меняют схему базы; база помнит, какие уже применены. Они вшиты в бинарник и применяются при старте, до того как сервер начнет слушать порт.
- **Метрики:** `http_requests_total`, `http_request_duration_seconds` (метки: code, method) и `visits_total`. Метки `path` намеренно нет: каждое уникальное значение создает новый временной ряд (отдельную хранимую последовательность чисел), и клиент мог бы завалить мониторинг случайными URL.
- **Счетчик без потерь:** посещение добавляется одной командой `INSERT ... ON CONFLICT DO UPDATE`, поэтому одновременные запросы не затирают друг друга. Интеграционный тест проверяет, что 20 параллельных запросов дают ровно +20.

## Стек и почему

- **Go** - собирается в один бинарник без зависимостей, образ получается ~19 МБ. На Go написаны Docker, Kubernetes и Terraform, хотелось познакомиться с языком.
- **PostgreSQL** - стандартная реляционная база, легко запускается в контейнере.
- **pgx** - нативный драйвер PostgreSQL с пулом соединений, без ORM.
- **goose** - миграции обычным SQL с секциями Up/Down, вшитые в бинарник, поэтому distroless-образу не нужны дополнительные файлы.
- **log/slog** - структурные логи из стандартной библиотеки, без лишней зависимости.
- **Клиент Prometheus** - фактический стандарт для метрик.
- **Docker + Compose** - стандарт, а для одного сервера Compose достаточно. Kubernetes на данный момент избыточен.
- **distroless** - базовый образ без shell и пакетного менеджера. scratch слишком пустой (нет CA-сертификатов и пользователей), в alpine есть лишнее.
- **GitHub Actions + ghcr.io** - код уже на GitHub, CI и registry в одном месте, для публичного репозитория бесплатно. Версии actions и базовых образов обновляет Dependabot.
- **Nginx + certbot** - Caddy или Traefik выпустили бы сертификат сами, но я хотел руками разобраться, как работают ACME и TLS.
- **sslip.io** - DNS по IP-адресу, позволяет получить настоящий сертификат Let's Encrypt без покупки домена

Сервер: VPS с Ubuntu 24.04, 2 vCPU / 4 GB.

## CI/CD

| Событие | Что происходит |
|---|---|
| Pull Request | проверки параллельно → сборка образа → Trivy |
| Merge в `main` | то же + публикация образа в ghcr.io → деплой на сервер → smoke test |
| Каждый день (06:30 UTC) | Trivy сканирует опубликованный образ по свежей базе уязвимостей |

Проверки на каждом PR:

| Проверка | Что ищет |
|---|---|
| `go test -race` | сломанную логику и гонки данных; тесты, которым нужна база, выполняются на временной PostgreSQL, которую CI запускает рядом с джобой |
| golangci-lint | ошибки в Go-коде, включая непроверенные ошибки (`errcheck`) и все, что находит `go vet` |
| hadolint | плохие практики в Dockerfile |
| ShellCheck | ошибки в shell-скриптах |
| gitleaks | секреты (токены, ключи) во всей истории git |
| Trivy | уязвимости HIGH/CRITICAL в собранном образе, для которых уже есть исправление |

- Каждый образ помечен sha коммита - по тегу понятно, какой код запущен, и можно откатиться на любую версию.
- Деплой: CI подключается по SSH и передает sha, скрипт на сервере записывает тег в `.env` и обновляет только контейнер приложения. Compose-файл, конфиг Nginx и файлы с секретами пока управляются на сервере вручную.
- После деплоя CI проверяет `/healthz` по HTTPS.
- `main` защищена: изменения попадают только через PR, где прошли все проверки. Каждая проверка добавлена в ruleset отдельно.
- Тесты без базы используют подделку хранилища за интерфейсом, поэтому они быстрые. Тесты с базой пропускаются, если `DATABASE_URL` не задан, и выполняются в CI.

## Безопасность

- **Образ:** distroless без shell и пакетного менеджера, запуск не от root.
- **Деплой:** отдельный пользователь deploy без пароля и sudo. Его SSH-ключ ограничен через restrict и forced command - с ним можно только запустить скрипт деплоя.
- **Скрипт деплоя:** принадлежит root, принимает только 40-символьный sha коммита.
- **CI:** ключ сервера проверяется через `known_hosts` (без `StrictHostKeyChecking=no`), секреты лежат в окружении `production`, у токена минимальные права.
- **Сеть:** наружу открыт только Nginx (80/443). Приложение и PostgreSQL доступны только внутри сети Docker, у базы нет опубликованного порта.
- **Секреты базы:** пароль лежит в `db.env` и `app.env` на сервере и никогда не попадает в git; в репозитории хранятся только `*.env.example` с заглушками.
- **Ошибки:** при сбое API отвечает общим `internal error`, подробности попадают только в логи.
- **Метрики:** `/metrics` закрыт в Nginx (404), его будут забирать внутри сети Docker.
- **TLS:** только TLS 1.2/1.3, редирект с HTTP на HTTPS, автопродление сертификата.
- **Уязвимости:** Trivy не пускает в реестр образы с исправимыми HIGH/CRITICAL. Неисправленные (`ignore-unfixed`) сборку не блокируют: с ними пока ничего не сделать, а CI, который всегда красный, перестают читать. Опубликованный образ дополнительно сканируется каждый день.
- **Секреты в коде:** gitleaks сканирует всю историю git на каждом PR, а push protection на GitHub включена: пуш с токеном отклоняется еще до попадания в репозиторий.
- **Сторонние actions:** actions закреплены по sha коммита, а не по тегу: тег можно перевесить на другой код, коммит - нет. Actions от самого GitHub (`actions/*`) оставлены на тегах: CI и так доверяет GitHub. Обновления sha присылает Dependabot.

## Проблемы, с которыми я столкнулся

- **На сервере работала старая версия, хотя деплой прошел.** На сервере и в registry лежали разные образы с одним тегом `latest`. Перешел на теги по sha коммита - сразу видно, что запущено, и можно откатиться.
- **Порт контейнера был доступен из интернета, хотя в ufw он закрыт.** Docker пробрасывает порты через DNAT, такой трафик идет по цепочке FORWARD и до правил ufw в INPUT не доходит. Теперь наружу публикуется только Nginx.
- **Контейнер падал с `exec /server: no such file or directory`, хотя файл на месте.** Бинарник собрался динамическим (без `CGO_ENABLED=0`), а в distroless нет libc и загрузчика. Нашел через `ldd` на этапе сборки.
- **Линтер нашел настоящий баг.** errcheck указал на непроверенную ошибку `http.ListenAndServe`: если порт занят, приложение молча завершалось с кодом 0. Теперь ошибка пишется в лог, а код выхода - 1.
- **PR можно было смержить, хотя линтер упал.** Упавший `lint` делал `build` пропущенным (skipped), а пропущенную обязательную проверку GitHub считает успешной. Сделал обязательной каждую джобу.
- **Trivy остановил публикацию образа.** HIGH-уязвимость в `crypto/x509` из стандартной библиотеки Go: сам я x509 не вызываю, но он вшит через `net/http`. Поднял Go до патч-версии с исправлением.
- **Смена `POSTGRES_PASSWORD` не меняла пароль.** Образ postgres читает эту переменную только при инициализации пустого каталога данных. Для существующего тома пришлось сменить пароль внутри базы (`ALTER USER`), обновить оба env-файла и пересоздать контейнеры (`restart` не перечитывает `env_file`). Успешный `psql` внутри контейнера тут ничего не доказывает, потому что локальные подключения там доверенные; настоящая проверка это подключение приложения по сети.

Подробности и другие истории: [docs/problems.ru.md](docs/problems.ru.md).

## Известные ограничения

- `POST /api/visits` публичный и без ограничения частоты. Это демонстрационный эндпоинт; ограничение можно добавить в Nginx (`limit_req`).
- `visits_total` живет в памяти процесса и после перезапуска начинается с нуля; настоящее число лежит в базе. Метрики показывают, как быстро и как меняется нагрузка (сколько посещений в минуту, растет ли их число), а точный итог хранит база.
- Compose-файл, конфиг Nginx и env-файлы копируются на сервер вручную. Это позже заменит Ansible.

## Структура репозитория

```
.
├── main.go                      # запуск, конфигурация, graceful shutdown, логи
├── server.go                    # HTTP-маршруты и обработчики
├── store.go                     # хранилище посещений в PostgreSQL
├── db.go                        # пул соединений
├── migrate.go                   # миграции goose, вшитые в бинарник
├── metrics.go                   # метрики Prometheus
├── migrations/                  # SQL-миграции
├── main_test.go, server_test.go # юнит-тесты (подделка хранилища)
├── store_test.go                # интеграционные тесты (настоящая PostgreSQL)
├── Dockerfile                   # multi-stage сборка
├── compose.dev.yml              # PostgreSQL для локальных тестов
├── docs/
│   ├── problems.md              # проблемы, с которыми я столкнулся, подробно
│   └── problems.ru.md           # то же по-русски
├── .github/
│   ├── workflows/
│   │   ├── ci.yml               # CI/CD: проверки → сборка + Trivy → деплой
│   │   └── scheduled-scan.yml   # ежедневный скан образа
│   └── dependabot.yml           # автообновление зависимостей
└── deploy/
    ├── docker-compose.yml       # приложение + PostgreSQL + Nginx + certbot
    ├── nginx/default.conf       # reverse proxy, TLS
    ├── deploy.sh                # скрипт деплоя на сервере
    ├── db.env.example           # шаблон для пароля базы
    └── app.env.example          # шаблон для DATABASE_URL
```

## Локальный запуск

Приложению нужна PostgreSQL. Одноразовая база для разработки лежит в `compose.dev.yml` (порт 5432 только на `127.0.0.1`, пароль `dev`):

```bash
docker compose -f compose.dev.yml up -d

export DATABASE_URL='postgres://app:dev@localhost:5432/app?sslmode=disable'
go run .                        # миграции применяются при старте

curl localhost:8080/healthz
curl -X POST localhost:8080/api/visits
curl localhost:8080/api/visits
curl localhost:8080/metrics
```

## Тесты

Без `DATABASE_URL` тесты, которым нужна база, пропускаются:

```bash
go test ./...
```

С базой для разработки (`-count=1` отключает кеш результатов).

Linux / macOS:

```bash
DATABASE_URL='postgres://app:dev@localhost:5432/app?sslmode=disable' go test -v -count=1 ./...
```

> [!IMPORTANT]
> **Предварительно необходимо установить ([Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/)).**

Windows (PowerShell). Переменная остается заданной в текущем терминале до его закрытия или пока вы не выполните `Remove-Item Env:DATABASE_URL`:

```powershell
$env:DATABASE_URL = 'postgres://app:dev@localhost:5432/app?sslmode=disable'
go test -v -count=1 ./...
```

### В Docker

Контейнер приложения должен видеть контейнер базы, поэтому запускаем их в одной сети Docker. Сеть (`go-service-platform-dev_default`) создается при запуске базы из `compose.dev.yml`, и внутри нее база доступна по имени `db`. Команды одинаковы на Linux, macOS и Windows (PowerShell):

```bash
docker compose -f compose.dev.yml up -d
docker build -t go-service-platform:dev .
docker run --rm -p 8080:8080 --network go-service-platform-dev_default -e DATABASE_URL="postgres://app:dev@db:5432/app?sslmode=disable" go-service-platform:dev
```

Остановить можно Ctrl+C. Проверка из второго терминала: `curl localhost:8080/readyz` (в PowerShell `curl.exe`).

## Первичная настройка сервера (пока вручную, до Ansible)

Нужен сервер с Docker и Compose. Пользователь `deploy` (без пароля и без sudo) владеет каталогом приложения и запускает `docker compose`. На сервере:

```bash
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG docker deploy
sudo mkdir -p /opt/go-service-platform
sudo chown deploy:deploy /opt/go-service-platform
```

Членство в группе `docker` фактически равно правам root, поэтому SSH-ключ этого пользователя ограничен: он может только запустить скрипт деплоя (`restrict` и forced command). Файлы `docker-compose.yml` и `nginx/default.conf` из каталога `deploy/` репозитория копируются в `/opt/go-service-platform` вручную.

Проверка, что все на месте:

```bash
id deploy && ls -ld /opt/go-service-platform
```

### Секреты и запуск
Файлы с секретами создаются один раз руками:

```bash
cd /opt/go-service-platform

# db.env: сгенерировать пароль (24 байта в hex)
echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)" | sudo tee db.env > /dev/null

# app.env: строка подключения строится из пароля в db.env
sed 's|^POSTGRES_PASSWORD=\(.*\)|DATABASE_URL=postgres://app:\1@db:5432/app?sslmode=disable|' db.env | sudo tee app.env > /dev/null

# владелец и права: читать может только deploy
sudo chown deploy:deploy db.env app.env
sudo chmod 600 db.env app.env

# запустить стек
docker compose up -d
docker compose ps            # db должен быть healthy
```

## Планы

- [ ] Наблюдаемость (метрики, дашборды, алерты, логи): Prometheus / VictoriaMetrics, Grafana, алерты
- [ ] Бэкапы: `pg_dump` в restic (программу для бэкапов) с проверенным восстановлением
- [ ] Ansible и OpenTofu - настройка сервера и инфраструктура как код
- [ ] Kubernetes (k3d), Helm, ArgoCD