package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
)

func main() {
	slog.Info("Starting Bridge Go API server")

	// Load config
	cfg, err := config.Load("config.toml")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
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

	// Set up auth middleware
	jwtSecret := cfg.Auth.NextAuthSecret
	if jwtSecret == "" {
		slog.Error("No JWT secret configured. Set NEXTAUTH_SECRET in .env")
		os.Exit(1)
	}
	authMw := auth.NewMiddleware(jwtSecret)

	// Build stores
	stores := handlers.NewStores(database)

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

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(authMw.RequireAuth)

		orgH := &handlers.OrgHandler{Orgs: stores.Orgs, Users: stores.Users}
		orgH.Routes(r)

		courseH := &handlers.CourseHandler{Courses: stores.Courses, Orgs: stores.Orgs}
		courseH.Routes(r)

		topicH := &handlers.TopicHandler{Topics: stores.Topics, Courses: stores.Courses, Orgs: stores.Orgs}
		topicH.Routes(r)

		classH := &handlers.ClassHandler{Classes: stores.Classes, Orgs: stores.Orgs, Users: stores.Users}
		classH.Routes(r)

		sessionH := &handlers.SessionHandler{Sessions: stores.Sessions, Broadcaster: events.NewBroadcaster()}
		sessionH.Routes(r)

		docH := &handlers.DocumentHandler{Documents: stores.Documents}
		docH.Routes(r)

		assignH := &handlers.AssignmentHandler{Assignments: stores.Assignments, Classes: stores.Classes}
		assignH.Routes(r)

		subH := &handlers.SubmissionHandler{Assignments: stores.Assignments}
		subH.Routes(r)

		annotH := &handlers.AnnotationHandler{Annotations: stores.Annotations}
		annotH.Routes(r)

		classroomH := &handlers.ClassroomHandler{Classrooms: stores.Classrooms, Sessions: stores.Sessions}
		classroomH.Routes(r)

		adminH := &handlers.AdminHandler{Orgs: stores.Orgs, Users: stores.Users, DB: database}
		adminH.Routes(r)
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
