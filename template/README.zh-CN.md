# Go 应用开发指南

[English](README.md) | 简体中文

本项目由 go-init 生成，使用 chi、pgx 和 godotenv，采用 Handler → Service → Repository 分层。数据库迁移使用 Goose，查询代码使用 sqlc 生成。

初始化只注册 `/health`，不预置业务表。`internal/template/` 提供可编译的分层占位代码；下面的业务开发示例需要按步骤添加，不会在初始化时自动创建。

## 1. 启动项目

初始化成功后，依赖、Goose、sqlc 和 PostgreSQL 已准备好。在生成项目的根目录执行：

```bash
go run ./cmd
```

访问 <http://localhost:8080/health>，响应为 `非常好👍`。应用默认监听 8080，数据库映射到本机 5432。

数据库停止后，可以重新启动：

```bash
docker compose up -d postgres
```

如果初始化时依赖下载未完成，先执行 `go mod download`。初始化失败后保留了已生成文件，应按错误提示完成剩余步骤，不要再次在该目录执行 go-init。

也可以通过容器运行数据库和 API：

```bash
docker compose up -d --build
```

本机运行和容器运行 API 时选择其中一种，避免同时占用 8080 端口。

## 2. 环境配置

生成项目只有 `.env`，没有 `.env.template`。默认配置如下：

```dotenv
DB_DSN="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
DB_DRIVER=postgres
DB_MIGRATION_DIR=./internal/adapters/postgresql/migrations

# Goose
GOOSE_DRIVER=postgres
GOOSE_DBSTRING=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
GOOSE_MIGRATION_DIR=./internal/adapters/postgresql/migrations
```

| 配置 | 用途 |
|---|---|
| `DB_DSN` | 应用数据库连接字符串 |
| `DB_DRIVER` | 数据库驱动名称 |
| `DB_MIGRATION_DIR` | 数据库迁移文件目录 |
| `GOOSE_DRIVER` | Goose 使用的数据库驱动 |
| `GOOSE_DBSTRING` | Goose 使用的数据库连接字符串 |
| `GOOSE_MIGRATION_DIR` | Goose 使用的迁移文件目录 |

默认数据库名、用户名和密码均为 `postgres`。应用通过 godotenv 加载 `.env`，Goose CLI 也会读取运行目录中的 `.env`；文档中的命令均在项目根目录执行。

Goose 配置在初始化时已经展开为实际值。修改数据库连接后，需要同步调整 `DB_DSN` 和 `GOOSE_DBSTRING`；修改驱动或迁移目录时也要同步对应的 Goose 配置。

本机应用连接 `localhost:5432`。API 容器由 `docker-compose.yaml` 配置连接 `postgres:5432`，其中 `postgres` 是 Compose 服务名。修改容器数据库配置时，同步修改 Compose 中的连接配置。

## 3. 目录与职责

```text
.
├── cmd/
│   ├── main.go
│   └── api.go
├── internal/
│   ├── template/
│   │   ├── types.go
│   │   ├── service.go
│   │   └── handlers.go
│   ├── adapters/postgresql/
│   │   ├── migrations/
│   │   │   └── .gitkeep
│   │   └── sqlc/
│   │       ├── queries.sql
│   │       ├── db.go
│   │       ├── querier.go
│   │       ├── models.go
│   │       └── template.go
│   ├── json/
│   └── env/
├── .env
├── .gitignore
├── .dockerignore
├── go.mod
├── go.sum
├── sqlc.yaml
├── Dockerfile
├── docker-compose.yaml
└── README.md
```

| 位置 | 职责 |
|---|---|
| `cmd/main.go` | 加载配置、连接数据库、启动应用 |
| `cmd/api.go` | 配置中间件、注入依赖、注册 HTTP 路由 |
| `internal/template/types.go` | 定义请求参数等业务类型 |
| `internal/template/service.go` | 定义 Service 接口，实现参数校验、业务规则和事务 |
| `internal/template/handlers.go` | 解析 HTTP 请求、调用 Service、处理错误和返回响应 |
| `internal/adapters/postgresql/migrations/` | 建表、字段、索引等数据库结构变更 |
| `internal/adapters/postgresql/sqlc/queries.sql` | 编写增删改查 SQL |
| `internal/adapters/postgresql/sqlc/*.go` | 数据库模型和查询方法；接入真实 SQL 后由 sqlc 管理生成文件 |
| `internal/json/` | 读取 JSON 请求、统一写入 JSON 成功响应 |
| `internal/env/` | 读取环境变量 |

现有 `repo.Template` 是空模型，`Queries.Template` 返回空列表，不执行 SQL。模板业务方法没有注册到路由，初始只能访问 `/health`。

## 4. 按顺序开发业务接口

下面用 `examples` 表和查询、创建接口演示完整流程。示例假设项目的 module 为 `my-app`。如果初始化时指定了其他模块路径，将 Go 示例中的 `my-app/` 替换为 `go.mod` 中的 module 路径，可通过 `go list -m` 查看。

### 4.1 创建迁移文件

```bash
goose -s create create_examples sql
```

