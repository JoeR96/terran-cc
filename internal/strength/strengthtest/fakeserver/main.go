// Command fakeserver runs a stand-in for the 99-Strength API on a local
// port, for trying Terran CC's habit widget before (or without) the real
// thing:
//
//	go run ./internal/strength/strengthtest/fakeserver
//	tcc strength connect --api http://127.0.0.1:5099/api/v1 --token <the token it prints>
//
// It keeps what is logged in -state, so stopping and starting it again
// behaves like an outage rather than a wipe.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"terrancc/internal/strength/strengthtest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:5099", "listen address")
	token := flag.String("token", strengthtest.Token, "the personal API token to accept")
	statePath := flag.String("state", "", "file to keep logged values in (optional)")
	slow := flag.Duration("slow", 0, "delay every answer by this long")
	flag.Parse()

	fake := strengthtest.New(*token)
	fake.Slow(*slow)
	if *statePath != "" {
		if b, err := os.ReadFile(*statePath); err == nil {
			if err := fake.Load(b); err != nil {
				log.Fatalf("reading %s: %v", *statePath, err)
			}
		}
	}
	var mu sync.Mutex
	seen := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.ServeHTTP(w, r)
		mu.Lock()
		defer mu.Unlock()
		reqs := fake.Requests()
		for _, q := range reqs[seen:] {
			// The Authorization header is deliberately not printed.
			fmt.Printf("%s %d %s %s %s\n", time.Now().Format("15:04:05"), q.Status, q.Method, q.Path, q.Body)
		}
		seen = len(reqs)
		if *statePath != "" && r.Method != http.MethodGet {
			if err := os.WriteFile(*statePath, fake.Save(), 0o600); err != nil {
				log.Printf("saving %s: %v", *statePath, err)
			}
		}
	})
	fmt.Printf("fake 99-Strength on http://%s/api/v1\ntoken: %s\n", *addr, *token)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
