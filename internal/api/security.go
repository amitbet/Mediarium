package api

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/auth"
	"github.com/rdborg/mediarium/internal/httpsec"
)

// securityState is what the server derives from TRUSTED_PROXIES and
// ALLOWED_ORIGINS, built on first use so the zero Server works.
type securityState struct {
	once    sync.Once
	proxies *httpsec.Proxies
	origins []string
	// byUser counts failed sign-ins per account name from every address
	// together, so a password guessed from many addresses at once (which the
	// per-address limit cannot see) is slowed down too.
	byUser *auth.LoginLimiter
	// confirmIP counts wrong password confirmations of signed-in people per
	// address. It is apart from the sign-in limit so one signed-in person
	// cannot use up the address for everyone else, the administrator included.
	confirmIP *auth.LoginLimiter
	// setup is the one-time code for creating the first administrator from
	// outside the home network (setup.go).
	setup setupState
}

// Failed sign-ins tolerated for one account name from all addresses together
// per window. Once they are used up the name is refused for everyone,
// including its real owner, until the oldest failure is a window old: anyone
// who knows a user name can keep that person out this way. It is far above
// the per-address limit, so a stranger needs several addresses to do it, and
// it is the price of stopping a guessing run spread over many addresses.
const (
	userLoginAttempts = 25
	userLoginWindow   = 15 * time.Minute
)

func (s *Server) sec() *securityState {
	st := &s.security
	st.once.Do(func() {
		p, err := httpsec.ParseProxies(s.cfg.TrustedProxies)
		if err != nil {
			// main refuses to start on a bad value; this is for tests and
			// embedders. Trusting nobody is the safe reading.
			slog.Error("security: ignoring invalid TRUSTED_PROXIES", "err", err)
			p, _ = httpsec.ParseProxies("none")
		}
		st.proxies = p
		for _, o := range strings.Split(s.cfg.AllowedOrigins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				st.origins = append(st.origins, o)
			}
		}
		st.byUser = auth.NewLoginLimiter(userLoginAttempts, userLoginWindow)
		st.confirmIP = auth.NewLoginLimiter(5, 15*time.Minute)
	})
	return st
}

// clientIP is the real address of the caller (see httpsec.Proxies.ClientIP).
func (s *Server) clientIP(r *http.Request) string {
	return s.sec().proxies.ClientIP(r)
}

// isHTTPS reports whether the caller reached Mediarium over HTTPS, directly
// or through a trusted proxy.
func (s *Server) isHTTPS(r *http.Request) bool {
	return s.sec().proxies.IsHTTPS(r)
}

// harden wraps the whole app in the security response headers and the
// cross-site request check.
func (s *Server) harden(next http.Handler) http.Handler {
	st := s.sec()
	csrf := st.proxies.CSRF(st.origins, next)
	checked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.oauthOpenPath(r.URL.Path) {
			next.ServeHTTP(w, r) // programs, not a browser with a session (see oauthOpenPath)
			return
		}
		csrf.ServeHTTP(w, r)
	})
	return st.proxies.HeadersWithFrames(checked, s.appFrameSources)
}
