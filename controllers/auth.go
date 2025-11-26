package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/emailing"
	"golang.org/x/crypto/bcrypt"
	"www.theskyscape.com/models"
)

func Auth() (string, *AuthController) {
	return "auth", &AuthController{
		models.Auth.Controller(
			authentication.WithCookie("theskyscape"),
			authentication.WithSigninHandler(func(c *authentication.Controller, user *authentication.User) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if next := r.FormValue("next"); next != "" && strings.HasPrefix(next, "/") {
						c.Redirect(w, r, next)
						return
					}

					c.Refresh(w, r)
				}
			}),
			authentication.WithSignupHandler(func(c *authentication.Controller, user *authentication.User) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					// In the background;
					go func() {
						// Welcome the new user to The Skyscape community
						models.Emails.Send(user.Email,
							"Welcome to The Skyscape",
							emailing.WithTemplate("welcome.html"),
							emailing.WithData("user", user),
							emailing.WithData("year", time.Now().Year()),
						)
					}()

					// While we redirect the user to their profile
					c.Redirect(w, r, "/profile")
				}
			}),
		),
	}
}

type AuthController struct {
	*authentication.Controller
}

func (c *AuthController) Setup(app *application.App) {
	// Call grandparent Setup to avoid double-registering auth routes
	c.Controller.Controller.Setup(app)

	// Register auth routes with rate limiting
	http.HandleFunc("POST /_auth/signup", c.signupWithRateLimit)
	http.HandleFunc("POST /_auth/signin", c.signinWithRateLimit)
	http.HandleFunc("POST /_auth/signout", c.Controller.HandleSignout)

	// Register view routes
	http.Handle("/signin", app.ProtectFunc(c.signin, nil))
	http.Handle("/signup", app.ProtectFunc(c.signup, nil))

	// Password reset routes
	http.Handle("POST /reset-password", app.ProtectFunc(c.resetPassword, nil))
	http.Handle("POST /forgot-password", app.ProtectFunc(c.sendPasswordToken, nil))

	http.Handle("GET /forgot-password", app.Serve("forgot-password.html", func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, _ := c.Authenticate(r)
		if user != nil {
			c.Redirect(w, r, "/")
			return false
		}

		return true
	}))

	http.Handle("GET /reset-password", app.Serve("reset-password.html", func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, _ := c.Authenticate(r)
		if user != nil {
			c.Redirect(w, r, "/")
			return false
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			c.Redirect(w, r, "/forgot-password")
			return false
		}

		return true
	}))
}

