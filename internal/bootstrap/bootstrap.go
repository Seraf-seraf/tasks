package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"log/slog"
	"net/http"
	"taskservice/internal/app"
	"taskservice/internal/config"
	"taskservice/internal/email"
	"taskservice/internal/httpapi"
	"taskservice/internal/repository"
	"taskservice/internal/service"
	"time"
)

func Build(ctx context.Context, c config.Config, log *slog.Logger) (func(context.Context) error, error) {
	const methodCtx = "bootstrap.Build"
	db, err := sql.Open("mysql", c.MySQL.DSN)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	db.SetMaxOpenConns(c.MySQL.MaxOpenConns)
	db.SetMaxIdleConns(c.MySQL.MaxIdleConns)
	db.SetConnMaxLifetime(c.MySQL.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	rc := redis.NewClient(&redis.Options{Addr: c.Redis.Addr, Password: c.Redis.Password, DB: c.Redis.DB})
	svc := service.New(repository.New(db), rc, email.New(c.Email.Endpoint))
	h := httpapi.New(svc, c.JWT.Secret)
	srv := &http.Server{Addr: c.HTTP.Addr, Handler: h.Router(), ReadHeaderTimeout: 5 * time.Second}
	shutdown, err := app.Run(ctx, log, srv)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { _ = shutdown(ctx); _ = rc.Close(); return db.Close() }, nil
}
