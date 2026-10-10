package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	c := http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get("http://127.0.0.1:8080/readiness")
	if err != nil {
		os.Exit(1)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		os.Exit(1)
	}
}