func (c AuthController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

var WebHostNames = []string{
	"cloud.digitalocean.com", // health checks
	"skysca.pe",
	"web.skysca.pe", // legacy
	"www.skysca.pe",
	"theskyscape.com",
	"www.theskyscape.com",
}

func (c *AuthController) forward(name string, w http.ResponseWriter, r *http.Request) {
	resource := fmt.Sprintf("http://%s:5000", name)
	url, err := url.Parse(resource)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.ServeHTTP(w, r)
}

func (c *AuthController) Optional(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	if strings.HasSuffix(r.Host, "skysca.pe") {
		if parts := strings.Split(r.Host, "."); len(parts) == 3 {
			c.forward(parts[0], w, r)
			return false
		}
	}

	return c.Controller.Optional(app, w, r)
}

func (c *AuthController) Required(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	if strings.HasSuffix(r.Host, "skysca.pe") {
		if parts := strings.Split(r.Host, "."); len(parts) == 3 {
			c.forward(parts[0], w, r)
			return false
		}
	}

	if ok := c.Controller.Required(app, w, r); !ok {
		return ok
	}

	profile := c.Use("profile").(*ProfileController)
	profile.Request = r
	if profile.CurrentProfile() == nil {
		c.Render(w, r, "setup.html", nil)
		return false
	}

	return true
}

func (c *AuthController) signin(w http.ResponseWriter, r *http.Request) {
	if user, _, _ := c.Authenticate(r); user != nil {
		c.Redirect(w, r, "/")
		return
	}

	c.Render(w, r, "signin.html", nil)
}

func (c *AuthController) signup(w http.ResponseWriter, r *http.Request) {
	if user, _, _ := c.Authenticate(r); user != nil {
		c.Redirect(w, r, "/")
		return
	}

	c.Render(w, r, "signup.html", nil)
}

func (c *AuthController) signinWithRateLimit(w http.ResponseWriter, r *http.Request) {
	// Get client IP address
	ip := c.getClientIP(r)

	// Check rate limit: 5 attempts per 15 minutes
	allowed, _, err := models.Check(ip, "signin", 5, 15*time.Minute)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if !allowed {
		c.Render(w, r, "error-message.html", errors.New("Too many signin attempts. Please try again in 15 minutes."))
		return
	}

	// Record the attempt before calling the handler
	models.Record(ip, "signin", 15*time.Minute)

	// Call the devtools signin handler
	c.Controller.HandleSignin(w, r)

	// Reset rate limit on successful signin (check if a session cookie was set)
	for _, cookie := range w.Header()["Set-Cookie"] {
		if strings.Contains(cookie, "theskyscape=") {
			models.Reset(ip, "signin")
			break
		}
	}
}

func (c *AuthController) signupWithRateLimit(w http.ResponseWriter, r *http.Request) {
	// Get client IP address
	ip := c.getClientIP(r)

	// Check rate limit: 3 attempts per hour
	allowed, _, err := models.Check(ip, "signup", 3, 1*time.Hour)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if !allowed {
		c.Render(w, r, "error-message.html", errors.New("Too many signup attempts. Please try again in 1 hour."))
		return
	}

	// Record the attempt before calling the handler
	models.Record(ip, "signup", 1*time.Hour)

	// Call the devtools signup handler
	c.Controller.HandleSignup(w, r)

	// Reset rate limit on successful signup (check if a session cookie was set)
	for _, cookie := range w.Header()["Set-Cookie"] {
		if strings.Contains(cookie, "theskyscape=") {
			models.Reset(ip, "signup")
			break
		}
	}
}

func (c *AuthController) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies/load balancers)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if idx := strings.Index(forwarded, ","); idx > 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}

	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// Fall back to RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
		return r.RemoteAddr[:idx]
	}

	return r.RemoteAddr
}

func (c *AuthController) sendPasswordToken(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	if user, err := models.Auth.Users.First("WHERE Email = ?", email); err == nil {
		if token, err := models.PasswordResetTokens.Insert(&models.ResetPasswordToken{
			UserID: user.ID,
		}); err == nil {
			err = models.Emails.Send(user.Email, "Skyscape Password Reset Token",
				emailing.WithTemplate("password-reset.html"),
				emailing.WithData("user", user),
				emailing.WithData("year", time.Now().Year()),
				emailing.WithData("resetURL", "https://www.theskyscape.com/reset-password?token="+token.ID))
			if err != nil {
				log.Println("Failed to send password reset email:", err)
			}
		}
	}

	w.Header().Set("Hx-Retarget", "#content")
	w.Write([]byte("If your email is registered with us, you should receive an email with a link to reset your password."))
}

func (c *AuthController) resetPassword(w http.ResponseWriter, r *http.Request) {
	if token := r.FormValue("token"); token == "" {
		c.Render(w, r, "error-message.html", errors.New("missing token"))
		return
	}

	token, err := models.PasswordResetTokens.Get(r.FormValue("token"))
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	user := token.User()
	if user == nil {
		c.Render(w, r, "error-message.html", errors.New("token no longer valid"))
		return
	}

	newPassword := r.FormValue("password")
	confirmPassword := r.FormValue("confirm-password")
	if newPassword != confirmPassword {
		c.Render(w, r, "error-message.html", errors.New("passwords do not match"))
		return
	}

	user.PassHash, err = bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if err = models.Auth.Users.Update(user); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if err = models.PasswordResetTokens.Delete(token); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	session, err := models.Auth.Sessions.Insert(&authentication.Session{
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 30),
	})

	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	cookie, _ := session.Token()
	http.SetCookie(w, &http.Cookie{
		Name:     "theskyscape",
		Value:    cookie,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   true,
	})

	c.Refresh(w, r)

}
