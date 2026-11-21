package tarpit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(Tarpit{})
	httpcaddyfile.RegisterHandlerDirective("tarpit", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("tarpit", httpcaddyfile.Before, "abort")
}

const defaultMaxConcurrent = 1024

// Tarpit holds the request open without ever writing a response, until the
// client gives up, Duration elapses, or the server shuts down; then the
// connection is aborted. When MaxConcurrent requests are already held, new
// ones are aborted immediately so the tarpit cannot exhaust the server.
type Tarpit struct {
	// Duration is how long to hold a request. Zero holds it indefinitely.
	Duration caddy.Duration `json:"duration,omitempty"`

	// MaxConcurrent caps how many requests are held at once. Default 1024.
	MaxConcurrent int `json:"max_concurrent,omitempty"`

	slots chan struct{}
	ctx   caddy.Context
}

func (Tarpit) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.tarpit",
		New: func() caddy.Module { return new(Tarpit) },
	}
}

func (t *Tarpit) Provision(ctx caddy.Context) error {
	if t.MaxConcurrent <= 0 {
		t.MaxConcurrent = defaultMaxConcurrent
	}
	t.slots = make(chan struct{}, t.MaxConcurrent)
	t.ctx = ctx
	return nil
}

func (t *Tarpit) ServeHTTP(_ http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	select {
	case t.slots <- struct{}{}:
		defer func() { <-t.slots }()
	default:
		panic(http.ErrAbortHandler)
	}

	var timeout <-chan time.Time
	if t.Duration > 0 {
		timer := time.NewTimer(time.Duration(t.Duration))
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-r.Context().Done():
	case <-t.ctx.Done():
	case <-timeout:
	}
	panic(http.ErrAbortHandler)
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	t := new(Tarpit)
	return t, t.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the handler from Caddyfile tokens:
//
//	tarpit [<matcher>] {
//		duration       <duration>
//		max_concurrent <n>
//	}
func (t *Tarpit) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next()
	if d.NextArg() {
		return d.ArgErr()
	}
	for d.NextBlock(0) {
		switch d.Val() {
		case "duration":
			if !d.NextArg() {
				return d.ArgErr()
			}
			dur, err := caddy.ParseDuration(d.Val())
			if err != nil {
				return d.Errf("parsing duration: %v", err)
			}
			t.Duration = caddy.Duration(dur)
		case "max_concurrent":
			if !d.NextArg() {
				return d.ArgErr()
			}
			n, err := strconv.Atoi(d.Val())
			if err != nil || n <= 0 {
				return d.Errf("max_concurrent must be a positive integer")
			}
			t.MaxConcurrent = n
		default:
			return d.Errf("unrecognized tarpit subdirective '%s'", d.Val())
		}
	}
	return nil
}

var (
	_ caddy.Provisioner           = (*Tarpit)(nil)
	_ caddyhttp.MiddlewareHandler = (*Tarpit)(nil)
	_ caddyfile.Unmarshaler       = (*Tarpit)(nil)
)
