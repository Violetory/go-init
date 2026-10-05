package main

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

//go:generate sh scripts/update-template.sh

//go:embed template.zip
var templateZip []byte

//go:embed tool-versions.json
var toolVersionsJSON []byte

var version = "dev"

func cliVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

type projectFile struct {
	name string
	data []byte
	mode fs.FileMode
}

type toolSpec struct {
	name, version, packagePath, versionArg string
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dir, err := os.Getwd()
	if err == nil {
		err = run(ctx, os.Args[1:], dir, os.Stdout, os.Stderr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-init:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dir string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("go-init", flag.ContinueOnError)
	flags.SetOutput(errOut)
	moduleName := flags.String("module", "", "新项目 module（默认使用当前目录名）")
	showVersion := flags.Bool("version", false, "显示 CLI 版本")
	flags.Usage = func() {
		fmt.Fprintln(errOut, "用法：在空目录执行 go-init [--module github.com/yourname/my-app]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("不接受位置参数，请在目标空目录执行 go-init")
	}
	if *showVersion {
		fmt.Fprintln(out, cliVersion())
		return nil
	}
	if err := checkEmptyDirectory(dir); err != nil {
		return err
	}
	if *moduleName == "" {
		*moduleName = filepath.Base(dir)
	}
	files, env, err := prepareProject(templateZip, *moduleName)
	if err != nil {
		return err
	}
	tools, err := readToolSpecs(toolVersionsJSON)
	if err != nil {
		return err
	}
	runner := commandRunner{dir: dir, out: out, errOut: errOut}
	fmt.Fprintln(out, "检查 Go、Docker 和 Compose…")
	if err := runner.preflight(ctx); err != nil {
		return err
	}
	fmt.Fprintln(out, "生成项目文件…")
	if err := writeProject(dir, files); err != nil {
		return err
	}
	fmt.Fprintln(out, "下载项目依赖…")
	if err := runner.execute(ctx, "go", "mod", "download"); err != nil {
		return fmt.Errorf("下载依赖失败：%w；文件已保留，可在项目目录重试 go mod download", err)
	}
	for _, tool := range tools {
		if err := runner.ensureTool(ctx, tool); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "启动 PostgreSQL…")
	if err := runner.execute(ctx, "docker", "compose", "up", "-d", "postgres"); err != nil {
		return fmt.Errorf("启动数据库失败：%w；重试 docker compose up -d postgres，确认 5432 端口可用", err)
	}
	if err := runner.waitForDatabase(ctx, env["DB_DSN"]); err != nil {
		return fmt.Errorf("等待数据库失败：%w；检查 docker compose logs postgres 和 .env 的 DB_DSN", err)
	}
	fmt.Fprintf(out, "\n初始化完成\n目录：%s\nmodule：%s\n数据库：就绪\n", dir, *moduleName)
	for _, tool := range tools {
		fmt.Fprintf(out, "%s：%s\n", tool.name, tool.version)
	}
	fmt.Fprintln(out, "\n启动应用：go run ./cmd\n健康检查：http://localhost:8080/health")
	return nil
}

func checkEmptyDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		return fmt.Errorf("目标目录不是空目录（发现 %s），不会覆盖已有文件", entry.Name())
	}
	return nil
}

func prepareProject(archive []byte, moduleName string) ([]projectFile, map[string]string, error) {
	if err := module.CheckImportPath(moduleName); err != nil {
		return nil, nil, fmt.Errorf("module 名称无效：%w；请使用 --module 指定有效路径", err)
	}
	if strings.Contains(strings.Split(moduleName, "/")[0], ".") {
		if err := module.CheckPath(moduleName); err != nil {
			return nil, nil, fmt.Errorf("module 路径无效：%w", err)
		}
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, nil, fmt.Errorf("读取模板失败：%w", err)
	}
	entries := make(map[string]projectFile)
	var total uint64
	for _, entry := range zr.File {
		name := entry.Name
		if !fs.ValidPath(strings.TrimSuffix(name, "/")) || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, ".git/") || name == ".git" {
			return nil, nil, fmt.Errorf("模板包含不安全路径：%q", name)
		}
		if entry.Mode()&fs.ModeType != 0 && !entry.FileInfo().IsDir() {
			return nil, nil, fmt.Errorf("模板包含不支持的文件类型：%s", name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if name == ".env" {
			return nil, nil, errors.New("模板不能包含本机 .env")
		}
		if _, exists := entries[name]; exists {
			return nil, nil, fmt.Errorf("模板包含重复文件：%s", name)
		}
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > 10<<20 || total > 64<<20 {
			return nil, nil, errors.New("模板解压大小超出限制")
		}
		r, err := entry.Open()
		if err != nil {
			return nil, nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(r, 10<<20+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || len(data) > 10<<20 {
			return nil, nil, fmt.Errorf("读取模板文件 %s 失败：%w", name, errors.Join(readErr, closeErr, sizeError(len(data))))
		}
		mode := fs.FileMode(0644)
		if entry.Mode()&0111 != 0 {
			mode = 0755
		}
		entries[name] = projectFile{name: name, data: data, mode: mode}
	}
	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := entries[parent]; exists {
				return nil, nil, fmt.Errorf("模板目录与文件冲突：%s", parent)
			}
		}
	}
	for _, required := range []string{"go.mod", "go.sum", ".env.template", "cmd/main.go", "cmd/api.go", "docker-compose.yaml", "sqlc.yaml", "internal/adapters/postgresql/migrations/.gitkeep", "internal/adapters/postgresql/sqlc/queries.sql"} {
		if _, ok := entries[required]; !ok {
			return nil, nil, fmt.Errorf("模板缺少文件：%s", required)
		}
	}
	mod := entries["go.mod"]
	mf, err := modfile.Parse("go.mod", mod.data, nil)
	if err != nil || mf.Module == nil {
		return nil, nil, fmt.Errorf("模板 go.mod 无效：%v", err)
	}
	oldModule := mf.Module.Mod.Path
	if err := mf.AddModuleStmt(moduleName); err != nil {
		return nil, nil, err
	}
	mod.data, err = mf.Format()
	if err != nil {
		return nil, nil, err
	}
	entries["go.mod"] = mod
	for name, file := range entries {
		if strings.HasSuffix(name, ".go") {
			file.data, err = rewriteImports(name, file.data, oldModule, moduleName)
			if err != nil {
				return nil, nil, err
			}
			entries[name] = file
		}
	}
	envData, env, err := renderEnv(entries[".env.template"].data)
	if err != nil {
		return nil, nil, err
	}
	delete(entries, ".env.template")
	entries[".env"] = projectFile{name: ".env", data: envData, mode: 0600}
	files := make([]projectFile, 0, len(entries))
	for _, file := range entries {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, env, nil
}

func sizeError(size int) error {
	if size > 10<<20 {
		return errors.New("文件过大")
	}
	return nil
}

func rewriteImports(name string, data []byte, oldModule, newModule string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, data, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", name, err)
	}
	for i := len(file.Imports) - 1; i >= 0; i-- {
		item := file.Imports[i].Path
		importPath, err := strconv.Unquote(item.Value)
		if err != nil {
			return nil, err
		}
		if importPath != oldModule && !strings.HasPrefix(importPath, oldModule+"/") {
			continue
		}
		start, end := fset.Position(item.Pos()).Offset, fset.Position(item.End()).Offset
		replacement := strconv.Quote(newModule + strings.TrimPrefix(importPath, oldModule))
		updated := make([]byte, 0, len(data)+len(replacement))
		updated = append(updated, data[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, data[end:]...)
		data = updated
	}
	return data, nil
}

func renderEnv(data []byte) ([]byte, map[string]string, error) {
	values, err := godotenv.Unmarshal(string(data))
	if err != nil {
		return nil, nil, fmt.Errorf("解析环境模板失败：%w", err)
	}
	for _, key := range []string{"DB_DSN", "DB_DRIVER", "DB_MIGRATION_DIR", "GOOSE_DRIVER", "GOOSE_DBSTRING", "GOOSE_MIGRATION_DIR"} {
		if values[key] == "" {
			return nil, nil, fmt.Errorf("环境模板缺少 %s", key)
		}
	}
	var output strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		key, _, assignment := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if assignment && !strings.HasPrefix(key, "#") {
			line = key + "=" + strconv.Quote(values[key])
		}
		output.WriteString(line + "\n")
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n"), values, nil
}

func writeProject(dir string, files []projectFile) error {
	if err := checkEmptyDirectory(dir); err != nil {
		return err
	}
	for _, file := range files {
		destination := filepath.Join(dir, filepath.FromSlash(file.name))
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.mode)
		if err != nil {
			return fmt.Errorf("写入 %s 失败：%w", file.name, err)
		}
		_, writeErr := f.Write(file.data)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	return nil
}

func readToolSpecs(data []byte) ([]toolSpec, error) {
	var versions struct{ Goose, Sqlc string }
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, err
	}
	if versions.Goose == "" || versions.Sqlc == "" {
		return nil, errors.New("工具版本配置不完整")
	}
	return []toolSpec{
		{"goose", versions.Goose, "github.com/pressly/goose/v3/cmd/goose", "-version"},
		{"sqlc", versions.Sqlc, "github.com/sqlc-dev/sqlc/cmd/sqlc", "version"},
	}, nil
}

type commandRunner struct {
	dir         string
	out, errOut io.Writer
}

func (r commandRunner) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	return cmd
}

func (r commandRunner) execute(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	cmd := r.command(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = r.out, r.errOut
	return cmd.Run()
}

func (r commandRunner) output(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := r.command(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s：%w\n%s", name, strings.Join(args, " "), err, data)
	}
	return strings.TrimSpace(string(data)), nil
}

func (r commandRunner) preflight(ctx context.Context) error {
	for _, name := range []string{"go", "docker"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("缺少 %s，请先安装并加入 PATH", name)
		}
	}
	if _, err := r.output(ctx, "go", "version"); err != nil {
		return err
	}
	if _, err := r.output(ctx, "docker", "compose", "version"); err != nil {
		return fmt.Errorf("Docker Compose 不可用：%w", err)
	}
	if _, err := r.output(ctx, "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("Docker 未就绪，请先启动 Docker：%w", err)
	}
	return nil
}

func matchesVersion(output, expected string) bool {
	for _, field := range strings.Fields(output) {
		if field == expected || "v"+field == expected {
			return true
		}
	}
	return false
}

func (r commandRunner) ensureTool(ctx context.Context, tool toolSpec) error {
	if output, err := r.output(ctx, tool.name, tool.versionArg); err == nil && matchesVersion(output, tool.version) {
		fmt.Fprintf(r.out, "%s %s 已可用\n", tool.name, tool.version)
		return nil
	}
	fmt.Fprintf(r.out, "安装 %s %s…\n", tool.name, tool.version)
	if err := r.execute(ctx, "go", "install", tool.packagePath+"@"+tool.version); err != nil {
		return fmt.Errorf("安装 %s 失败：%w；可重试 go install %s@%s", tool.name, err, tool.packagePath, tool.version)
	}
	if output, err := r.output(ctx, tool.name, tool.versionArg); err == nil && matchesVersion(output, tool.version) {
		return nil
	}
	installDir, err := r.output(ctx, "go", "env", "GOBIN")
	if err != nil {
		return err
	}
	if installDir == "" {
		gopath, err := r.output(ctx, "go", "env", "GOPATH")
		if err != nil {
			return err
		}
		installDir = filepath.Join(filepath.SplitList(gopath)[0], "bin")
	}
	filename := tool.name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	installed := filepath.Join(installDir, filename)
	output, err := r.output(ctx, installed, tool.versionArg)
	if err != nil || !matchesVersion(output, tool.version) {
		return fmt.Errorf("无法确认安装后的 %s 版本：%v", tool.name, err)
	}
	return fmt.Errorf("%s 已安装在 %s，但 PATH 未优先使用此版本。将 %s 放到 PATH 最前面；然后安装其余工具并执行 docker compose up -d postgres 和 go run ./cmd（文件已保留）", tool.name, installed, installDir)
}

func (r commandRunner) waitForDatabase(ctx context.Context, dsn string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	id, err := r.output(ctx, "docker", "compose", "ps", "-q", "postgres")
	if err != nil || id == "" {
		return fmt.Errorf("未找到数据库容器：%v", err)
	}
	fmt.Fprintln(r.out, "等待数据库健康检查和连接确认…")
	var lastErr error
	for {
		status, err := r.output(ctx, "docker", "inspect", "--format", "{{.State.Health.Status}}", id)
		lastErr = err
		if err == nil && status == "healthy" {
			attempt, stop := context.WithTimeout(ctx, 5*time.Second)
			conn, connectErr := pgx.Connect(attempt, dsn)
			if connectErr == nil {
				connectErr = conn.Ping(attempt)
				_ = conn.Close(attempt)
			}
			stop()
			if connectErr == nil {
				return nil
			}
			lastErr = connectErr
		} else if err == nil {
			lastErr = fmt.Errorf("容器健康状态：%s", status)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w；最后状态：%v", ctx.Err(), lastErr)
		case <-time.After(time.Second):
		}
	}
}
