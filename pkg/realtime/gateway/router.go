package gateway

import (
	"strings"
	"sync"

	"github.com/huwenlong92/sdkit/core/realtime"
)

type Router struct {
	mu          sync.RWMutex
	prefix      string
	middlewares []realtime.MiddlewareFunc
	routes      map[string]routeEntry
	parent      *Router
}

type routeEntry struct {
	route realtime.Route
}

func NewRouter(middleware ...realtime.MiddlewareFunc) *Router {
	return &Router{
		middlewares: compactMiddleware(middleware),
		routes:      make(map[string]routeEntry),
	}
}

func (r *Router) Use(middleware ...realtime.MiddlewareFunc) {
	if r == nil {
		return
	}
	root := r.rootRouter()
	root.mu.Lock()
	defer root.mu.Unlock()
	r.middlewares = append(r.middlewares, compactMiddleware(middleware)...)
}

func (r *Router) On(action string, handlers ...realtime.HandlerFunc) {
	if r == nil {
		return
	}
	action = joinAction(r.prefix, action)
	if action == "" || len(handlers) == 0 {
		return
	}
	localHandlers := compactHandlers(handlers)
	if len(localHandlers) == 0 {
		return
	}
	root := r.rootRouter()
	root.mu.Lock()
	defer root.mu.Unlock()
	route := realtime.Route{
		Action:     action,
		Middleware: r.allMiddleware(),
		Handlers:   localHandlers,
	}
	route.Handler = pipeline(localHandlers...)
	route.Compiled = BuildPipeline(&route)
	root.routes[action] = routeEntry{
		route: route,
	}
}

func (r *Router) Group(prefix string, middleware ...realtime.MiddlewareFunc) realtime.Router {
	if r == nil {
		return NewRouter(middleware...)
	}
	group := &Router{
		prefix:      joinAction(r.prefix, prefix),
		middlewares: compactMiddleware(middleware),
		parent:      r,
	}
	return group
}

func (r *Router) Match(action string) (*realtime.Route, bool) {
	entry, ok := r.match(action)
	if !ok {
		return nil, false
	}
	route := realtime.Route{
		Action:     entry.route.Action,
		Middleware: append([]realtime.MiddlewareFunc(nil), entry.route.Middleware...),
		Handler:    entry.route.Handler,
		Compiled:   entry.route.Compiled,
		Handlers:   append([]realtime.HandlerFunc(nil), entry.route.Handlers...),
	}
	return &route, true
}

func (r *Router) rootRouter() *Router {
	if r == nil {
		return nil
	}
	if r.parent != nil {
		return r.parent.rootRouter()
	}
	return r
}

func (r *Router) allMiddleware() []realtime.MiddlewareFunc {
	if r == nil {
		return nil
	}
	var out []realtime.MiddlewareFunc
	if r.parent != nil {
		out = append(out, r.parent.allMiddleware()...)
	}
	out = append(out, r.middlewares...)
	return out
}

func (r *Router) match(action string) (routeEntry, bool) {
	if r == nil {
		return routeEntry{}, false
	}
	root := r.rootRouter()
	root.mu.RLock()
	defer root.mu.RUnlock()
	entry, ok := root.routes[strings.TrimSpace(action)]
	if !ok {
		return routeEntry{}, false
	}
	entry.route.Handlers = append([]realtime.HandlerFunc(nil), entry.route.Handlers...)
	entry.route.Middleware = append([]realtime.MiddlewareFunc(nil), entry.route.Middleware...)
	return entry, true
}

func joinAction(prefix string, action string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	action = strings.Trim(strings.TrimSpace(action), ".")
	switch {
	case prefix == "":
		return action
	case action == "":
		return prefix
	default:
		return prefix + "." + action
	}
}

func compactMiddleware(items []realtime.MiddlewareFunc) []realtime.MiddlewareFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]realtime.MiddlewareFunc, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, item)
		}
	}
	return out
}

func compactHandlers(items []realtime.HandlerFunc) []realtime.HandlerFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]realtime.HandlerFunc, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, item)
		}
	}
	return out
}

var _ realtime.Router = (*Router)(nil)
