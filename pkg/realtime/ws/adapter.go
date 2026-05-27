package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracecontext"
	"github.com/huwenlong92/sdkit/core/tracking"
	"github.com/huwenlong92/sdkit/pkg/realtime/transport"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Adapter struct {
	registry Registry
	opts     Options
	upgrader websocket.Upgrader
}

func New(registry Registry, opts Options) *Adapter {
	opts = opts.normalize()
	return &Adapter{
		registry: registry,
		opts:     opts,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  opts.ReadBufferSize,
			WriteBufferSize: opts.WriteBufferSize,
			CheckOrigin:     opts.CheckOrigin,
		},
	}
}

func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.registry == nil {
		http.Error(w, "websocket adapter not configured", http.StatusInternalServerError)
		return
	}
	ctx, responseHeaders := handshakeContext(r)
	for key, values := range responseHeaders {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	conn, err := a.upgrader.Upgrade(w, r, responseHeaders)
	if err != nil {
		transport.Warn(ctx, a.opts.Logger, "websocket upgrade failed", err)
		return
	}
	client := a.clientFromRequest(r)
	a.ServeConn(ctx, conn, client)
}

func (a *Adapter) ServeConn(ctx context.Context, conn *websocket.Conn, client *realtime.Client) {
	if a == nil || a.registry == nil || conn == nil || client == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opts := a.opts.normalize()
	if client.ID == "" {
		client.ID = uuid.NewString()
	}
	if client.Events == nil {
		client.Events = make(map[string]bool)
	}
	if client.Ch == nil {
		client.Ch = make(chan realtime.Event, opts.ClientBufferSize)
	}
	now := time.Now()
	if client.CreatedAt.IsZero() {
		client.CreatedAt = now
	}
	if client.LastActiveAt.IsZero() {
		client.LastActiveAt = now
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer conn.Close()

	if err := a.connect(ctx, client, opts); err != nil {
		transport.Warn(ctx, opts.Logger, "websocket client register failed", err, "client_id", client.ID)
		return
	}
	defer func() {
		if err := a.disconnect(ctx, client, opts); err != nil {
			transport.Warn(ctx, opts.Logger, "websocket client remove failed", err, "client_id", client.ID)
		}
	}()

	writer := &connWriter{conn: conn}
	closer := transport.NewCloser(cancel, conn.Close)

	go func() {
		<-ctx.Done()
		_ = closer.Close()
	}()

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		a.writeLoop(ctx, writer, client, opts, closer)
	}()

	a.readLoop(ctx, conn, client, opts)
	_ = closer.Close()
	<-writeDone
}

func (a *Adapter) readLoop(ctx context.Context, conn *websocket.Conn, client *realtime.Client, opts Options) {
	if opts.MaxMessageSize > 0 {
		conn.SetReadLimit(opts.MaxMessageSize)
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if opts.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(opts.ReadTimeout))
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				transport.Warn(ctx, opts.Logger, "websocket read failed", err, "client_id", client.ID)
			}
			return
		}
		if opts.Lifecycle != nil {
			if err := opts.Lifecycle.OnActivity(ctx, client); err != nil {
				transport.Warn(ctx, opts.Logger, "websocket activity update failed", err, "client_id", client.ID)
			}
		} else {
			client.LastActiveAt = time.Now()
		}
		switch messageType {
		case websocket.TextMessage:
			if opts.OnMessage != nil {
				if err := opts.OnMessage(ctx, client, payload); err != nil {
					transport.Warn(ctx, opts.Logger, "websocket message handler failed", err, "client_id", client.ID)
				}
			}
		case websocket.CloseMessage:
			return
		}
	}
}

func (a *Adapter) connect(ctx context.Context, client *realtime.Client, opts Options) error {
	if opts.Lifecycle != nil {
		return opts.Lifecycle.OnConnect(ctx, client)
	}
	return a.registry.Add(client)
}

func (a *Adapter) disconnect(ctx context.Context, client *realtime.Client, opts Options) error {
	if opts.Lifecycle != nil {
		return opts.Lifecycle.OnDisconnect(ctx, client)
	}
	return a.registry.Remove(client.ID)
}

func (a *Adapter) writeLoop(ctx context.Context, writer *connWriter, client *realtime.Client, opts Options, closer *transport.Closer) {
	var ticker *time.Ticker
	if opts.PingInterval > 0 {
		ticker = time.NewTicker(opts.PingInterval)
		defer ticker.Stop()
	}
	var tick <-chan time.Time
	if ticker != nil {
		tick = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			_ = writer.WriteClose(opts.WriteTimeout)
			return
		case event, ok := <-client.Ch:
			if !ok {
				_ = writer.WriteClose(opts.WriteTimeout)
				_ = closer.Close()
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				transport.WarnTrace(opts.Logger, event.TraceID, "websocket event encode failed", err, "client_id", client.ID)
				continue
			}
			if err := writer.WriteText(payload, opts.WriteTimeout); err != nil {
				transport.WarnTrace(opts.Logger, event.TraceID, "websocket write failed", err, "client_id", client.ID)
				_ = closer.Close()
				return
			}
		case <-tick:
			if err := writer.WritePing(opts.WriteTimeout); err != nil {
				transport.Warn(ctx, opts.Logger, "websocket ping failed", err, "client_id", client.ID)
				_ = closer.Close()
				return
			}
		}
	}
}

type connWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *connWriter) WriteText(payload []byte, timeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if timeout > 0 {
		_ = w.conn.SetWriteDeadline(time.Now().Add(timeout))
	}
	return w.conn.WriteMessage(websocket.TextMessage, payload)
}

func (w *connWriter) WritePing(timeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	return w.conn.WriteControl(websocket.PingMessage, nil, deadline)
}

func (w *connWriter) WriteClose(timeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	return w.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
}

func (a *Adapter) clientFromRequest(r *http.Request) *realtime.Client {
	if a.opts.NewClient != nil {
		if client := a.opts.NewClient(r); client != nil {
			return client
		}
	}
	userID := queryValue(r, "user_id", "uid")
	return &realtime.Client{
		ID:     uuid.NewString(),
		UserID: userID,
		Events: parseEvents(queryValue(r, "events")),
		Ch:     make(chan realtime.Event, a.opts.ClientBufferSize),
	}
}

func handshakeContext(r *http.Request) (context.Context, http.Header) {
	ctx := context.Background()
	if r != nil {
		ctx = tracecontext.ExtractHTTPHeader(r.Context(), r.Header)
	}
	trackID := tracking.TrackID(ctx)
	if trackID == "" {
		trackID = tracking.NewTrackID()
		ctx = tracking.WithTrackID(ctx, trackID)
	}
	reqID := tracecontext.RequestID(ctx)
	if reqID == "" {
		reqID = uuid.NewString()
		ctx = requestid.WithRequestID(ctx, reqID)
	}
	headers := http.Header{}
	headers.Set(tracking.Header, trackID)
	headers.Set(requestid.Header, reqID)
	return ctx, headers
}

func queryValue(r *http.Request, keys ...string) string {
	if r == nil || r.URL == nil {
		return ""
	}
	for _, key := range keys {
		if value := r.URL.Query().Get(key); value != "" {
			return value
		}
	}
	return ""
}

func parseEvents(raw string) map[string]bool {
	events := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		event := strings.TrimSpace(part)
		if event != "" {
			events[event] = true
		}
	}
	return events
}
