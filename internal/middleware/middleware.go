package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

type MiddlewareChain struct {
	middlewares []Middleware
}

func (c *MiddlewareChain) Use(mw Middleware) {
	c.middlewares = append(c.middlewares, mw)
}

func (c *MiddlewareChain) Apply(handler http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}
