package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// Write-access authentication (2026-10-28).
//
//	API_TOKEN   shared secret. When set, every request that can change data
//	            or spend AI quota (any method other than GET/HEAD/OPTIONS)
//	            must carry "Authorization: Bearer <API_TOKEN>" or
//	            "X-API-Token: <API_TOKEN>"; otherwise 401 UNAUTHORIZED.
//	            The browser uses X-API-Token so that nginx basic auth keeps
//	            the Authorization header. Reads stay public. /healthz is exempt.
//	            Unset = no check (local development), logged at start-up.
//
// nginx basic auth (deploy/xdigest-18089.nginx) is the outer layer; this is
// the one the API itself enforces, so a direct hit on 127.0.0.1 is covered
// too.

func requireAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		log.Printf("WARNING: API_TOKEN not set, write endpoints are unauthenticated")
		return next
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !needsAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-API-Token"))
		if got == "" {
			got, _ = strings.CutPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="xdigest"`)
			writeErr(w, 401, "UNAUTHORIZED", "missing or invalid API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func needsAuth(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return r.URL.Path != "/healthz" && r.URL.Path != "/api/v1/healthz"
}
