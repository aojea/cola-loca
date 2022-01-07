package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// This function is used for setup before executing the test functions
func TestMain(m *testing.M) {
	// Set Gin to Test Mode
	gin.SetMode(gin.TestMode)

	// Run the other tests
	os.Exit(m.Run())
}

// Helper function to process a request and test its response
func testHTTPResponse(t *testing.T, r *gin.Engine, req *http.Request, f func(w *httptest.ResponseRecorder) bool) {
	// Create a response recorder
	w := httptest.NewRecorder()
	// Create the service and process the above request.
	r.ServeHTTP(w, req)
	if !f(w) {
		t.Fatal(w, req)
	}
}

func TestCreateQueue(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	testApp := NewApp("file::memory:?cache=shared")

	// Create a request to send to the above route
	data := `{"name":"my_login2"}`
	req := httptest.NewRequest("POST", "/api/v1/queue", strings.NewReader(data))
	req.Header.Add("Content-Type", "application/json")
	testHTTPResponse(t, testApp.router, req, func(w *httptest.ResponseRecorder) bool {
		statusOK := w.Code == http.StatusCreated
		return statusOK
	})

	// Create a request to send to the above route
	req = httptest.NewRequest("GET", "/api/v1/queue", nil)
	testHTTPResponse(t, testApp.router, req, func(w *httptest.ResponseRecorder) bool {
		statusOK := w.Code == http.StatusOK

		var q []Queue
		p, err := ioutil.ReadAll(w.Body)
		if err != nil {
			return false
		}
		err = json.Unmarshal(p, &q)
		if err != nil {
			return false
		}
		if len(q) != 1 || q[0].Name != "my_login2" {
			return false
		}
		return statusOK
	})

}
