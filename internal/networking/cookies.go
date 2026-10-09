package networking

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Cookie is one stored cookie with Netscape metadata.
type Cookie struct {
	Domain   string
	Path     string
	Secure   bool
	Expires  time.Time
	Name     string
	Value    string
	HTTPOnly bool
}

func (c Cookie) toNetCookie() *http.Cookie {
	ck := &http.Cookie{
		Name:     c.Name,
		Value:    c.Value,
		Path:     c.Path,
		Domain:   strings.TrimPrefix(c.Domain, "."),
		Secure:   c.Secure,
		HttpOnly: c.HTTPOnly,
	}
	if !c.Expires.IsZero() {
		ck.Expires = c.Expires
	}
	return ck
}

// CookieStore mirrors the cookie jar so it can be persisted to and loaded
// from Netscape/Mozilla cookie files.
type CookieStore struct {
	cookies map[string]Cookie // key: domain|path|name
}

// NewCookieStore returns an empty store.
func NewCookieStore() *CookieStore {
	return &CookieStore{cookies: map[string]Cookie{}}
}

func cookieKey(domain, path, name string) string {
	return domain + "|" + path + "|" + name
}

// Set stores a cookie.
func (s *CookieStore) Set(domain, name, value, path string) {
	if path == "" {
		path = "/"
	}
	s.cookies[cookieKey(domain, path, name)] = Cookie{
		Domain: domain, Path: path, Name: name, Value: value,
	}
}

// All returns all stored cookies.
func (s *CookieStore) All() []Cookie {
	out := make([]Cookie, 0, len(s.cookies))
	for _, c := range s.cookies {
		out = append(out, c)
	}
	return out
}

// Merge adds cookies from another store.
func (s *CookieStore) Merge(o *CookieStore) {
	for k, c := range o.cookies {
		s.cookies[k] = c
	}
}

// Save writes the store in Netscape format.
func (s *CookieStore) Save(path string) error {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range s.All() {
		domain := c.Domain
		includeSub := "FALSE"
		if strings.HasPrefix(domain, ".") {
			includeSub = "TRUE"
		}
		expires := int64(0)
		if !c.Expires.IsZero() {
			expires = c.Expires.Unix()
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		httpOnly := "#HttpOnly_"
		if !c.HTTPOnly {
			httpOnly = ""
		}
		fmt.Fprintf(&b, "%s%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			httpOnly, domain, includeSub, c.Path, secure, expires, c.Name, c.Value)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// LoadNetscapeCookies parses a Netscape cookie file.
func LoadNetscapeCookies(path string) (*CookieStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	store := NewCookieStore()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_") {
			continue
		}
		line = strings.TrimPrefix(line, "#HttpOnly_")
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := fields[0]
		includeSub := fields[1] == "TRUE"
		if includeSub && !strings.HasPrefix(domain, ".") {
			domain = "." + domain
		}
		expires, _ := strconv.ParseInt(fields[4], 10, 64)
		var exp time.Time
		if expires > 0 {
			exp = time.Unix(expires, 0)
		}
		store.cookies[cookieKey(domain, fields[2], fields[5])] = Cookie{
			Domain:  domain,
			Path:    fields[2],
			Secure:  fields[3] == "TRUE",
			Expires: exp,
			Name:    fields[5],
			Value:   fields[6],
			HTTPOnly: strings.HasPrefix(sc.Text(), "#HttpOnly_"),
		}
	}
	return store, sc.Err()
}

func insecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opt-in
}