`-s` 使用顺序编号，`create_examples` 是本次迁移的名称。文件生成在 `internal/adapters/postgresql/migrations/`。

填写迁移文件中的建表和回滚部分：

```sql
-- +goose Up
CREATE TABLE examples (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE examples;
```

后续新增字段、索引等继续创建新的迁移文件。已应用的迁移通过新迁移修正，避免已有数据库与迁移记录不一致。

### 4.2 执行数据库迁移

```bash
goose up
goose status
```

`goose up` 将未应用的迁移执行到实际数据库；`goose status` 查看迁移状态。Goose 会创建自己的迁移版本表，用于记录哪些迁移已经执行。

需要回滚最近一次迁移时使用 `goose down`，执行前检查该文件的 Down SQL；本例回滚会删除 `examples` 表及其中的数据。

### 4.3 编写查询 SQL

将 `internal/adapters/postgresql/sqlc/queries.sql` 的占位注释替换为：

```sql
-- name: ListExamples :many
SELECT id, name FROM examples ORDER BY id;

-- name: CreateExample :one
INSERT INTO examples (name)
VALUES ($1)
RETURNING id, name;
```

名称会成为生成的 Go 方法名。`:many` 返回多条记录，`:one` 返回一条记录；不返回记录的写操作通常使用 `:exec`，需要受影响行数时可以使用 `:execrows`。

### 4.4 生成 Repository

```bash
sqlc generate
```

当前 `sqlc.yaml` 配置读取：

- 表结构：`internal/adapters/postgresql/migrations/`。
- 查询 SQL：`internal/adapters/postgresql/sqlc/queries.sql`。
- Go 输出：`internal/adapters/postgresql/sqlc/`，包名为 `repo`。

本例会生成 `repo.Example`、`ListExamples`、`CreateExample` 和包含查询方法的 `repo.Querier` 等代码。

Goose 更新实际数据库；sqlc 根据 SQL 文件生成 Go 代码。当前配置下，sqlc 不要求先连接数据库执行迁移，日常开发按“执行迁移 → 编写查询 → 生成代码”的顺序操作即可。

修改表结构或查询 SQL 后重新执行 `sqlc generate`。只修改 Service、Handler 或路由时不需要执行。初始没有业务 schema 和查询时，不执行生成。

### 4.5 替换占位代码

首次接入真实查询时：

1. 移除手写的 `internal/adapters/postgresql/sqlc/template.go`，它引用了原来的占位模型。
2. 将业务层中的 `repo.Template` 和 `Template()` 调整为实际模型和方法，下面给出完整替换示例。
3. 保留生成的 `db.go`、`models.go`、`querier.go`、查询实现文件；这些文件交由 sqlc 管理，不手工编辑。

`sqlc generate` 不负责清理所有手写占位文件，也不会保留原有占位接口。完成业务层替换后再进行整体编译。

### 4.6 定义请求类型

将 `internal/template/types.go` 替换为：

```go
package template

type Request struct {
    Name string `json:"name"`
}
```

可以继续使用 `template` 包，或者按业务重命名目录、包名及对应 import。

### 4.7 编写 Service 和事务

业务逻辑写在 `internal/template/service.go`。保留 `Service` 接口、`service` 实现和 `NewService` 构造函数。

初始占位代码的 `repo` 字段使用 `repo.Querier`。下面的事务开发示例按 develop 分支中的订单实现，改为带 `WithTx` 方法的 `*repo.Queries`；sqlc 生成的 `repo.Querier` 接口只包含查询方法。

将 `service.go` 替换为：

```go
package template

import (
    "context"
    "errors"

    repo "my-app/internal/adapters/postgresql/sqlc"
    "github.com/jackc/pgx/v5"
)

type Service interface {
    ListExamples(ctx context.Context) ([]repo.Example, error)
    CreateExample(ctx context.Context, params Request) (repo.Example, error)
}

// 错误提示定义
var ErrNameRequired = errors.New("name 不能为空")

type service struct {
    repo *repo.Queries
    db   *pgx.Conn
}

func NewService(repo *repo.Queries, db *pgx.Conn) Service {
    return &service{repo: repo, db: db}
}

func (s *service) ListExamples(ctx context.Context) ([]repo.Example, error) {
    return s.repo.ListExamples(ctx)
}

func (s *service) CreateExample(
    ctx context.Context,
    params Request,
) (repo.Example, error) {
    // 校验入参
    if params.Name == "" {
        return repo.Example{}, ErrNameRequired
    }

    // 开启事务
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return repo.Example{}, err
    }
    defer tx.Rollback(ctx)

    // 创建绑定事务的查询对象
    qtx := s.repo.WithTx(tx)

    // 创建记录
    result, err := qtx.CreateExample(ctx, params.Name)
    if err != nil {
        return repo.Example{}, err
    }

    // 执行其他需要一起提交的业务操作
    // 所有事务内的数据库操作都通过 qtx 调用
    // 任意一步失败，立即返回错误

    // 提交事务
    if err := tx.Commit(ctx); err != nil {
        return repo.Example{}, err
    }

    // 返回结果
    return result, nil
}
```

