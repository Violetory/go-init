# go-init

在空目录生成 Go HTTP 项目，保留 PostgreSQL 连接、chi 中间件、通用工具和 Handler → Service → Repository 结构，默认只开放 `/health`。

## 安装

本地源码安装：

```bash
go install .
```

安装当前版本 `v0.1.1`（升级时执行同一命令）：

```bash
go install github.com/Violetory/go-init@v0.1.1
go-init --version
```

Go 的工具安装目录需要在 PATH 中：设置了 `GOBIN` 时使用它，否则使用 `$(go env GOPATH)/bin`。

如果当前代理下载超时，可以临时使用官方 Go 代理：

```bash
GOPROXY=https://proxy.golang.org,direct go install github.com/Violetory/go-init@v0.1.1
```

## 创建项目

需要 Go 1.27.1（或可自动取得该工具链）、Docker Compose，以及已启动的 Docker。首次下载需要网络；数据库使用本机 5432 端口。

```bash
mkdir my-app
cd my-app
go-init
go run ./cmd
```

默认生成 `module my-app`。也可以在空目录指定完整模块路径：

```bash
go-init --module github.com/yourname/my-app
```

两种方式均会同步修改内部 import。CLI 只接受空目录或只有 `.git` 的目录，不覆盖已有项目。

初始化会下载项目依赖，检查或安装固定版本 Goose 和 sqlc，启动 PostgreSQL 并等待连接成功；不执行建表、不运行 sqlc、不启动 API。执行 `go run ./cmd` 启动应用后，访问 <http://localhost:8080/health>。

初始化后只有 `.env`，没有 `.env.template`。默认数据库名、用户和密码均为 `postgres`。开发工具固定为 Goose v3.28.0、sqlc v1.31.1，见 `tool-versions.json`。

如果初始化失败，已生成文件保留，按错误提示完成剩余步骤；不要在已有文件的目录再次执行 init。PATH 中的工具必须是指定版本，安装位置不能被旧版本遮蔽。

## 应用开发

生成项目的 README 包含从数据库迁移到接口验证的完整开发流程，也可以查看 [应用开发指南](template/README.md)：

1. 创建迁移文件，填写 Up/Down SQL，执行 `goose up`。
2. 编写 `queries.sql`，执行 `sqlc generate`，替换 Repository 占位代码。
3. 定义请求类型，在 Service 中编写业务逻辑及 Begin、WithTx、Rollback、Commit 事务代码。
4. 编写 Handler，在 `cmd/api.go` 中注入依赖并注册路由。
5. 格式化、编译、启动并验证接口。

`v0.1.1` 已内置该开发指南。升级 CLI 后新生成的项目使用新版模板；已生成项目的文件不会自动更新。

## 模板维护

根目录是 CLI 模块，`template/` 是独立的应用模板模块，各自维护 go.mod/go.sum。CLI 本身的 pgx、godotenv 用于检查目标数据库连接及解析环境资源。

编辑 `template/` 后打包（维护脚本需要 Python 3）：

```bash
sh scripts/update-template.sh
# 或
go generate .
```

脚本只打包模板目录，包含隐藏文件和 `.gitkeep`，排除本机 `.env`、IDE、Git 及构建产物。`template/.env.template` 是配置生成资源，解压时不写入目标目录。模板 ZIP 在固定文件内容下可重复生成相同字节。

分别验证两个模块：

```bash
go vet ./...
go build -o /tmp/go-init .
(cd template && go vet ./... && go build -o /tmp/go-init-app ./cmd)
```

发布前运行现有打包脚本，提交源码、工具版本和最新 `template.zip`，推送分支并发布版本标签：

下面以新版本 `v0.1.2` 为例，发布时按实际版本调整。已发布的版本标签不覆盖、不重复使用。

```bash
sh scripts/update-template.sh
git add -A
git commit -m 'docs: 更新模板和开发文档'
git push origin master
git tag -a v0.1.2 -m '发布 v0.1.2'
git push origin v0.1.2
```

Go CLI 的源码版本通过标签发布，用户可直接 `go install`，无需预编译二进制。`go-init --version` 自动读取安装时的模块版本；本地构建显示 `dev`，也可通过 `go build -ldflags '-X main.version=v0.1.1'` 显式标记版本。

完整设计见 [修改方案](docs/脚手架工具修改方案.md)。

## 版本来源

- [Goose v3.28.0](https://github.com/pressly/goose/releases/tag/v3.28.0)
- [sqlc v1.31.1](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)
