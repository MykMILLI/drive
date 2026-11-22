package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// MethodSupabaseAuth is used to identify Supabase (GoTrue) auth.
const MethodSupabaseAuth settings.AuthMethod = "supabase"

// supabaseHTTPTimeout bounds every call made to the Supabase Auth server.
const supabaseHTTPTimeout = 10 * time.Second

// SupabaseAuth authenticates users against a (self-hosted) Supabase Auth
// server. Users sign in with email + password, a magic link or an OAuth
// provider configured in Supabase; on first sign-in a matching File Browser
// user is provisioned automatically, keyed by email address.
type SupabaseAuth struct {
	// URL is the public base URL of the Supabase gateway, e.g.
	// https://base.example.com (without /auth/v1).
	URL string `json:"url" yaml:"url"`
	// AnonKey is the Supabase anon (public) API key. It is only ever used
	// server-side, but it is not a secret.
	AnonKey string `json:"anonKey" yaml:"anonKey"`
	// Admins lists email addresses that are granted admin permissions.
	Admins []string `json:"admins" yaml:"admins"`
	// AllowedDomains, when not empty, restricts sign-in to email addresses
	// in these domains.
	AllowedDomains []string `json:"allowedDomains" yaml:"allowedDomains"`
	// Providers lists OAuth providers (google, github, azure, ...) to offer
	// on the login page. They must be enabled in Supabase as well.
	Providers []string `json:"providers" yaml:"providers"`
	// MagicLink enables passwordless sign-in links sent by Supabase.
	MagicLink bool `json:"magicLink" yaml:"magicLink"`
	// AllowLocal keeps local File Browser accounts working (e.g. the
	// original "admin" account) as a break-glass fallback.
	AllowLocal bool `json:"allowLocal" yaml:"allowLocal"`

	// client is overridable for tests.
	client *http.Client
}

type supabaseCred struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	AccessToken string `json:"accessToken"`
}

// SupabaseUser is the subset of the GoTrue user object we rely on.
type SupabaseUser struct {
	ID           string                 `json:"id"`
	Email        string                 `json:"email"`
	AppMetadata  map[string]interface{} `json:"app_metadata"`
	UserMetadata map[string]interface{} `json:"user_metadata"`
}

type supabaseTokenResponse struct {
	AccessToken string       `json:"access_token"`
	User        SupabaseUser `json:"user"`
}

func (a *SupabaseAuth) httpClient() *http.Client {
	if a.client != nil {
		return a.client
	}
	return &http.Client{Timeout: supabaseHTTPTimeout}
}

// SetHTTPClient overrides the HTTP client used to reach Supabase.
func (a *SupabaseAuth) SetHTTPClient(c *http.Client) { a.client = c }

func (a *SupabaseAuth) endpoint(path string) string {
	return strings.TrimRight(a.URL, "/") + "/auth/v1" + path
}

// Auth implements Auther.
func (a *SupabaseAuth) Auth(r *http.Request, usr users.Store, stg *settings.Settings, srv *settings.Server) (*users.User, error) {
	if r.Body == nil {
		return nil, os.ErrPermission
	}

	var cred supabaseCred
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		return nil, os.ErrPermission
	}

	if a.URL == "" || a.AnonKey == "" {
		return nil, errors.New("supabase auth is not configured: url and anonKey are required")
	}

	var (
		sbUser *SupabaseUser
		err    error
	)

	switch {
	case cred.AccessToken != "":
		sbUser, err = a.GetUser(cred.AccessToken)
	case cred.Username != "" && cred.Password != "":
		if strings.Contains(cred.Username, "@") {
			sbUser, err = a.passwordGrant(cred.Username, cred.Password)
		} else {
			err = os.ErrPermission
		}
		// Break-glass: fall back to local accounts when allowed.
		if errors.Is(err, os.ErrPermission) && a.AllowLocal {
			return localAuth(usr, srv, cred.Username, cred.Password)
		}
	default:
		return nil, os.ErrPermission
	}

	if err != nil {
		return nil, err
	}

	return a.provision(sbUser, usr, stg, srv)
}

func localAuth(usr users.Store, srv *settings.Server, username, password string) (*users.User, error) {
	u, err := usr.Get(srv.Root, srv.FollowExternalSymlinks, username)
	hash := dummyHash
	if err == nil {
		hash = u.Password
	}
	if !users.CheckPwd(password, hash) || err != nil {
		return nil, os.ErrPermission
	}
	return u, nil
}