事务执行顺序是：校验入参 → Begin → defer Rollback → WithTx → 业务数据库操作 → Commit → 返回结果。

普通查询通过 `s.repo` 调用。事务中的查询和写入全部通过 `qtx` 调用；中途返回错误时由延迟执行的 Rollback 回滚，提交成功后再执行 Rollback 不会撤销已提交结果。

单次 INSERT 通常无需显式事务，本例用于展示开发结构。像 develop 分支中的创建订单、创建订单明细、扣减库存，需要一起成功或失败时，将这些操作全部放在同一个事务中，并逐步检查错误。事务放在 Service 中管理。

### 4.8 编写 Handler

HTTP 请求处理写在 `internal/template/handlers.go`。将文件替换为：

```go
package template

import (
    "errors"
    "log"
    "net/http"

    "my-app/internal/json"
)

type Handler struct {
    service Service
}

func NewHandler(service Service) *Handler {
    return &Handler{service: service}
}

func (h *Handler) ListExamples(w http.ResponseWriter, r *http.Request) {
    result, err := h.service.ListExamples(r.Context())
    if err != nil {
        log.Println(err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    json.Write(w, http.StatusOK, result)
}

func (h *Handler) CreateExample(w http.ResponseWriter, r *http.Request) {
    // 解析请求参数
    var params Request
    if err := json.Read(r, &params); err != nil {
        log.Println(err)
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // 调用业务服务
    result, err := h.service.CreateExample(r.Context(), params)
    if err != nil {
        if errors.Is(err, ErrNameRequired) {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        log.Println(err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    // 返回成功响应
    json.Write(w, http.StatusCreated, result)
}
```

`json.Read` 解析请求，`r.Context()` 向 Service 传递上下文，`json.Write` 统一设置 JSON 响应的 Content-Type。Handler 不重复设置成功响应的 Content-Type，也不管理数据库事务。

错误响应使用 `http.Error`，它返回文本响应。请求格式或本例参数校验错误返回 400，内部错误返回 500；其他业务错误按接口约定映射到适当的 HTTP 状态码。

### 4.9 注入依赖并注册路由

在 `cmd/api.go` 的 import 中增加：

```go
repo "my-app/internal/adapters/postgresql/sqlc"
"my-app/internal/template"
```

在 `mount()` 中、现有中间件和 `/health` 路由之后、`return r` 之前添加：

```go
// 创建数据库查询对象
queries := repo.New(app.db)

// 初始化业务服务和处理器
exampleService := template.NewService(queries, app.db)
exampleHandler := template.NewHandler(exampleService)

// 注册业务路由
r.Get("/examples", exampleHandler.ListExamples)
r.Post("/examples", exampleHandler.CreateExample)
```

请求调用顺序为：路由 → Handler → Service → Repository → PostgreSQL。后续新增业务模块时，在各自的业务目录中定义类型、Service 和 Handler，并在这里注入依赖和注册路由。

### 4.10 格式化、编译和验证

```bash
go fmt ./...
go vet ./...
go build -o /tmp/my-app ./cmd
go run ./cmd
```

已在本机运行的服务需要重启才能加载修改。使用 API 容器时执行 `docker compose up -d --build api` 重新构建并启动。

在另一个终端检查接口：

```bash
# 健康检查
curl -i http://localhost:8080/health

# 创建记录，应返回 201
curl -i -X POST http://localhost:8080/examples \
  -H 'Content-Type: application/json' \
  -d '{"name":"示例"}'

# 查询记录，应返回 200
curl -i http://localhost:8080/examples

# 参数校验，应返回 400
curl -i -X POST http://localhost:8080/examples \
  -H 'Content-Type: application/json' \
  -d '{"name":""}'
```

涉及多项数据库操作的事务，还需要在开发环境验证中间某一步失败时，前面的写入是否一起回滚；所有操作成功时，检查结果是否完整提交。

## 5. 日常修改与命令

| 修改内容 | 后续操作 |
|---|---|
| 新增表、字段、索引 | 新建迁移 → `goose up` → 按需更新查询 → `sqlc generate` → 调整 Go 代码 |
| 新增或修改查询 SQL | `sqlc generate` → 调整 Service 调用 |
| 修改 Service、Handler 或路由 | 格式化 → 编译 → 重启并验证接口 |
| 首次接入真实查询 | 移除手写占位文件，替换原有占位模型和方法引用 |
| 修改数据库连接 | 更新 `.env` 中应用和 Goose 的对应配置，按需同步 Compose 配置 → 重启应用 |

数据库数据保存在 Compose 数据卷中。删除迁移文件不会清空已有数据库；新项目默认使用自己的 Compose 数据卷，若复用旧数据卷或连接已有数据库，原有表和数据仍会保留。

## 6. 工具参考

- [Goose 使用说明](https://github.com/pressly/goose#usage)
- [sqlc 查询注释](https://docs.sqlc.dev/en/latest/reference/query-annotations.html)
- [sqlc 配置](https://docs.sqlc.dev/en/latest/reference/config.html)
- [sqlc 事务与 WithTx](https://docs.sqlc.dev/en/latest/howto/transactions.html)
