package main

import (
	"context"
	"fmt"
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
		t.Fail()
	}
}

func TestCreateQueue(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	testApp := NewApp("file::memory:?cache=shared")

	// Create a request to send to the above route
	req, err := http.NewRequest("POST", "/api/v1/queue", strings.NewReader(`"name":"testq1"`))
	if err != nil {
		t.Fatal(err)
	}
	testHTTPResponse(t, testApp.router, req, func(w *httptest.ResponseRecorder) bool {
		statusOK := w.Code == http.StatusOK
		return statusOK
	})

	// Create a request to send to the above route
	req, err = http.NewRequest("GET", "/api/v1/queue/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	testHTTPResponse(t, testApp.router, req, func(w *httptest.ResponseRecorder) bool {
		statusOK := w.Code == http.StatusOK

		p, err := ioutil.ReadAll(w.Body)
		pageOK := err == nil && strings.Index(string(p), "<title>Register</title>") > 0
		fmt.Println(string(p))
		return statusOK && pageOK
	})

}
