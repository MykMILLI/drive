package share

import (
	"errors"
	"regexp"
	"strings"
)

type CreateBody struct {
	Password string `json:"password"`
	Expires  string `json:"expires"`
	Unit     string `json:"unit"`
	// Slug is an optional, human-friendly identifier for the link (e.g.
	// "family-photos" -> /share/family-photos). When empty a random hash is
	// generated, as before.
	Slug string `json:"slug"`
}

// ErrInvalidSlug is returned when a custom share slug is malformed.
var ErrInvalidSlug = errors.New("custom links must be 3-64 characters: letters, numbers, '-' or '_', starting with a letter or number")

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)

// NormalizeSlug lowercases and trims a user supplied slug and validates it.
func NormalizeSlug(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !slugRe.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return s, nil
}

// Link is the information needed to build a shareable link.
type Link struct {
	Hash         string `json:"hash" storm:"id,index"`
	Path         string `json:"path" storm:"index"`
	UserID       uint   `json:"userID"`
	Expire       int64  `json:"expire"`
	PasswordHash string `json:"password_hash,omitempty"`
	// Token is a random value that will only be set when PasswordHash is set. It is
	// URL-Safe and is used to download links in password-protected shares via a
	// query arg.
	Token string `json:"token,omitempty"`
}
