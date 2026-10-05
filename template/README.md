# Go 应用

项目使用 chi、pgx 和 godotenv，保留数据库连接和分层结构，默认只注册 `/health`，不预置业务表。

## 启动

由 `go-init` 创建时依赖、开发工具和数据库已经准备好，执行：

```bash
go run ./cmd
```

访问 <http://localhost:8080/health>，响应为 `非常好👍`。

如果依赖或数据库准备失败，手动完成：

```bash
go mod download
docker compose up -d postgres
go run ./cmd
```

`.env` 提供应用的 DB_DSN 以及 Goose 配置。默认使用 `postgres` 数据库，用户名和密码均为 `postgres`；本机应用连接 localhost:5432，API 容器连接 Compose 的 postgres 服务。

完整容器运行：

```bash
docker compose up -d --build
```

## 结构

- `cmd/`：应用入口、HTTP 路由与服务配置。
- `internal/template/`：Handler、Service、请求类型的可编译示例。
- `internal/adapters/postgresql/migrations/`：初始只有 `.gitkeep`。
- `internal/adapters/postgresql/sqlc/`：数据库基础设施、占位接口与模型、查询入口。
- `internal/json/`：统一 JSON 响应和请求解码。
- `internal/env/`：环境变量读取。

模板方法默认不注册到路由。Handler 保留 NewHandler、Service 注入、上下文和错误处理，成功响应调用 json.Write，错误使用 http.Error。Service 保留 NewService、repo.Querier 和数据库注入。

## 添加表与查询

Goose 自动读取项目根目录的 `.env`：

```bash
goose -s create create_example sql
```

填写生成迁移的 `-- +goose Up` 建表 SQL 和 `-- +goose Down` 回滚 SQL，再执行：

```bash
goose up
goose status
```

在 queries.sql 中添加带 sqlc 名称注释的查询，然后执行：

```bash
sqlc generate
```

初始没有 schema 和查询，不执行 sqlc generate。Goose 执行实际数据库变更；sqlc 只生成查询代码。

## 替换占位代码

当前 repo.Template 是空类型，Queries.Template 返回空列表，不执行数据库操作。db.go 保留 DBTX、Queries、New 和 WithTx。

真实 SQL 生成后，querier.go/models.go 等将被生成器接管。移除或调整手写 template.go，避免与生成类型或方法重名；同时把 internal/template 的接口和调用改成真实生成的类型与查询，或移除示例模块。生成后执行 `go vet ./...` 和 `go build ./cmd`，不要假设生成器会保留占位接口。

数据库启动时保留连接检查。删除迁移文件不会清空已有数据库，使用新项目的新数据卷获得初始无业务表状态，不自动清理旧数据。
