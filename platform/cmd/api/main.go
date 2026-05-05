package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/weiboz0/bridge/platform/internal/auth"
	"github.com/weiboz0/bridge/platform/internal/config"
	"github.com/weiboz0/bridge/platform/internal/db"
	"github.com/weiboz0/bridge/platform/internal/events"
	"github.com/weiboz0/bridge/platform/internal/handlers"
	"github.com/weiboz0/bridge/platform/internal/llm"
	"github.com/weiboz0/bridge/platform/internal/sandbox"
	"github.com/weiboz0/bridge/platform/internal/skills"
)

func main() {
	slog.Info("Starting Bridge Go API server")

	if err := validateDevAuthEnv(os.Getenv); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load("config.toml")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Plan 065 — fail fast at boot when BRIDGE_SESSION_AUTH=1 but
	// the supporting secrets are unset. Otherwise the flag-on
	// production deployment would silently 503 every authenticated
	// request to /api/internal/sessions, which is much harder to
	// notice than a refused boot.
	if err := validateBridgeSessionEnv(cfg); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// Initialize database
	database, err := db.Open(cfg.Database.URL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.Info("Database connected", "url", maskURL(cfg.Database.URL))

	// Plan 068 phase 3 — refuse to start if the latest schema-affecting
	// migration hasn't been applied. Bridge applies 0003+ via psql -f
	// (TODO.md:10) so Drizzle's tracking table is incomplete; this
	// probe checks the actual end-state schema instead. Browser review
	// 010 §P0 #2 caught a tunneled review env serving the parent
	// dashboard against a DB missing parent_links — the page hard-
	// crashed at first request. This guard turns that runtime crash
	// into a startup refusal with a clear remediation message.
	if err := db.CheckSchemaProbe(context.Background(), database); err != nil {
		slog.Error("Schema probe failed", "error", err.Error())
		os.Exit(1)
	}

	// Build stores BEFORE auth middleware so the AdminChecker
	// (plan 065 phase 1) can be wired into the middleware at
	// construction time. The pre-065 order (middleware before
	// stores) didn't allow DI of the store-backed lookup.
	stores := handlers.NewStores(database)

	// Set up auth middleware
	jwtSecret := cfg.Auth.NextAuthSecret
	if jwtSecret == "" {
		slog.Error("No JWT secret configured. Set NEXTAUTH_SECRET in .env")
		os.Exit(1)
	}
	authMw := auth.NewMiddleware(jwtSecret)
	// Plan 065: plumb the bridge.session reader + live-admin
	// AdminChecker onto the middleware now so Phase 3 can flip
	// RequireAuth to consume them without touching this file.
	// Phase 1 builds the wiring; Phase 3 starts using it.
	adminChecker := auth.NewCachedAdminChecker(&auth.SQLAdminLookup{DB: database})
	authMw.WithBridgeSession(
		cfg.BridgeSession.Secrets,
		cfg.BridgeSession.AuthFlag,
		adminChecker,
	)

	// Build LLM backend
	llmBackend, err := llm.CreateBackend(llm.LLMConfig{
		Backend: cfg.LLM.Backend,
		Model:   cfg.LLM.Model,
		APIKey:  cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL,
	})
	if err != nil {
		slog.Warn("LLM backend not configured", "error", err)
	}

	// Build Piston executor for code execution
	var codeExecutor skills.CodeExecutor
	if cfg.Sandbox.PistonURL != "" {
		codeExecutor = sandbox.NewPistonExecutor(sandbox.NewPistonClient(cfg.Sandbox.PistonURL))
		slog.Info("Piston code execution enabled", "url", cfg.Sandbox.PistonURL)
	}

	// Build skill registry (used by agentic loop)
	_ = skills.NewBridgeRegistry(codeExecutor, llmBackend)

	// Shared event broadcaster
	broadcaster := events.NewBroadcaster()

	// Build router
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(slogMiddleware)
	r.Use(middleware.Recoverer)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3003", "http://127.0.0.1:3003"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "Cookie"},
		ExposedHeaders:   []string{"Link", "Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (no auth)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public routes (no auth)
	authH := &handlers.AuthHandler{Users: stores.Users}
	authH.PublicRoutes(r)

	// Optional auth routes (work with or without token)
	meH := &handlers.MeHandler{Orgs: stores.Orgs, Courses: stores.Courses, Classes: stores.Classes}
	r.Group(func(r chi.Router) {
		r.Use(authMw.OptionalAuth)
		meH.OptionalAuthRoutes(r)
	})

	// Plan 053 phase 1: Hocuspocus realtime token endpoints. Two
	// surfaces, two auth styles. The handler is built once here and
	// referenced from both the user-auth group (Routes) and the
	// outside-auth registration below (InternalRoutes).
	//   - POST /api/realtime/token: USER-AUTHENTICATED. Mounted
	//     INSIDE the RequireAuth group via realtimeH.Routes(r).
	//   - POST /api/internal/realtime/auth: SERVER-TO-SERVER.
	//     Mounted OUTSIDE any user-auth middleware so the bearer
	//     check (HOCUSPOCUS_TOKEN_SECRET) runs first; mounting it
	//     under RequireAuth would 401 the unauthenticated callback
	//     before our handler could see the bearer.
	realtimeH := &handlers.RealtimeHandler{
		Sessions:              stores.Sessions,
		Classes:               stores.Classes,
		Orgs:                  stores.Orgs,
		TeachingUnits:         stores.TeachingUnits,
		Problems:              stores.Problems,
		Attempts:              stores.Attempts,
		Users:                 stores.Users,
		ParentLinks:           stores.ParentLinks, // plan 053b phase 4
		HocuspocusTokenSecret: cfg.Realtime.HocuspocusTokenSecret,
	}
	realtimeH.InternalRoutes(r)

	// Plan 065 Phase 1 — Bridge session mint endpoint. Like the
	// realtime internal callback, this is server-to-server only
	// (Auth.js's mint helper sends BRIDGE_INTERNAL_SECRET as a
	// bearer). Mounted OUTSIDE the user-auth group so the bearer
	// check runs first; under RequireAuth the unauthenticated
	// caller would be 401'd before our bearer was inspected.
	internalSessionsH := &handlers.InternalSessionsHandler{
		Users: stores.Users,
		// Sign with the FIRST secret in the rotation list. Phase 3
		// uses the full list for verification.
		PrimarySigningSecret: firstOrEmpty(cfg.BridgeSession.Secrets),
		InternalBearer:       cfg.BridgeSession.InternalBearer,
	}
	internalSessionsH.Routes(r)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(authMw.RequireAuth)

		orgH := &handlers.OrgHandler{Orgs: stores.Orgs, Users: stores.Users}
		orgH.Routes(r)

		// Plan 070 phase 1 — org-admin parent-link CRUD.
		orgParentLinksH := &handlers.OrgParentLinksHandler{
			Orgs:        stores.Orgs,
			ParentLinks: stores.ParentLinks,
			Users:       stores.Users,
		}
		orgParentLinksH.Routes(r)

		courseH := &handlers.CourseHandler{Courses: stores.Courses, Orgs: stores.Orgs}
		courseH.Routes(r)

		topicH := &handlers.TopicHandler{Topics: stores.Topics, Courses: stores.Courses, Orgs: stores.Orgs, TeachingUnits: stores.TeachingUnits}
		topicH.Routes(r)

		problemH := &handlers.ProblemHandler{
			Problems:      stores.Problems,
			TestCases:     stores.TestCases,
			Attempts:      stores.Attempts,
			Solutions:     stores.Solutions,
			TopicProblems: stores.TopicProblems,
			Topics:        stores.Topics,
			Courses:       stores.Courses,
			Orgs:          stores.Orgs,
		}
		problemH.Routes(r)

		solutionH := &handlers.SolutionHandler{
			Problems:      stores.Problems,
			Solutions:     stores.Solutions,
			Orgs:          stores.Orgs,
			TopicProblems: stores.TopicProblems,
			Topics:        stores.Topics,
			Courses:       stores.Courses,
		}
		solutionH.Routes(r)

		topicProblemH := &handlers.TopicProblemHandler{
			Problems:      stores.Problems,
			TopicProblems: stores.TopicProblems,
			Topics:        stores.Topics,
			Courses:       stores.Courses,
			Orgs:          stores.Orgs,
		}
		topicProblemH.Routes(r, problemH.ListProblemsByTopic)

		classH := &handlers.ClassHandler{Classes: stores.Classes, Orgs: stores.Orgs, Users: stores.Users}
		classH.Routes(r)

		sessionH := &handlers.SessionHandler{Sessions: stores.Sessions, Schedules: stores.Schedules, Classes: stores.Classes, Courses: stores.Courses, Topics: stores.Topics, TeachingUnits: stores.TeachingUnits, Orgs: stores.Orgs, ParentLinks: stores.ParentLinks, Broadcaster: broadcaster}
		sessionH.Routes(r)

		scheduleH := &handlers.ScheduleHandler{
			Schedules: stores.Schedules, Sessions: stores.Sessions, Classes: stores.Classes,
			Orgs: stores.Orgs, Broadcaster: broadcaster,
		}
		scheduleH.Routes(r)

		docH := &handlers.DocumentHandler{Documents: stores.Documents}
		docH.Routes(r)

		assignH := &handlers.AssignmentHandler{Assignments: stores.Assignments, Classes: stores.Classes, Orgs: stores.Orgs}
		assignH.Routes(r)

		subH := &handlers.SubmissionHandler{Assignments: stores.Assignments, Classes: stores.Classes}
		subH.Routes(r)

		annotH := &handlers.AnnotationHandler{
			Annotations: stores.Annotations,
			Sessions:    stores.Sessions,
			Classes:     stores.Classes,
			Orgs:        stores.Orgs,
		}
		annotH.Routes(r)

		if llmBackend != nil {
			aiH := &handlers.AIHandler{
				Interactions: stores.Interactions,
				Sessions:     stores.Sessions,
				Classes:      stores.Classes,
				Courses:      stores.Courses,
				Backend:      llmBackend,
				Broadcaster:  broadcaster,
			}
			aiH.Routes(r)
		}

		parentH := &handlers.ParentHandler{
			Reports:     stores.Reports,
			ParentLinks: stores.ParentLinks,
		}
		parentH.Routes(r)

		meH.Routes(r)

		teacherH := &handlers.TeacherHandler{Courses: stores.Courses, Classes: stores.Classes, Orgs: stores.Orgs}
		teacherH.Routes(r)

		// Plan 070 phase 3 — teacher's class-detail parent-link popover.
		teacherParentLinksH := &handlers.TeacherParentLinksHandler{
			Classes:     stores.Classes,
			Orgs:        stores.Orgs,
			ParentLinks: stores.ParentLinks,
		}
		teacherParentLinksH.Routes(r)

		teacherProblemH := &handlers.TeacherProblemHandler{
			Problems:      stores.Problems,
			Topics:        stores.Topics,
			TopicProblems: stores.TopicProblems,
			Classes:       stores.Classes,
			Attempts:      stores.Attempts,
			Users:         stores.Users,
		}
		teacherProblemH.Routes(r)

		// Test runner — only registered when a Piston backend is configured.
		if codeExecutor != nil {
			pistonClient := sandbox.NewPistonClient(cfg.Sandbox.PistonURL)
			attemptTestH := &handlers.AttemptTestHandler{
				Attempts:  stores.Attempts,
				Problems:  stores.Problems,
				TestCases: stores.TestCases,
				Piston:    pistonClient,
			}
			attemptTestH.Routes(r)
		}

		orgDashH := &handlers.OrgDashboardHandler{Orgs: stores.Orgs, Courses: stores.Courses, Classes: stores.Classes, Stats: stores.Stats}
		orgDashH.Routes(r)

		adminH := &handlers.AdminHandler{
			Orgs:        stores.Orgs,
			Users:       stores.Users,
			Stats:       stores.Stats,
			ParentLinks: stores.ParentLinks,
			DB:          database,
			Mw:          authMw,
		}
		adminH.Routes(r)

		unitH := &handlers.TeachingUnitHandler{
			Units:   stores.TeachingUnits,
			Orgs:    stores.Orgs,
			Courses: stores.Courses,
		}
		unitH.Routes(r)

		unitAIH := &handlers.UnitAIHandler{
			Units:   stores.TeachingUnits,
			Orgs:    stores.Orgs,
			Backend: llmBackend, // may be nil — handler returns 503
		}
		unitAIH.Routes(r)

		collectionH := &handlers.UnitCollectionHandler{
			Collections:   stores.UnitCollections,
			Orgs:          stores.Orgs,
			TeachingUnits: stores.TeachingUnits,
		}
		collectionH.Routes(r)

		uploadH := &handlers.UploadHandler{
			UploadDir: "uploads",
		}
		uploadH.Routes(r)

		// Plan 053 phase 1: USER-AUTHENTICATED mint endpoint. The
		// matching server-to-server callback endpoint is registered
		// outside this group (see realtimeH definition above).
		realtimeH.Routes(r)
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}
	slog.Info("Server stopped")
}

func slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// maskURL hides the password portion of a database URL for logging.
func maskURL(url string) string {
	if len(url) > 30 {
		return url[:30] + "..."
	}
	return url
}

// firstOrEmpty returns the first element of a string slice, or empty
// when the slice is nil/empty. Used for the bridge.session signing
// path: the rotation list verifies against any entry, but the mint
// always uses the first.
func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

// validateBridgeSessionEnv refuses to start when the operator has
// turned on plan 065's bridge.session reader (BRIDGE_SESSION_AUTH=1)
// without configuring the secrets it needs.
//
// Why a startup guard instead of just letting the request-time 503
// fire? A silent 503 on every authenticated request looks like a
// generic upstream outage in dashboards; a refused boot fails loudly
// in the deploy pipeline and rolls back. This mirrors plan 050's
// DEV_SKIP_AUTH-in-production guard.
//
// The guard only fires when the flag is ON. With the flag off
// (default), missing secrets are fine — the mint endpoint is
// dormant and the legacy JWE path still works.
func validateBridgeSessionEnv(cfg *config.Config) error {
	if !cfg.BridgeSession.AuthFlag {
		return nil
	}
	if len(cfg.BridgeSession.Secrets) == 0 {
		return fmt.Errorf("refusing to start: BRIDGE_SESSION_AUTH=1 but BRIDGE_SESSION_SECRETS (or BRIDGE_SESSION_SECRET) is empty. Set the signing secret(s) before enabling the flag")
	}
	if cfg.BridgeSession.InternalBearer == "" {
		return fmt.Errorf("refusing to start: BRIDGE_SESSION_AUTH=1 but BRIDGE_INTERNAL_SECRET is empty. The mint endpoint is unreachable without the bearer token")
	}
	return nil
}

// Plan 050: refuse to start when DEV_SKIP_AUTH is set with
// APP_ENV=production. DEV_SKIP_AUTH bypasses authentication entirely
// (any request → fully-privileged dev user). If the variable leaks
// into staging/prod via operator error or secrets-manager misconfig,
// every request becomes admin. Absence-of-APP_ENV is treated as "not
// production" (safe default for dev).
//
// Plan 068 phase 1 — additional layer of defense for tunneled /
// non-localhost dev environments. Browser review 010 §P0 #1 caught a
// tunneled review server running with DEV_SKIP_AUTH=admin and
// APP_ENV=development; the prod-only guard didn't fire because APP_ENV
// wasn't "production". Bridge now ALSO refuses to start when
// DEV_SKIP_AUTH is set AND the operator has declared the host as
// "exposed" via BRIDGE_HOST_EXPOSURE=exposed. The escape hatch
// ALLOW_DEV_AUTH_OVER_TUNNEL=true is for the rare case the operator
// explicitly wants the bypass on a tunneled host (e.g., a private
// demo machine). Defaulting BRIDGE_HOST_EXPOSURE to "localhost" keeps
// local dev friction-free; ops discipline (set "exposed" on shared
// servers) is the gate.
//
// Extracted as a function (taking a getEnv closure) so the guard is
// unit-testable without invoking os.Exit.
func validateDevAuthEnv(getEnv func(string) string) error {
	devSkipAuth := getEnv("DEV_SKIP_AUTH")
	if devSkipAuth == "" {
		return nil
	}
	if appEnv := getEnv("APP_ENV"); appEnv == "production" {
		return fmt.Errorf(
			"refusing to start: DEV_SKIP_AUTH=%q is set with APP_ENV=production. Unset DEV_SKIP_AUTH or set APP_ENV != production",
			devSkipAuth,
		)
	}
	exposure := strings.ToLower(strings.TrimSpace(getEnv("BRIDGE_HOST_EXPOSURE")))
	switch exposure {
	case "", "localhost":
		// Default / explicit localhost — guard does not fire; fall through
		// to the generic dev-bypass warning below.
	case "exposed":
		// Tunneled / shared host. Refuse unless escape hatch is engaged.
		escape := strings.ToLower(strings.TrimSpace(getEnv("ALLOW_DEV_AUTH_OVER_TUNNEL")))
		if escape != "true" {
			return fmt.Errorf(
				"refusing to start: DEV_SKIP_AUTH=%q is set with BRIDGE_HOST_EXPOSURE=exposed. Either unset DEV_SKIP_AUTH (recommended), set BRIDGE_HOST_EXPOSURE=localhost (only when bound to loopback), or set ALLOW_DEV_AUTH_OVER_TUNNEL=true (deliberate escape hatch — use sparingly)",
				devSkipAuth,
			)
		}
		slog.Warn("DEV_SKIP_AUTH is active on an EXPOSED host. ALLOW_DEV_AUTH_OVER_TUNNEL escape hatch is engaged. Identity bypass is reachable from any client that can connect to this server.",
			"DEV_SKIP_AUTH", devSkipAuth)
		return nil
	default:
		// Unknown value (typo, e.g., "EXPOSED" with stray prefix, or
		// an entirely different word). Refuse rather than silently
		// fall through to the generic warning — the operator clearly
		// intended SOMETHING and we don't want a typo to defang the
		// guard. (Codex pass-1 of phase-1 caught the silent-bypass risk.)
		return fmt.Errorf(
			"refusing to start: DEV_SKIP_AUTH=%q is set with BRIDGE_HOST_EXPOSURE=%q (unrecognized). Allowed values are 'localhost' (default) and 'exposed'",
			devSkipAuth, exposure,
		)
	}

	slog.Warn("DEV_SKIP_AUTH is active — all requests bypass authentication. NEVER use in production.",
		"DEV_SKIP_AUTH", devSkipAuth)
	return nil
}
