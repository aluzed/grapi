package grapi_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdminHistoryEnforcesDocumentAndLocalePermissions(t *testing.T) {
	for _, scope := range []string{"creator", "locale"} {
		t.Run(scope, func(t *testing.T) {
			fixture := "basic"
			if scope == "locale" {
				fixture = "i18n-dp"
			}
			ts, _, stop := newAdminFixture(t, fixture)
			defer stop()
			c := adminLogin(t, ts.URL)
			_, b, _ := c.do("POST", "/roles", `{"name":"History editors"}`)
			role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
			conditions, properties := `["admin::is-creator"]`, `{}`
			if scope == "locale" {
				conditions, properties = `[]`, `{"locales":["en"]}`
				c.do("POST", "/i18n/locales", `{"code":"fr"}`)
			}
			var perms []string
			for _, action := range []string{"read", "create", "update"} {
				perms = append(perms, fmt.Sprintf(`{"action":"plugin::content-manager.explorer.%s","subject":"api::article.article","properties":%s,"conditions":%s}`, action, properties, conditions))
			}
			if status, b, _ := c.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[`+strings.Join(perms, ",")+`]}`); status != 200 {
				t.Fatalf("permissions: %d %v", status, b)
			}
			_, b, _ = c.do("POST", "/users", `{"firstname":"Editor","email":"editor@example.com","roles":[`+role+`]}`)
			editor := newAdminClient(t, ts.URL)
			_, b, _ = editor.do("POST", "/auth/register", `{"registrationToken":"`+dataOf(b)["registrationToken"].(string)+`","userInfo":{"firstname":"Editor","password":"Edit0rPass"}}`)
			editor.token = dataOf(b)["token"].(string)
			const base = "/content-manager/collection-types/api::article.article"
			status, b, _ := editor.do("POST", base, `{"data":{"title":"Owned"}}`)
			if status != 201 {
				t.Fatalf("create: %d %v", status, b)
			}
			owned := dataOf(b)["documentId"].(string)
			foreign := owned
			if scope == "creator" {
				_, b, _ = c.do("POST", base, `{"data":{"title":"Other admin's secret"}}`)
				foreign = dataOf(b)["documentId"].(string)
			} else {
				if status, b, _ := c.do("PUT", base+"/"+owned, `{"locale":"fr","data":{"title":"French secret"}}`); status != 200 {
					t.Fatalf("French version: %d %v", status, b)
				}
			}
			const history = "/content-manager/history/"
			_, b, _ = c.do("GET", history+"api::article.article/"+foreign, "")
			versions := b["data"].([]any)
			var forbiddenVersion, ownVersion string
			for _, item := range versions {
				v := item.(map[string]any)
				id := fmt.Sprint(int(v["id"].(float64)))
				if scope == "creator" || v["locale"] == "fr" {
					forbiddenVersion = id
				} else {
					ownVersion = id
				}
			}
			if forbiddenVersion == "" {
				t.Fatal("missing forbidden history fixture")
			}
			for _, endpoint := range []struct{ method, path string }{
				{"GET", history + "versions/" + forbiddenVersion},
				{"GET", history + "versions/" + forbiddenVersion + "/diff"},
				{"POST", history + "versions/" + forbiddenVersion + "/restore"},
			} {
				if status, b, _ := editor.do(endpoint.method, endpoint.path, ""); status != 404 {
					t.Errorf("inaccessible history %s: %d %v", endpoint.path, status, b)
				}
			}
			if ownVersion != "" {
				if status, b, _ := editor.do("GET", history+"versions/"+ownVersion+"/diff?against="+forbiddenVersion, ""); status != 404 {
					t.Errorf("diff against a forbidden locale: %d %v", status, b)
				}
			}
			status, b, _ = editor.do("GET", history+"api::article.article/"+foreign, "")
			if status != 200 {
				t.Fatalf("history list: %d %v", status, b)
			}
			for _, item := range b["data"].([]any) {
				v := item.(map[string]any)
				if scope == "creator" || v["locale"] == "fr" {
					t.Errorf("forbidden version in list: %v", v)
				}
			}
			_, b, _ = editor.do("GET", history+"api::article.article/"+owned, "")
			own := b["data"].([]any)
			if len(own) == 0 {
				t.Fatal("the editor must retain access to their allowed history")
			}
			id := fmt.Sprint(int(own[0].(map[string]any)["id"].(float64)))
			for _, endpoint := range []struct{ method, path string }{
				{"GET", history + "versions/" + id},
				{"GET", history + "versions/" + id + "/diff"},
				{"POST", history + "versions/" + id + "/restore"},
			} {
				if status, b, _ := editor.do(endpoint.method, endpoint.path, ""); status != 200 {
					t.Errorf("allowed history %s: %d %v", endpoint.path, status, b)
				}
			}
		})
	}
}
