package pilot

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type EngineOption func(*Engine)

func WithContext(ctx context.Context) EngineOption {
	return func(e *Engine) {
		nCtx, cancel := context.WithCancel(ctx)
		e.ctx = nCtx
		e.cancel = cancel
	}
}

func WithWatcher(w Watcher) EngineOption {
	return func(e *Engine) {
		e.watcher = w
	}
}

type Engine struct {
	cfg    Config
	router *Router
	server *http.Server

	ctx    context.Context
	cancel context.CancelFunc

	watcher Watcher
}

func NewEngine(cfg Config, opts ...EngineOption) (*Engine, error) {
	cfg.setDefaults()

	engine := &Engine{
		cfg:    cfg,
		router: NewRouter(),
	}

	for _, opt := range opts {
		opt(engine)
	}

	if engine.ctx != nil && engine.cancel != nil {
		ctx, cancel := context.WithCancel(context.Background())
		engine.ctx = ctx
		engine.cancel = cancel
	}

	if engine.watcher == nil {
		watcher, err := NewEtcdWatcher(cfg.Discovery.Etcd.Endpoints, cfg.Discovery.Etcd.DialTimeout, cfg.Discovery.Etcd.DiscoveryPath)
		if err != nil {
			return nil, err
		}

		engine.watcher = watcher
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", engine.router.ServeHTTP)

	var handler http.Handler = mux

	engine.server = &http.Server{
		Addr:           cfg.HTTP.Addr,
		Handler:        handler,
		ReadTimeout:    cfg.HTTP.ReadTimeout,
		WriteTimeout:   cfg.HTTP.WriteTimeout,
		MaxHeaderBytes: cfg.HTTP.MaxHeaderBytes,
	}

	return engine, nil
}

func (e *Engine) Start() error {
	if err := e.watcher.Start(e.ctx); err != nil {
		return err
	}

	go e.handleWatchEvents()
	go e.listenAndServe()

	return nil
}

func (e *Engine) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := e.server.Shutdown(ctx); err != nil {
		return err
	}

	if err := e.watcher.Stop(); err != nil {
		return err
	}

	e.router.Close()
	return nil
}

func (e *Engine) Use(mw ...HandlerFunc) {
	e.router.Use(mw...)
}

func (e *Engine) handleWatchEvents() {
	eventChan := e.watcher.Watch()
	for {
		select {
		case <-e.ctx.Done():
			return
		case event := <-eventChan:
			switch event.Type {
			case EventAdd, EventUpdate:
				if err := e.router.insert(event.Service); err != nil {
					// TODO : log error
				}
			case EventDelete:
				if err := e.router.delete(event.Service); err != nil {
					// TODO : log error
				}
			}
		}
	}
}

func (e *Engine) listenAndServe() {
	if err := e.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// TODO : log error
	}
}
