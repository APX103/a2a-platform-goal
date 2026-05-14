package svc

import (
	"database/sql"
	"time"

	"a2a-platform/internal/config"
	"a2a-platform/internal/messagebus"
	"a2a-platform/internal/registry"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"

	_ "modernc.org/sqlite"
)

// ServiceContext holds all shared dependencies for HTTP handlers.
type ServiceContext struct {
	Config      *config.Config
	DB          *sql.DB
	AgentRepo   repository.AgentRepository
	TaskRepo    repository.TaskRepository
	MessageRepo repository.MessageRepository
	TraceRepo   repository.TraceRepository
	Registry    *registry.Registry
	MessageBus  *messagebus.MessageBus
	Tracer      *tracer.Tracer
}

// NewServiceContext creates a ServiceContext from the given config, initialising
// the database, repositories, registry, tracer, and message bus.
func NewServiceContext(c *config.Config) *ServiceContext {
	db, err := sql.Open("sqlite", c.DataSource)
	if err != nil {
		panic("open db: " + err.Error())
	}
	if err := repository.InitDB(db); err != nil {
		panic("init db: " + err.Error())
	}

	agentRepo := repository.NewAgentRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	tr := tracer.New(traceRepo)
	reg := registry.NewWithRetry(agentRepo, c.AgentRetryMax, time.Duration(c.AgentRetryBaseDelay)*time.Second)
	bus := messagebus.NewMessageBus(taskRepo, messageRepo, tr)
	bus.SetRegistry(func(name string) *a2a.Client {
		conn := reg.GetClient(name)
		if conn == nil {
			return nil
		}
		return conn.Client
	})

	return &ServiceContext{
		Config:      c,
		DB:          db,
		AgentRepo:   agentRepo,
		TaskRepo:    taskRepo,
		MessageRepo: messageRepo,
		TraceRepo:   traceRepo,
		Registry:    reg,
		MessageBus:  bus,
		Tracer:      tr,
	}
}
