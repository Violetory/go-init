# go-init

English | [简体中文](README.zh-CN.md)

Generate a Go HTTP project in an empty directory, retaining the PostgreSQL connection, chi middleware, shared utilities, and the Handler → Service → Repository structure. Only `/health` is exposed by default.

## Installation

Install from local source:

```bash
go install .
```

Install the current version, `v0.1.2` (run the same command to upgrade):

```bash
go install github.com/Violetory/go-init@v0.1.2
go-init --version
```

Go's tool installation directory must be in PATH: use `GOBIN` if it is set; otherwise, use `$(go env GOPATH)/bin`.

If downloads through your current proxy time out, you can temporarily use the official Go proxy:

```bash
GOPROXY=https://proxy.golang.org,direct go install github.com/Violetory/go-init@v0.1.2
```

## Creating a project

You need Go 1.27.1 (or the ability to download that toolchain automatically), Docker Compose, and a running Docker daemon. Initial downloads require network access; the database uses local port 5432.

```bash
mkdir my-app
cd my-app
go-init
go run ./cmd
```

The default module is `module my-app`. You can also specify a full module path in an empty directory:

```bash
go-init --module github.com/yourname/my-app
```

Both options update internal imports to match. The CLI only accepts an empty directory or one containing only `.git`; it does not overwrite an existing project.

Initialization downloads project dependencies, checks or installs pinned versions of Goose and sqlc, starts PostgreSQL, and waits for a successful connection. It does not create tables, run sqlc, or start the API. After starting the application with `go run ./cmd`, visit <http://localhost:8080/health>.

After initialization, the project contains only `.env`, not `.env.template`. The default database name, username, and password are all `postgres`. Development tools are pinned to Goose v3.28.0 and sqlc v1.31.1; see `tool-versions.json`.

If initialization fails, generated files are retained. Follow the error message to complete the remaining steps; do not run init again in a directory that already contains files. Tools in PATH must match the specified versions, and older versions must not shadow the installation directory.

## Application development

The generated project's README includes the complete development workflow, from database migrations to API verification. You can also read the [application development guide](template/README.md):

1. Create a migration file, fill in the Up/Down SQL, and run `goose up`.
2. Write `queries.sql`, run `sqlc generate`, and replace the Repository placeholder code.
3. Define request types and implement business logic and transaction code using Begin, WithTx, Rollback, and Commit in the Service.
4. Implement the Handler, inject dependencies, and register routes in `cmd/api.go`.
5. Format, build, start, and verify the API.

`v0.1.2` embeds this development guide in English and Chinese, with English as the default and a link to switch languages. Projects generated after upgrading the CLI use the updated template; files in existing projects are not updated automatically.

## Template maintenance

The root directory is the CLI module, while `template/` is a separate application template module. Each maintains its own go.mod/go.sum. The CLI's pgx and godotenv dependencies are used to check the target database connection and parse environment configuration resources.

After editing `template/`, package it (the maintenance script requires Python 3):

```bash
sh scripts/update-template.sh
# Or
go generate .
```

The script packages only the template directory, including hidden files and `.gitkeep`, while excluding local `.env` files, IDE files, Git files, and build artifacts. `template/.env.template` is a configuration generation resource and is not written to the target directory during extraction. With unchanged file contents, the template ZIP can be regenerated with identical bytes.

Validate the two modules separately:

```bash
go vet ./...
go build -o /tmp/go-init .
(cd template && go vet ./... && go build -o /tmp/go-init-app ./cmd)
```

Before publishing, run the existing packaging script, commit the source, tool versions, and latest `template.zip`, then push the branch and publish a version tag:

The following example uses the new version `v0.1.3`; adjust it to the actual version when publishing. Do not overwrite or reuse published version tags.

```bash
sh scripts/update-template.sh
git add -A
git commit -m 'docs: 更新模板和开发文档'
git push origin master
git tag -a v0.1.3 -m '发布 v0.1.3'
git push origin v0.1.3
```

Go CLI source versions are published through tags. Users can install directly with `go install`, without prebuilt binaries. `go-init --version` automatically reads the module version recorded during installation. Local builds display `dev`; you can also explicitly set the version with `go build -ldflags '-X main.version=v0.1.2'`.

See the [modification plan](docs/脚手架工具修改方案.md) for the complete design.

## Version sources

- [Goose v3.28.0](https://github.com/pressly/goose/releases/tag/v3.28.0)
- [sqlc v1.31.1](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1)
