package security

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// CheckReverseProxy redirects apex domain to www and forwards app subdomains.
// Returns true if the request was handled (redirected or forwarded).
func CheckReverseProxy(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	// Redirect apex domain to www to avoid cookie issues
	if r.Host == "theskyscape.com" {
		target := "https://www.theskyscape.com" + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return true
	}

	// Forward app subdomains to their containers
	if strings.HasSuffix(r.Host, "skysca.pe") {
		if parts := strings.Split(r.Host, "."); len(parts) == 3 {
			forward(parts[0], w, r)
			return true
		}
	}

	return false
}

// forward forwards requests to a specific container
func forward(name string, w http.ResponseWriter, r *http.Request) {
	resource := fmt.Sprintf("http://%s:5000", name)
	url, err := url.Parse(resource)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.ServeHTTP(w, r)
}
