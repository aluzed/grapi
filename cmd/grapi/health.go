package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// health is the container HEALTHCHECK: the image has no curl.
func health() error {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil || port <= 0 || port > 65535 {
		port = 1337
	}
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/_health") //nolint:gosec // loopback, numeric port
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("/_health answered %d", res.StatusCode)
	}
	return nil
}
