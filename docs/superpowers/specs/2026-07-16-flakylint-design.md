# flakylint — дизайн

Дата: 2026-07-16
Статус: утверждается

## 1. Цель и позиционирование

**flakylint** — статический анализатор (набор линтеров на `golang.org/x/tools/go/analysis`),
который находит известные флаки-паттерны в Go-тестах на этапе PR — до того, как тест
начал флакать в CI.

Позиционирование: первый линтер, для которого флакиность — категория, а не побочный
эффект. Комплемент к runtime-детекторам (flakeguard, Mergify Test Insights,
flakiness.io): они ловят флаки постфактум перезапусками, flakylint — предотвращает.

Не дублируем существующие линтеры: paralleltest (пропущенный `t.Parallel`),
tparallel, thelper, testifylint, usetesting, bodyclose (прод-код). Все проверки MVP
не покрыты ни одним линтером из golangci-lint.

- Module path: `github.com/malikov73/flakylint`
- Лицензия: MIT
- Go: 1.24+ (для сборки самого линтера; анализируемый код — любой версии)
- Язык README и диагностик: английский

## 2. Скоуп MVP: 4 анализатора

Философия: **лучше 4 точных проверки, чем 8 шумных**. Каждая проверка консервативна —
сомневаемся → молчим. Все проверки работают только по `_test.go` файлам.

### 2.1 `httptestclose`

