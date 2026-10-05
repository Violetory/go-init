package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"example.com/go-init-template/internal/env"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	// Logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Reload local configuration on each startup, overriding inherited values.
	if err := godotenv.Overload(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Error("加载 .env 失败", "error", err)
		os.Exit(1)
	}

	config := config{
		addr: ":8080",
		db: dbConfig{
			dsn: env.GetString("DB_DSN"),
		},
	}

	if config.db.dsn == "" {
		logger.Error("缺少数据库连接配置，请在 .env 或环境变量中设置 DB_DSN")
		os.Exit(1)
	}

	// Database
	conn, err := pgx.Connect(ctx, config.db.dsn)
	if err != nil {
		logger.Error("数据库连接失败", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	logger.Info("数据库连接成功")

	api := application{
		config: config,
		db:     conn,
	}

	if err := api.run(api.mount()); err != nil {
		slog.Error("服务启动失败", "error", err)
		os.Exit(1)
	}
}
