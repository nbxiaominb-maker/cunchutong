package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"bucketd/internal/config"
	"bucketd/internal/handler"
	"bucketd/internal/meta"
	"bucketd/internal/store"
)

var (
	version = "dev"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	host := flag.String("host", "", "bind address")
	port := flag.Int("port", 0, "bind port")
	dataDir := flag.String("data-dir", "", "storage root directory")
	logLevel := flag.String("log-level", "", "log level: debug, info, warn, error")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("bucketd %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if *host != "" {
		cfg.Server.Host = *host
	}
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *dataDir != "" {
		cfg.Storage.DataDir = *dataDir
	}
	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}

	logger := setupLogger(cfg.Logging.Level)
	slog.SetDefault(logger)

	diskStore, err := store.NewDiskStore(cfg.FilesDir(), cfg.Storage.TmpDir)
	if err != nil {
		logger.Error("failed to initialize disk store", "error", err)
		os.Exit(1)
	}

	metaStore, err := meta.NewSQLiteStore(cfg.Database.Path)
	if err != nil {
		logger.Error("failed to initialize metadata store", "error", err)
		os.Exit(1)
	}
	defer metaStore.Close()

	if err := seedAPIKeys(metaStore, cfg); err != nil {
		logger.Error("failed to seed API keys", "error", err)
		os.Exit(1)
	}

	baseURL := fmt.Sprintf("http://%s", cfg.Addr())
	logger.Info("starting bucketd", "addr", cfg.Addr(), "data_dir", cfg.Storage.DataDir)

	apiKeyMap := handler.BuildAPIKeyMap(convertAPIKeys(cfg.Security.APIKeys))
	mw := handler.NewMiddleware(apiKeyMap, cfg.CORS.AllowedOrigins, logger)

	uploadH := handler.NewUploadHandler(diskStore, metaStore, cfg.Server.MaxUploadSize, baseURL)
	downloadH := handler.NewDownloadHandler(diskStore, metaStore)
	deleteH := handler.NewDeleteHandler(diskStore, metaStore)
	listH := handler.NewListHandler(metaStore, baseURL)
	updateH := handler.NewUpdateHandler(metaStore, baseURL)
	healthH := handler.NewHealthHandler(metaStore)
	webUIH := handler.NewWebUIHandler()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", webUIH.Serve)
	mux.HandleFunc("GET /healthz", healthH.Healthz)

	mux.HandleFunc("POST /api/v1/files", uploadH.Upload)
	mux.HandleFunc("POST /api/v1/multipart/init", uploadH.MultipartInit)
	mux.HandleFunc("PUT /api/v1/multipart/{uploadID}/chunks/{chunkNumber}", uploadH.MultipartChunk)
	mux.HandleFunc("POST /api/v1/multipart/{uploadID}/complete", uploadH.MultipartComplete)
	mux.HandleFunc("GET /api/v1/files", listH.List)
	mux.HandleFunc("GET /api/v1/files/{id}", listH.GetFile)
	mux.HandleFunc("PUT /api/v1/files/{id}", updateH.Update)
	mux.HandleFunc("DELETE /api/v1/files/{id}", deleteH.Delete)

	mux.HandleFunc("GET /f/{hash}", downloadH.ServeFile)
	mux.HandleFunc("GET /f/{hash}/thumb", downloadH.ServeThumbnail)

	var h http.Handler = mux
	h = mw.CORS(h)
	h = mw.Auth(h)
	h = mw.Logging(h)

	server := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      h,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	if err := server.ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func seedAPIKeys(m *meta.SQLiteStore, cfg *config.Config) error {
	var keys []struct {
		Name        string
		KeyHash     string
		Permissions []string
	}
	for _, k := range cfg.Security.APIKeys {
		keys = append(keys, struct {
			Name        string
			KeyHash     string
			Permissions []string
		}{
			Name:        k.Name,
			KeyHash:     handler.HashAPIKey(k.Key),
			Permissions: k.Permissions,
		})
	}
	return m.SeedAPIKeys(keys)
}

func convertAPIKeys(keys []config.APIKeyConfig) []struct {
	Name        string
	Key         string
	Permissions []string
} {
	result := make([]struct {
		Name        string
		Key         string
		Permissions []string
	}, len(keys))
	for i, k := range keys {
		result[i].Name = k.Name
		result[i].Key = k.Key
		result[i].Permissions = k.Permissions
	}
	return result
}