Ловит `httptest.NewServer` / `NewTLSServer` / `NewUnstartedServer` без закрытия —
утечка порта и горутины, гонка GC-финализатора с connection pool
(реальный инцидент: google/go-github#4210).

- **Репортим**, когда на переменной сервера нет вызова `.Close` нигде в объемлющей
  функции (прямой вызов, `defer`, внутри `t.Cleanup(...)`).
- **Молчим**, если переменная «убегает»: возвращается из функции, передаётся
  аргументом в другую функцию (кроме `t.Cleanup(srv.Close)`), присваивается в поле
  структуры или package-level переменную.
- **SuggestedFix**: вставить `t.Cleanup(srv.Close)` после создания, если в скоупе
  есть `testing.TB`; иначе `defer srv.Close()`.

### 2.2 `sleepassert`

Ловит `time.Sleep` в теле теста — синхронизацию через реальное время
(гонка со шедулером, зависимость от нагрузки CI-раннера).

- **Репортим**: statement `time.Sleep(d)` непосредственно в теле Test-функции или
  в литерале сабтеста `t.Run(...)`.
- **Молчим**: sleep внутри `for`-цикла (polling/retry — отдельная, менее флачная
  история); внутри synctest-бабла (`synctest.Run` / `synctest.Test`); `d == 0`.
- **Диагностика** предлагает `testing/synctest` (Go 1.24+) или синхронизацию через
  канал/WaitGroup. Автофикса в MVP нет — переписывание на synctest нетривиально.

### 2.3 `parallelglobal`

Ловит запись в package-level переменную из теста с `t.Parallel()` —
data race между параллельными тестами, который проявляется только под нагрузкой.

- **Репортим**: тест (или сабтест) вызывает `t.Parallel()`, и в его теле есть запись
  в идентификатор, резолвящийся в package-level `var`: прямое присваивание,
  compound (`+=`), `x.f = ...`, `x[i] = ...`, `x++/x--`.
- **Ограничение (документируем)**: не анализируем защиту мьютексами — запись в
  глобал из параллельного теста почти всегда ошибка независимо от синхронизации.

### 2.4 `exitfatal`

Ловит `os.Exit` / `log.Fatal*` в тестах — процесс умирает мимо `t.Cleanup`/`defer`,
teardown (контейнеры, временные файлы, серверы) не выполняется и отравляет
следующие тесты.

- **Репортим**: вызовы `os.Exit`, `log.Fatal`, `log.Fatalf`, `log.Fatalln`
  (package-level функции stdlib `log`) в Test/Benchmark/Fuzz-функциях и сабтестах.
- **Отдельный кейс TestMain**: репортим `os.Exit(...)` в `TestMain`, только если в
  нём есть `defer`-стейтменты (они молча пропускаются) — канонический
  `os.Exit(m.Run())` без defer'ов валиден, молчим.
- **SuggestedFix**: `log.Fatal*` → `t.Fatal*`, когда `t` в скоупе.

### Вне скоупа MVP (roadmap, v0.2+)

- Ассерции, зависящие от порядка итерации map.
- Незакрытый `resp.Body` в тестах (следим за golang/go#75902 — vet может закрыть сам).
- Контекст с таймаутом в параллельных сабтестах (сейчас — paralleltestctx; возможно,
  переосмыслить и поглотить).
- Проверка покрытия тестов goleak'ом.
- Автофикс sleep → synctest.

## 3. Архитектура

```
flakylint/
├── cmd/flakylint/main.go        # multichecker.Main со всеми анализаторами
├── analyzers/
│   ├── httptestclose/
│   │   ├── httptestclose.go     # var Analyzer *analysis.Analyzer
│   │   └── testdata/src/a/      # для analysistest, want-комментарии
│   ├── sleepassert/
│   ├── parallelglobal/
│   └── exitfatal/
├── internal/testfuncs/          # общие хелперы: isTestFile, isTestFunc,
│                                # поиск testing.TB в скоупе, детект t.Parallel,
│                                # детект литералов сабтестов t.Run
├── docs/superpowers/specs/
├── .github/workflows/ci.yml
└── .goreleaser.yml
```

Принципы:

- Каждый анализатор — самостоятельный пакет с экспортом `Analyzer`, пригодный для
  `singlechecker` и `go vet -vettool` по отдельности. Это требование для будущей
  интеграции в golangci-lint.
- Общий обход AST через `passes/inspect` (стандартный `Requires` для производительности).
- Никаких зависимостей кроме `golang.org/x/tools`.

## 4. Конфигурация

Zero-config по умолчанию. Отключение анализатора — штатными флагами multichecker
(`flakylint -sleepassert=false ./...`). Кастомных флагов в MVP нет; дефолты
подобраны так, чтобы не бесить (требование golangci-lint).

## 5. Тестирование

- Юнит: `analysistest.Run` + `analysistest.RunWithSuggestedFixes` на testdata с
  `// want`-комментариями для каждого анализатора; кейсы «не репортим» обязательны
  для каждой anti-FP эвристики.
- Self-lint: flakylint и golangci-lint на самом себе в CI.
- Корпус-прогон (ручной, перед релизом): скрипт гоняет бинарь по клонам популярных
  репозиториев (kubernetes, grafana, prometheus, testcontainers-go, go-github) —
  замер FP-rate вручную по выборке; цель < 5%. Результаты — материал для запуска.

## 6. Дистрибуция и релизы

1. Бинарь через goreleaser (macOS/Linux/Windows), semver с v0.1.0.
2. `go install github.com/malikov73/flakylint/cmd/flakylint@latest`.
3. `go vet -vettool=$(which flakylint)`.
4. Через 1–2 месяца жизни проекта — PR в golangci-lint (требования: go/analysis,
   функциональные тесты в `pkg/golinters/flakylint/testdata/`, регистрация в
   `builder_linter.go`, не-дубликат существующих — обеспечено скоупом).
5. Позже: GitHub Action-обёртка.

README: на английском, каждая проверка — с реальным примером флака и ссылкой на
инцидент в известном проекте (go-github#4210 и т.п.).

## 7. Критерии успеха MVP

- 4 анализатора с тестами, FP-rate < 5% на корпусе реальных репозиториев.
- ≥ 3 принятых upstream PR с фиксами, найденными линтером, в известных проектах.
- Пост-запуск (Show HN / r/golang / Golang Weekly) и заявка в golangci-lint.
