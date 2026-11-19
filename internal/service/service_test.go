package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dmethi/ladder-agent/internal/fetch"
)

func TestInputBoundaryRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	calls := 0
	handler := Handler(func(context.Context, fetch.Input) fetch.Result { calls++; return fetch.Failure("error", "unexpected") })
	for _, body := range []string{`{"url":"https://example.com","format":"text","headers":{"Cookie":"x"}}`, `{"url":"https://example.com","format":"text"} {}`, strings.Repeat(" ", MaxInput+1)} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/fetch", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid input: %d", response.Code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached fetcher")
	}
}

func TestServiceConcurrencyLimit(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	handler := Handler(func(context.Context, fetch.Input) fetch.Result {
		entered <- struct{}{}
		<-release
		return fetch.Failure("ok", "")
	})
	invoke := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/fetch", strings.NewReader(`{"url":"https://example.com","format":"text"}`)))
		return r
	}
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { invoke(); done <- struct{}{} }()
	}
	<-entered
	<-entered
	response := invoke()
	if response.Code != http.StatusTooManyRequests {
		t.Fatal("exceeded concurrency limit")
	}
	close(release)
	<-done
	<-done
}
