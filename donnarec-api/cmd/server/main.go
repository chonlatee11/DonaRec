// Package main is the entrypoint for donnarec-api.
//
// Wiring order: pool → queries → services → handlers → router → server.
// All dependencies are constructor-injected; no global state (except logger and i18n bundle).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/captcha"
	"github.com/donnarec/donnarec-api/internal/config"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/edonation"
	"github.com/donnarec/donnarec-api/internal/i18n"
	"github.com/donnarec/donnarec-api/internal/mailer"
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/donnarec/donnarec-api/internal/ratelimit"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/report"
	"github.com/donnarec/donnarec-api/internal/settings"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/donnarec/donnarec-api/internal/users"
	"github.com/donnarec/donnarec-api/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
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

	// Warn loudly if DATABASE_URL disables TLS against a non-localhost host
	// (IN-04): unencrypted traffic to a remote Postgres violates PDPA/NFR-02.
	if insecure, host := cfg.InsecureDatabaseTLS(); insecure {
		logger.Warn("DATABASE_URL uses sslmode=disable against a non-localhost host — "+
			"connection to Postgres is UNENCRYPTED; use sslmode=verify-full outside local dev (NFR-02)",
			zap.String("db_host", host),
		)
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
		cfg.KeycloakIssuer,
		logger,
	)
	if err != nil {
		logger.Fatal("auth middleware init failed", zap.Error(err))
	}

	// --------------------------------------------------------
	// Services
	// --------------------------------------------------------
	// Audit service: append-only hash-chained audit trail (D-17, NFR-05)
	auditSvc := audit.NewAuditService(pool, queries, logger)

	userSvc := users.NewUserService(pool, queries, logger)

	// Envelope key provider: reads DONAREC_KEK from env (D-26, NFR-02).
	keyProvider, err := crypto.NewEnvKeyProvider()
	if err != nil {
		logger.Fatal("crypto key provider init failed", zap.Error(err))
	}

	// Receipt number allocator: gap-less counter via SELECT … FOR UPDATE (D-33, NFR-04).
	allocator := receiptno.NewAllocator(queries)

	// Donation service: PII encrypt/mask, state machine, consent capture (FR-07/09/11/29).
	donationSvc := donation.NewDonationService(pool, queries, allocator, auditSvc, keyProvider, logger)

	// Object storage client: MinIO/S3-compatible, used for slip file uploads (Plan 03-04, D-48).
	storageClient, err := storage.NewStorageClient(
		cfg.MinIO.Endpoint,
		cfg.MinIO.AccessKey,
		cfg.MinIO.SecretKey,
		cfg.MinIO.Bucket,
		cfg.MinIO.Secure,
	)
	if err != nil {
		logger.Fatal("storage client init failed", zap.Error(err))
	}

	// Slip service: upload/view/remove donation slip attachments (Plan 03-04, D-48, D-54).
	slipSvc := donation.NewSlipService(pool, queries, storageClient, auditSvc, logger)

	// --------------------------------------------------------
	// Outbox worker (Phase 4, plan 04-05): polls outbox_jobs enqueued by the
	// Approve issuance tx (Step 7) and renders/freezes/emails the receipt PDF
	// entirely off the issuance lock path (NFR-07).
	// --------------------------------------------------------

	// Receipts object storage client: separate bucket from slips (D-56).
	receiptsStore, err := storage.NewStorageClient(
		cfg.MinIO.Endpoint,
		cfg.MinIO.AccessKey,
		cfg.MinIO.SecretKey,
		cfg.MinIO.ReceiptsBucket,
		cfg.MinIO.Secure,
	)
	if err != nil {
		logger.Fatal("receipts storage client init failed", zap.Error(err))
	}

	// Wire the SAME receipts store into DonationService for DownloadReceipt's
	// presigned URL (FR-28, plan 04-06) — resend/download reuse the frozen PDF the
	// worker (above) writes; neither path re-renders or allocates a new number.
	donationSvc.SetReceiptsStore(receiptsStore)

	// Sandboxed remote-Chromium renderer (D-58) — dials the chrome sidecar over CDP.
	// Passing logger lets the WR-02 fix (04-REVIEW.md) actually surface FailRequest
	// errors instead of them vanishing into a discarded goroutine error.
	pdfRenderer, err := pdf.NewRenderer(cfg.Worker.ChromeWSURL, logger)
	if err != nil {
		logger.Fatal("pdf renderer init failed", zap.Error(err))
	}

	// i18n bundle for bilingual receipt/email text (FR-23/26). LOCALES_DIR
	// defaults to "/locales", matching the Dockerfile's COPY of
	// internal/i18n/locales into the runtime image; override for local
	// `go run` dev where the working directory is the module root.
	localesDir := os.Getenv("LOCALES_DIR")
	if localesDir == "" {
		localesDir = "/locales"
	}
	i18nBundle, err := i18n.SetupBundle(localesDir)
	if err != nil {
		logger.Fatal("i18n bundle load failed", zap.Error(err), zap.String("locales_dir", localesDir))
	}

	// EmailSender: dev/local capture only this phase (D-60) — a real provider
	// (SES/Postmark) is a stakeholder gate deferred to a later phase.
	//
	// BI-01 fix (04-REVIEW-PRESHIP.md): DevSender writes donor-PII PDFs
	// unencrypted to local disk and performs NO real delivery, so it must never
	// be wired implicitly. Require an explicit MAIL_DEV=1 opt-in; refuse to start
	// otherwise (no real provider is wired yet, so this is fail-fast by design —
	// a production deploy that forgets to configure a real sender will not
	// silently capture PII to /tmp).
	if !mailDevEnabled(os.Getenv) {
		logger.Fatal("refusing to start with the dev-only DevSender email backend: " +
			"set MAIL_DEV=1 to explicitly enable local mail capture (no real email provider is wired yet — BI-01)")
	}
	// MAIL_DEV_OUTDIR defaults to a fixed path so captured messages are easy
	// to find in a running dev container.
	mailDevOutDir := os.Getenv("MAIL_DEV_OUTDIR")
	if mailDevOutDir == "" {
		mailDevOutDir = "/tmp/donnarec-mail-dev"
	}
	emailSender := &mailer.DevSender{OutDir: mailDevOutDir}

	outboxWorker := worker.New(pool, queries, receiptsStore, pdfRenderer, emailSender, i18nBundle, logger, worker.Config{
		PollInterval:    cfg.Worker.PollInterval,
		MaxAttempts:     int32(cfg.Worker.MaxAttempts),
		StuckJobTimeout: cfg.Worker.StuckJobTimeout,
		ComputeBackoff: func(attempts int32) time.Duration {
			return cfg.Worker.ComputeBackoff(int(attempts))
		},
	})

	// Settings service (Phase 4, plan 04-07): Admin-only receipt template/compliance
	// config store (D-58/D-59/NFR-09). Reuses the SAME receiptsStore the worker (above)
	// reads branding images from, and the SAME pdfRenderer for the real-PDF preview path
	// (D-61 — preview must go through the identical sandboxed pipeline as production).
	settingsSvc := settings.NewSettingsService(pool, queries, receiptsStore, logger)

	// e-Donation export service + config accessor (Phase 5, plan 05-02, FR-30/D-75).
	// Reuses the SAME auditSvc + keyProvider as donationSvc — the export's audited
	// decrypt (Service.Export) mirrors RevealPII's discipline exactly (D-64).
	edonationSvc := edonation.NewService(pool, queries, auditSvc, keyProvider, logger)
	edonationCfg := edonation.NewConfig(queries)

	// Donation summary report service (Phase 5, plan 05-05, FR-32/D-70/D-71).
	// Deliberately constructed with ONLY queries — no keyProvider, no auditSvc —
	// since SummaryByMonth/SummaryByDay select no PII column and there is no
	// decrypt/audit-reveal step anywhere on this path.
	reportSvc := report.NewService(queries)

	// App user resolver: maps a Keycloak subject ("sub") -> internal users.id for the
	// auth.ResolveAppUser middleware (bug: created-by-fk-mismatch). Kept as a closure here
	// (not in internal/auth) so the auth package stays DB-agnostic; pgx.ErrNoRows is
	// translated to auth.ErrSubjectNotProvisioned, which the middleware maps to HTTP 403.
	appUserResolver := func(ctx context.Context, subject string) (pgtype.UUID, error) {
		u, err := queries.GetUserByKeycloakSubject(ctx, subject)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgtype.UUID{}, auth.ErrSubjectNotProvisioned
			}
			return pgtype.UUID{}, err
		}
		return u.ID, nil
	}

	// --------------------------------------------------------
	// Handlers
	// --------------------------------------------------------
	userHandler := users.NewUserHandler(userSvc, logger)
	donationHandler := donation.NewDonationHandler(donationSvc, logger)
	slipHandler := donation.NewSlipHandler(slipSvc, logger)
	settingsHandler := settings.NewHandler(settingsSvc, pdfRenderer, logger)
	edonationHandler := edonation.NewHandler(edonationSvc, edonationCfg, logger)
	reportHandler := report.NewHandler(reportSvc, logger)

	// --------------------------------------------------------
	// Flow B public submission (Phase 6, plan 06-03): the FIRST unauthenticated
	// route. Its handler holds the slip StorageClient + the resolved public-web
	// system users.id (D-76). The route group swaps RequireAuth for a per-IP
	// rate limiter + a Cloudflare Turnstile CAPTCHA verifier (D-78/82/83).
	// --------------------------------------------------------
	var publicWebUserID pgtype.UUID
	if err := publicWebUserID.Scan(donation.PublicWebUserID); err != nil {
		logger.Fatal("public-web system user id parse failed", zap.Error(err))
	}
	publicDonationHandler := donation.NewPublicDonationHandler(donationSvc, storageClient, publicWebUserID, logger)

	// Cloudflare Turnstile verifier (fail-closed) + gin middleware. The secret is
	// env-only (TURNSTILE_SECRET_KEY); an empty secret still fails closed against
	// the real siteverify API (rejects every submission) rather than accepting bots.
	captchaMW := captcha.NewMiddleware(captcha.NewTurnstileVerifier(cfg.TurnstileSecretKey))

	// Per-IP token bucket: burst = SubmissionsPerWindow, sustained refill =
	// SubmissionsPerWindow per Window (env-tunable, default 5 per 10 minutes).
	rlRate := rate.Limit(float64(cfg.RateLimit.SubmissionsPerWindow) / cfg.RateLimit.Window.Seconds())
	rlBurst := cfg.RateLimit.SubmissionsPerWindow

	// --------------------------------------------------------
	// Router: middleware chain order matters — see Pattern D
	// --------------------------------------------------------
	router := setupRouter(authMW, auditSvc, appUserResolver, userHandler, donationHandler, slipHandler, settingsHandler, edonationHandler, reportHandler, publicDonationHandler, captchaMW, rlRate, rlBurst, logger)

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

	// Start the outbox worker in its own goroutine, sharing the SAME
	// signal.NotifyContext ctx as the HTTP server for graceful shutdown
	// (Phase 4, plan 04-05 — mirrors the pattern above).
	go outboxWorker.Run(ctx)
	logger.Info("outbox worker started", zap.Duration("poll_interval", cfg.Worker.PollInterval))

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
// Middleware chain order (Pattern D — from PATTERNS.md):
//  1. Recovery   — catch panics before anything else
//  2. Request logger — structured zap logging for all requests
//  3. AuditMiddleware — BEFORE RequireAuth to capture auth-failure events too (D-15)
//  4. Public routes — /healthz (no auth required)
//  5. Protected /api group — RequireAuth()
//  6. Admin /api/admin group — RequireAuth() + RequireRoles(RoleAdmin) + ResolveAppUser
//  7. e-Donation /api/edonation group — RequireAuth() + RequireAnyRole(Checker,Admin) + ResolveAppUser
//  8. Reports /api/reports group — RequireAuth() ONLY, deliberately NO role gate (D-71,
//     Phase 5 plan 05-05) — the report has no PII column, so every authenticated staff
//     member (Maker/Checker/Admin) may view/export it.
func setupRouter(authMW *auth.AuthMiddleware, auditSvc *audit.AuditService, appUserResolver auth.UserIDResolver, userHandler *users.UserHandler, donationHandler *donation.DonationHandler, slipHandler *donation.SlipHandler, settingsHandler *settings.Handler, edonationHandler *edonation.Handler, reportHandler *report.Handler, publicDonationHandler *donation.PublicDonationHandler, captchaMW *captcha.Middleware, rlRate rate.Limit, rlBurst int, logger *zap.Logger) *gin.Engine {
	router := gin.New()

	// 1. Recover from panics — must be first
	router.Use(gin.Recovery())

	// 2. Structured request logging
	router.Use(zapRequestLogger(logger))

	// 3. Audit middleware — BEFORE RequireAuth (Pattern D) so auth events are captured.
	//    Skips plain GETs; audits all mutations + PII-reveal GETs (D-15, D-13).
	router.Use(audit.AuditMiddleware(auditSvc))

	// ---- Public routes ----
	// /healthz: liveness probe (no auth — used by docker-compose healthcheck + load balancers)
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ---- Flow B unauthenticated public submission (Phase 6, plan 06-03, D-78) ----
	// The FIRST route group WITHOUT authMW.RequireAuth. It hangs off the ROOT router
	// (NOT the /api group, which applies RequireAuth), so donors reach it without a
	// session. RequireAuth is substituted by two defensive middlewares, in order:
	//   1. ratelimit.PerIP — cheapest rejection first, before any outbound siteverify
	//   2. captcha.VerifyTurnstile — server-side Cloudflare token verification
	// It still inherits the router-level Recovery/zapLogger/AuditMiddleware above.
	//
	// ASVS V4 (T-06-11): this group registers EXACTLY ONE handler (POST /donations,
	// create one pending_review record) — no update/delete/reveal/approve is ever
	// exposed unauthenticated.
	publicGroup := router.Group("/api/public")
	publicGroup.Use(ratelimit.PerIP(rlRate, rlBurst))
	publicGroup.Use(captchaMW.VerifyTurnstile())
	publicGroup.POST("/donations", publicDonationHandler.CreatePublic)

	// ---- Protected /api group ----
	api := router.Group("/api")
	api.Use(authMW.RequireAuth())

	// GET /api/me — returns JWT subject + email (auth smoke test)
	api.GET("/me", userHandler.Me)

	// ---- Admin /api/admin group (requires admin role — D-01) ----
	adminGroup := api.Group("/admin")
	adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))
	// Resolve Keycloak sub -> users.id for admin routes that need updated_by (Phase 4,
	// plan 04-07 settings save/image-upload) — mirrors donationGroup's ResolveAppUser
	// wiring (bug: created-by-fk-mismatch). POST /users does not consume app_user_id
	// today but requiring the calling admin to be a provisioned users row here too is
	// consistent with every other *_by-writing route in this API.
	adminGroup.Use(auth.ResolveAppUser(appUserResolver, logger))

	// POST /api/admin/users — create user (Admin-only, D-01)
	adminGroup.POST("/users", userHandler.CreateUser)

	// ---- Settings: Admin-only receipt template/compliance config store (Phase 4,
	// plan 04-07, D-58/D-59/D-61, NFR-09) ----
	adminGroup.GET("/settings", settingsHandler.Get)
	adminGroup.PUT("/settings", settingsHandler.Save)
	adminGroup.POST("/settings/images/:slot", settingsHandler.UploadImage)
	adminGroup.POST("/settings/preview", settingsHandler.Preview)
	adminGroup.POST("/settings/preview/pdf", settingsHandler.PreviewPDF)

	// ---- e-Donation field-mapping/threshold config: Admin-only (Phase 5, plan
	// 05-02, D-75/NFR-09 — editable without a deploy) ----
	adminGroup.GET("/edonation-config", edonationHandler.GetConfig)
	adminGroup.PUT("/edonation-config", edonationHandler.UpdateConfig)

	// ---- Maker/Checker/Admin — /api/donations (all staff) ----
	// Pattern E: RequireRoles after RequireAuth — claims must already exist in context.
	// Checker/Admin review actions (approve/return/reject/cancel) wired in plans 03-05/03-06.
	donationGroup := api.Group("/donations")
	donationGroup.Use(auth.RequireAnyRole(auth.RoleMaker, auth.RoleChecker, auth.RoleAdmin))
	// Resolve Keycloak sub -> users.id ONCE for every donation route (bug: created-by-fk-mismatch).
	// Scoped to donationGroup only (all *_by FK writes live here; slip + checker subgroups inherit).
	// Deliberately NOT on the admin group — user provisioning (POST /api/admin/users) has no such FK.
	donationGroup.Use(auth.ResolveAppUser(appUserResolver, logger))
	donationGroup.POST("", donationHandler.Create)
	donationGroup.GET("", donationHandler.List)
	donationGroup.GET("/:id", donationHandler.GetByID)
	donationGroup.PUT("/:id", donationHandler.Update)
	donationGroup.POST("/:id/submit", donationHandler.Submit)

	// ---- Slip attachment routes (Plan 03-04, D-48) ----
	// POST   /:id/slip        — upload a slip file (multipart/form-data, field "file")
	// GET    /:id/slip        — view slip presigned URL (15-min TTL, T-03-16)
	// DELETE /:id/slip        — soft-delete the active slip (D-54)
	donationGroup.POST("/:id/slip", slipHandler.Upload)
	donationGroup.GET("/:id/slip", slipHandler.View)
	donationGroup.DELETE("/:id/slip", slipHandler.Remove)

	// GET /:id/pii — full PII reveal (D-46); route open to all staff but service gates to Checker/Admin.
	// The role check inside RevealPII returns ErrForbidden (403) for Makers.
	// Placed here (not checkerGroup) so a Maker receives 403, not 401/404 (better UX + testability).
	donationGroup.GET("/:id/pii", donationHandler.RevealPII)

	// GET /:id/receipt-pdf — download the frozen receipt PDF via a short-lived presigned
	// URL (FR-28, D-57 "staff download always", plan 04-06). Open to all of
	// Maker/Checker/Admin — placed on donationGroup (not checkerGroup) so every staff
	// role can always retrieve the PDF, matching the RevealPII placement rationale above.
	donationGroup.GET("/:id/receipt-pdf", donationHandler.DownloadReceipt)

	// ---- Checker/Admin review actions (Plans 03-05 + 03-06, D-45, D-47) ----
	// POST /:id/approve  — issue receipt via atomic 7-step tx (INV-1, FR-08)
	// POST /:id/return   — return to draft with mandatory reason (FR-12)
	// POST /:id/reject   — permanently reject with mandatory reason (FR-12)
	// POST /:id/cancel   — void an issued receipt; retains number (FR-19, D-47, plan 03-06)
	// POST /:id/reissue  — void & reissue; creates corrected draft linked via replaces (D-50, plan 03-06)
	//
	// Scoped to Checker + Admin only (defense-in-depth over service-layer SoD and role guards).
	// Inherits parent donationGroup middleware (RequireAuth + Maker/Checker/Admin).
	checkerGroup := donationGroup.Group("")
	checkerGroup.Use(auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin))
	checkerGroup.POST("/:id/approve", donationHandler.Approve)
	checkerGroup.POST("/:id/return", donationHandler.ReturnToDraft)
	checkerGroup.POST("/:id/reject", donationHandler.Reject)
	checkerGroup.POST("/:id/cancel", donationHandler.Cancel)
	checkerGroup.POST("/:id/reissue", donationHandler.Reissue)
	// POST /:id/resend — re-enqueue an outbox issue_receipt job for an issued donation
	// (D-56/D-57, FR-27, plan 04-06). Never allocates a new number or re-renders.
	checkerGroup.POST("/:id/resend", donationHandler.Resend)

	// ---- Checker/Admin — /api/edonation (Phase 5, plan 05-02, FR-30, D-63) ----
	// RequireAnyRole (OR-guard, not RequireRoles/AND — D-63) so a Checker-only or
	// Admin-only user both pass; a Maker-only user gets 403.
	edonationGroup := api.Group("/edonation")
	edonationGroup.Use(auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin))
	edonationGroup.Use(auth.ResolveAppUser(appUserResolver, logger))
	edonationGroup.GET("/export", edonationHandler.Export)
	// POST /keyed + GET /aging (Phase 5, plan 05-04, FR-31, D-67/D-68) — same
	// RequireAnyRole(Checker,Admin) OR-guard as /export; a Maker-only user gets 403.
	edonationGroup.POST("/keyed", edonationHandler.SetKeyed)
	edonationGroup.GET("/aging", edonationHandler.Aging)

	// ---- Reports /api/reports (Phase 5, plan 05-05, FR-32, D-71) ----
	// Deliberately NO RequireAnyRole/RequireRoles — every authenticated staff
	// member (Maker/Checker/Admin) may view/export this PII-free summary report.
	// Region-scoped negative assertion (05-05 Task 2 acceptance criteria) relies
	// on this block containing no role-guard call.
	reportGroup := api.Group("/reports")
	reportGroup.GET("/summary", reportHandler.Summary)
	reportGroup.GET("/export", reportHandler.Export)

	return router
}

// mailDevEnabled reports whether the dev/local capture EmailSender (DevSender)
// is explicitly enabled via MAIL_DEV=1 (BI-01). DevSender writes donor-PII PDFs
// unencrypted to local disk and performs no real delivery, so it must never be
// wired implicitly in a non-dev environment — only an exact "1" opts in.
func mailDevEnabled(getenv func(string) string) bool {
	return getenv("MAIL_DEV") == "1"
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
