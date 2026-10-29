package grapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestAdminFirstRegistrationIsAtomic(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	const requests = 8
	start := make(chan struct{})
	results := make(chan int, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() {
			<-start
			body := fmt.Sprintf(`{"firstname":"First","email":"first%d@example.com","password":"Passw0rd"}`, i)
			res, err := http.Post(ts.URL+"/admin/api/v1/auth/register-admin", "application/json", strings.NewReader(body))
			if err != nil {
				t.Errorf("register: %v", err)
				return
			}
			defer res.Body.Close()
			var b map[string]any
			if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
				t.Errorf("response: %v", err)
			}
			results <- res.StatusCode
		})
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == http.StatusOK {
			success++
		} else if status != http.StatusBadRequest {
			t.Errorf("unexpected registration status: %d", status)
		}
	}
	if success != 1 {
		t.Fatalf("only one first super admin may be registered, got %d", success)
	}
}
