package oaktree

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfficeActivityClearSecurity(t *testing.T) {
	token := strings.Repeat("a", 64)
	handler := officeHandler(NewStore(t.TempDir()), 4321, token, nil)
	for _, tc := range []struct {
		name, method, origin, auth, content, body string
		want                                      int
	}{
		{"valid", "POST", "http://127.0.0.1:4321", "Bearer " + token, "application/json", "{}", 200},
		{"wrong method", "GET", "http://127.0.0.1:4321", "Bearer " + token, "application/json", "{}", 405},
		{"missing origin", "POST", "", "Bearer " + token, "application/json", "{}", 403},
		{"foreign origin", "POST", "http://evil.invalid", "Bearer " + token, "application/json", "{}", 403},
		{"missing token", "POST", "http://127.0.0.1:4321", "", "application/json", "{}", 401},
		{"wrong type", "POST", "http://127.0.0.1:4321", "Bearer " + token, "text/plain", "{}", 415},
		{"unknown field", "POST", "http://127.0.0.1:4321", "Bearer " + token, "application/json", `{"extra":1}`, 422},
		{"extra value", "POST", "http://127.0.0.1:4321", "Bearer " + token, "application/json", "{} {}", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://127.0.0.1:4321/api/activity/clear", strings.NewReader(tc.body))
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", tc.auth)
			req.Header.Set("Content-Type", tc.content)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status %d, want %d", response.Code, tc.want)
			}
		})
	}
}

func TestOfficeActivityModuleServed(t *testing.T) {
	handler := officeHandler(NewStore(t.TempDir()), 4321, "token", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:4321/activity.js", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "createActivityTracker") {
		t.Fatalf("module not served: %d", response.Code)
	}
}
