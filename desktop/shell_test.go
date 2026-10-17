package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(code int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Write([]byte(body))
	}))
}

// A failed read must not look like "no windows saved", or restore would open
// the defaults and the next save would overwrite the user's layout.
func TestSavedWindowsDistinguishesErrorFromEmpty(t *testing.T) {
	for _, c := range []struct {
		code int
		body string
		n    int
		ok   bool
	}{
		{200, "null", 0, true},
		{200, `[{"id":"a","kind":"side"}]`, 1, true},
		{500, `{"error":"boom"}`, 0, false},
		{200, `not json`, 0, false},
	} {
		srv := serve(c.code, c.body)
		list, err := newShell(srv.URL, "t").savedWindows()
		srv.Close()
		if len(list) != c.n || (err == nil) != c.ok {
			t.Errorf("%d %q: got %d windows, err %v", c.code, c.body, len(list), err)
		}
	}
	if _, err := newShell("http://127.0.0.1:1", "t").savedWindows(); err == nil {
		t.Error("unreachable tccd should be an error")
	}
}
