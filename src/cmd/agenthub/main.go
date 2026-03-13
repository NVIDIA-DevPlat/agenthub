package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	agenthub "github.com/NVIDIA-DevPlat/agenthub"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/api"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/auth"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/beads"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/config"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/dolt"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/kanban"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/openclaw"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/store"
	"golang.org/x/term"
)

// Version and Build are set at compile time via -ldflags.
var (
	Version = "dev"
	Build   = "unknown"
)

// openDB is the factory used to open the dolt database. Tests can override it.
var openDB = dolt.Open

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}

	switch args[0] {
	case "serve":
		return cmdServe(args[1:])
	case "setup":
		return cmdSetup(args[1:])
	case "version":
		fmt.Printf("agenthub %s (build %s)\n", Version, Build)
		return nil
	default:
		return fmt.Errorf("unknown command %q — try: serve, setup, version", args[0])
	}
}

func cmdServe(_ []string) error {
	cfgPath := "config.yaml"
	if v := os.Getenv("AGENTHUB_CONFIG"); v != "" {
		cfgPath = v
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	tmpl, err := loadTemplates()
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	// Detect first-run: if the store file doesn't exist, start in setup mode.
	if _, statErr := os.Stat(cfg.Store.Path); os.IsNotExist(statErr) {
		return cmdServeSetupMode(cfg, tmpl)
	}

	// Prompt for the admin password to unlock the encrypted store.
	password, err := readPassword("Admin password: ")
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}

	st, err := store.Open(cfg.Store.Path, password)
	if err != nil {
		return fmt.Errorf("opening secrets store: %w", err)
	}

	adminHash, err := st.Get("admin_password_hash")
	if err != nil {
		return fmt.Errorf("admin password hash not found — run 'agenthub setup' first")
	}

	sessionSecret, err := st.Get("session_secret")
	if err != nil {
		return fmt.Errorf("session secret not found — run 'agenthub setup' first")
	}

	authMgr := auth.NewManager([]byte(sessionSecret), []byte(adminHash), cfg.Server.SessionCookieName)

	db, err := openDB(cfg.Dolt.DSN)
	if err != nil {
		return fmt.Errorf("opening dolt: %w", err)
	}
	defer db.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Wire kanban to beads if configured, otherwise fall back to empty board.
	var kb api.KanbanBuilder
	var beadsClient *beads.Client
	if cfg.Beads.DBPath != "" {
		bc, beadsErr := beads.New(ctx, cfg.Beads.DBPath)
		if beadsErr != nil {
			slog.Warn("beads unavailable, kanban will show empty board", "error", beadsErr)
			kb = &simpleKanbanBuilder{cfg: cfg.Kanban}
		} else {
			beadsClient = bc
			kb = &beadsKanbanBuilder{storage: bc.Storage(), columns: cfg.Kanban.Columns}
		}
	} else {
		kb = &simpleKanbanBuilder{cfg: cfg.Kanban}
	}

	checker := &botChecker{db: db, timeout: cfg.Openclaw.LivenessTimeout, cfg: cfg.Openclaw}

	// Build server options.
	opts := []api.ServerOption{
		api.WithDeleter(db),
		api.WithChecker(checker),
		api.WithRegistrar(db),
		api.WithHealthProber(&openclawProber{cfg: cfg.Openclaw, timeout: cfg.Openclaw.LivenessTimeout}),
		api.WithCapacityReader(db),
	}
	if beadsClient != nil {
		opts = append(opts, api.WithTaskManager(&beadsTaskManager{client: beadsClient}))
	}

	srv := api.NewServer(authMgr, db, kb, st, tmpl, opts...)

	// Serve static files.
	staticSub, err := fs.Sub(agenthub.Static, "web/static")
	if err != nil {
		return fmt.Errorf("static fs sub: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	mux.Handle("/", srv)

	httpSrv := &http.Server{
		Addr:         cfg.Server.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	fmt.Printf("agenthub %s serving on %s\n", Version, cfg.Server.HTTPAddr)

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// cmdServeSetupMode starts the server in first-run setup mode (no store, no dolt).
func cmdServeSetupMode(cfg config.Config, tmpl *template.Template) error {
	// Placeholder auth — login always fails in setup mode (setup page is public).
	authMgr := auth.NewManager([]byte("setup-placeholder-32-bytes-pad!!"), nil, cfg.Server.SessionCookieName)

	srv := api.NewServer(
		authMgr,
		&noopBotLister{},
		&simpleKanbanBuilder{cfg: cfg.Kanban},
		nil,
		tmpl,
		api.WithSetupMode(cfg.Store.Path),
	)

	staticSub, err := fs.Sub(agenthub.Static, "web/static")
	if err != nil {
		return fmt.Errorf("static fs sub: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	mux.Handle("/", srv)

	httpSrv := &http.Server{
		Addr:         cfg.Server.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	fmt.Printf("agenthub %s in setup mode — visit http://localhost%s/admin/setup\n", Version, cfg.Server.HTTPAddr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server (setup mode): %w", err)
	}
	return nil
}

// noopBotLister satisfies api.BotLister with no bots (used in setup mode).
type noopBotLister struct{}

func (n *noopBotLister) ListAllInstances(_ context.Context) ([]*dolt.Instance, error) {
	return nil, nil
}

func cmdSetup(_ []string) error {
	cfgPath := "config.yaml"
	if v := os.Getenv("AGENTHUB_CONFIG"); v != "" {
		cfgPath = v
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Print("Choose admin password: ")
	password, err := readPassword("")
	if err != nil {
		return err
	}

	fmt.Print("Confirm password: ")
	confirm, err := readPassword("")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	st, err := store.Open(cfg.Store.Path, password)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := st.Set("admin_password_hash", hash); err != nil {
		return err
	}

	// Generate a random session secret.
	secret, err := generateSecret(32)
	if err != nil {
		return err
	}
	if err := st.Set("session_secret", secret); err != nil {
		return err
	}

	// Generate a registration token for bot auto-registration.
	regToken, err := generateSecret(16)
	if err != nil {
		return err
	}
	if err := st.Set("registration_token", regToken); err != nil {
		return err
	}

	fmt.Printf("Setup complete. Registration token: %s\nRun 'agenthub serve' to start.\n", regToken)
	return nil
}

func loadTemplates() (*template.Template, error) {
	sub, err := fs.Sub(agenthub.Templates, "web/templates")
	if err != nil {
		return nil, err
	}
	return template.ParseFS(sub, "*.html")
}

// simpleKanbanBuilder returns columns from config with no issues loaded.
type simpleKanbanBuilder struct {
	cfg config.KanbanConfig
}

func (kb *simpleKanbanBuilder) Build(_ context.Context) (*kanban.Board, error) {
	board := &kanban.Board{}
	for _, col := range kb.cfg.Columns {
		board.Columns = append(board.Columns, &kanban.Column{Status: col})
	}
	return board, nil
}

// beadsKanbanBuilder builds a live kanban board from the beads issue tracker.
type beadsKanbanBuilder struct {
	storage kanban.IssueSearcher
	columns []string
}

func (kb *beadsKanbanBuilder) Build(ctx context.Context) (*kanban.Board, error) {
	return kanban.BuildBoard(ctx, kb.storage, kb.columns)
}

// instancesLister is the subset of dolt.DB used by botChecker.
type instancesLister interface {
	ListAllInstances(ctx context.Context) ([]*dolt.Instance, error)
}

// botChecker implements api.BotChecker using a dolt DB and openclaw HTTP client.
type botChecker struct {
	db      instancesLister
	timeout time.Duration
	cfg     config.OpenclawConfig
}

func (bc *botChecker) CheckBot(ctx context.Context, name string) (bool, error) {
	instances, err := bc.db.ListAllInstances(ctx)
	if err != nil {
		return false, fmt.Errorf("listing instances: %w", err)
	}
	for _, inst := range instances {
		if inst.Name == name {
			client := openclaw.NewClient(inst.Host, inst.Port, bc.timeout,
				bc.cfg.HealthPath, bc.cfg.DirectivesPath)
			checkCtx, cancel := context.WithTimeout(ctx, bc.timeout)
			defer cancel()
			err := client.Health(checkCtx)
			return err == nil, err
		}
	}
	return false, fmt.Errorf("bot %q not found", name)
}

// openclawProber satisfies api.HealthProber using a fresh openclaw.Client.
type openclawProber struct {
	cfg     config.OpenclawConfig
	timeout time.Duration
}

func (p *openclawProber) Probe(ctx context.Context, host string, port int) error {
	client := openclaw.NewClient(host, port, p.timeout, p.cfg.HealthPath, p.cfg.DirectivesPath)
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	return client.Health(probeCtx)
}

// doltCapacityUpdater adapts dolt.DB to openclaw.CapacityUpdater.
type doltCapacityUpdater struct{ db *dolt.DB }

func (d *doltCapacityUpdater) UpdateCapacity(ctx context.Context, id string, cap *openclaw.CapacityReport) error {
	return d.db.UpdateCapacity(ctx, id, dolt.Capacity{
		BotID:       id,
		GPUFreeMB:   cap.GPUFreeMB,
		JobsQueued:  cap.JobsQueued,
		JobsRunning: cap.JobsRunning,
	})
}

// beadsTaskManager adapts *beads.Client to api.TaskManager.
type beadsTaskManager struct{ client *beads.Client }

func (m *beadsTaskManager) UpdateStatus(ctx context.Context, issueID, status, note, actor string) error {
	return m.client.UpdateStatus(ctx, issueID, status, note, actor)
}

func (m *beadsTaskManager) GetTask(ctx context.Context, id string) (api.TaskRecord, error) {
	issue, err := m.client.GetTask(ctx, id)
	if err != nil {
		return api.TaskRecord{}, err
	}
	return api.TaskRecord{ID: issue.ID, Title: issue.Title, Status: string(issue.Status)}, nil
}

func (m *beadsTaskManager) CreateTask(ctx context.Context, title, desc, actor string, priority int) (api.TaskRecord, error) {
	issue, err := m.client.CreateTask(ctx, title, desc, actor, priority)
	if err != nil {
		return api.TaskRecord{}, err
	}
	return api.TaskRecord{ID: issue.ID, Title: issue.Title, Status: string(issue.Status)}, nil
}

// readPassword reads a password from stdin with echo suppressed when on a real
// terminal. Falls back to fmt.Scan for pipes and non-TTY environments (tests, CI).
func readPassword(prompt string) (string, error) {
	if prompt != "" {
		fmt.Print(prompt)
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		pw, err := term.ReadPassword(fd)
		if err != nil {
			return "", fmt.Errorf("reading password: %w", err)
		}
		fmt.Println() // emit newline suppressed by ReadPassword
		return string(pw), nil
	}
	var pw string
	if _, err := fmt.Scan(&pw); err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return pw, nil
}

// generateSecret generates n random bytes encoded as hex.
func generateSecret(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	return fmt.Sprintf("%x", buf), nil
}