func (a *SupabaseAuth) passwordGrant(email, password string) (*SupabaseUser, error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequest(http.MethodPost, a.endpoint("/token?grant_type=password"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", a.AnonKey)

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, os.ErrPermission
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("supabase: unexpected status %d: %s", resp.StatusCode, string(b))
	}

	var tr supabaseTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("supabase: decoding token response: %w", err)
	}
	if tr.User.Email == "" {
		return nil, os.ErrPermission
	}
	return &tr.User, nil
}

// GetUser validates a Supabase access token by asking the Auth server who it
// belongs to. This avoids having to share the JWT secret with File Browser.
func (a *SupabaseAuth) GetUser(accessToken string) (*SupabaseUser, error) {
	req, err := http.NewRequest(http.MethodGet, a.endpoint("/user"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", a.AnonKey)

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusBadRequest {
		return nil, os.ErrPermission
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase: unexpected status %d", resp.StatusCode)
	}

	var u SupabaseUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("supabase: decoding user: %w", err)
	}
	if u.Email == "" {
		return nil, os.ErrPermission
	}
	return &u, nil
}

// SendMagicLink asks Supabase to email a sign-in link. redirectTo must be in
// the Supabase "Redirect URLs" allow list. Existing users only.
func (a *SupabaseAuth) SendMagicLink(email, redirectTo string) error {
	if !a.MagicLink {
		return os.ErrPermission
	}
	if !a.emailAllowed(email) {
		// Pretend success so the endpoint can't be used to probe the policy.
		return nil
	}
	body, _ := json.Marshal(map[string]interface{}{"email": email, "create_user": false})
	u := a.endpoint("/otp")
	if redirectTo != "" {
		u += "?redirect_to=" + url.QueryEscape(redirectTo)
	}
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", a.AnonKey)

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("supabase: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return fberrors.ErrPermissionDenied
	}
	// Any other status is swallowed so unknown addresses are not revealed.
	return nil
}

func (a *SupabaseAuth) emailAllowed(email string) bool {
	if len(a.AllowedDomains) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range a.AllowedDomains {
		if strings.EqualFold(strings.TrimSpace(d), domain) {
			return true
		}
	}
	return false
}

func (a *SupabaseAuth) isAdmin(u *SupabaseUser) bool {
	for _, e := range a.Admins {
		if strings.EqualFold(strings.TrimSpace(e), u.Email) {
			return true
		}
	}
	// app_metadata can only be written with the service role key, so it is
	// safe to trust for authorization decisions.
	if v, ok := u.AppMetadata["drive_admin"].(bool); ok && v {
		return true
	}
	return false
}

func (a *SupabaseAuth) provision(sb *SupabaseUser, usr users.Store, stg *settings.Settings, srv *settings.Server) (*users.User, error) {
	email := strings.ToLower(strings.TrimSpace(sb.Email))
	if !a.emailAllowed(email) {
		return nil, os.ErrPermission
	}

	user, err := usr.Get(srv.Root, srv.FollowExternalSymlinks, email)
	if errors.Is(err, fberrors.ErrNotExist) {
		return a.createUser(usr, stg, srv, email, a.isAdmin(sb))
	}
	if err != nil {
		return nil, err
	}

	// Promote users that were added to the admin list after provisioning.
	if a.isAdmin(sb) && !user.Perm.Admin {
		user.Perm.Admin = true
		if err := usr.Update(user, "Perm"); err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (a *SupabaseAuth) createUser(usr users.Store, setting *settings.Settings, srv *settings.Server, username string, admin bool) (*users.User, error) {
	const randomPasswordLength = settings.DefaultMinimumPasswordLength + 10
	pwd, err := users.RandomPwd(randomPasswordLength)
	if err != nil {
		return nil, err
	}

	hashed, err := users.ValidateAndHashPwd(pwd, setting.MinimumPasswordLength)
	if err != nil {
		return nil, err
	}

	user := &users.User{
		Username:     username,
		Password:     hashed,
		LockPassword: true,
	}
	setting.Defaults.Apply(user)
	user.Perm.Admin = admin
	if !admin {
		user.Perm.Execute = false
		user.Commands = []string{}
	}

	derivedScope, err := setting.CreateUserHome(user, srv.Root, false)
	if err != nil {
		return nil, err
	}

	if err := usr.SaveProvisioned(user, derivedScope); err != nil {
		return nil, err
	}
	return user, nil
}

// LoginPage tells that Supabase auth requires a login page.
func (a *SupabaseAuth) LoginPage() bool {
	return true
}
