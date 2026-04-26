package main

import (
	"backend/graph"
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vektah/gqlparser/v2/ast"
)

const defaultPort = "8080"

func corsMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://gbialystok:gbialystok_secret@localhost:5433/gbialystok"
	}

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3001"
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}
	log.Println("connected to database")
	log.Println("database schema is expected to be initialized from db/init.sql")

	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{DB: pool},
	}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	cors := corsMiddleware(corsOrigin)

	http.Handle("/", cors(playground.Handler("GraphQL playground", "/query")))
	http.Handle("/query", cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cRole := strings.ToUpper(strings.TrimSpace(r.Header.Get("X-User-Role")))
		cId := strings.TrimSpace(r.Header.Get("X-User-Id"))
		cEmail := strings.TrimSpace(r.Header.Get("X-User-Email"))
		cName := strings.TrimSpace(r.Header.Get("X-User-Name"))

		ctx := context.WithValue(r.Context(), "callerRole", cRole)
		ctx = context.WithValue(ctx, "callerId", cId)
		ctx = context.WithValue(ctx, "callerEmail", cEmail)
		ctx = context.WithValue(ctx, "callerName", cName)
		srv.ServeHTTP(w, r.WithContext(ctx))
	})))

	// Statute REST endpoints (read and update statute, list versions)
	http.Handle("/statute", cors(graph.StatuteHandler(pool)))
	http.Handle("/statute/versions", cors(graph.StatuteVersionsHandler(pool)))

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
