// Package main is the entrypoint for donnarec-api.
//
// Wiring order: pool → queries → services → handlers → router → server.
// All dependencies are constructor-injected; no global state (except logger and i18n bundle).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/config"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/users"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func main() {
	// Production-quality structured logger (JSON output)
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// Load and validate configuration from environment
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config load failed", zap.Error(err))
	}

	// Graceful shutdown context — listens for SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --------------------------------------------------------
	// Database: pgxpool connection
	// --------------------------------------------------------
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("pgxpool connect failed", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("postgres ping failed", zap.Error(err))
	}
	logger.Info("postgres connected")

	// --------------------------------------------------------
	// Data layer: sqlc-generated queries
	// --------------------------------------------------------
	queries := db.New(pool)

	// --------------------------------------------------------
	// Auth middleware: OIDC token validation via Keycloak
	// --------------------------------------------------------
	authMW, err := auth.NewAuthMiddleware(
		cfg.KeycloakBaseURL,
		cfg.KeycloakRealm,
		cfg.KeycloakClientID,
		logger,
	)
	if err != nil {
		logger.Fatal("auth middleware init failed", zap.Error(err))
	}

	// --------------------------------------------------------
	// Services
	// --------------------------------------------------------
	userSvc := users.NewUserService(pool, queries, logger)

	// --------------------------------------------------------
	// Handlers
	// --------------------------------------------------------
	userHandler := users.NewUserHandler(userSvc, logger)

	// --------------------------------------------------------
	// Router: middleware chain order matters — see Pattern D
	// --------------------------------------------------------
	router := setupRouter(authMW, userHandler, logger)

	// --------------------------------------------------------
	// HTTP server with graceful shutdown
	// --------------------------------------------------------
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background goroutine
	go func() {
		logger.Info("donnarec-api starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Block until OS signal
	<-ctx.Done()

	logger.Info("shutdown signal received; draining connections...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	} else {
		logger.Info("server shutdown complete")
	}
}

// setupRouter wires the Gin router with middleware and route groups.
//
// Middleware chain order (Pattern D):
//  1. Recovery  — catch panics before anything else
//  2. Request logger — structured zap logging for all requests
//  3. (Audit middleware placeholder — wired in plan 01-02)
//  4. Public routes — /healthz (no auth required)
//  5. Protected /api group — RequireAuth()
//  6. Admin /api/admin group — RequireAuth() + RequireRoles(RoleAdmin)
func setupRouter(authMW *auth.AuthMiddleware, userHandler *users.UserHandler, logger *zap.Logger) *gin.Engine {
	router := gin.New()

	// 1. Recover from panics — must be first
	router.Use(gin.Recovery())

	// 2. Structured request logging
	router.Use(zapRequestLogger(logger))

	// 3. TODO(01-02): audit middleware placeholder
	// router.Use(auditMiddleware(auditSvc))

	// ---- Public routes ----
	// /healthz: liveness probe (no auth — used by docker-compose healthcheck + load balancers)
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ---- Protected /api group ----
	api := router.Group("/api")
	api.Use(authMW.RequireAuth())

	// GET /api/me — returns JWT subject + email (auth smoke test)
	api.GET("/me", userHandler.Me)

	// ---- Admin /api/admin group (requires admin role — D-01) ----
	adminGroup := api.Group("/admin")
	adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))

	// POST /api/admin/users — create user (Admin-only, D-01)
	adminGroup.POST("/users", userHandler.CreateUser)

	return router
}

// zapRequestLogger returns a Gin middleware that logs each request with structured fields.
// It does NOT log request/response bodies to avoid logging PII (Pattern C).
func zapRequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		logger.Info("request",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			// ห้าม log request body / headers ที่อาจมี PII หรือ token (Pattern C)
		)
	}
}
