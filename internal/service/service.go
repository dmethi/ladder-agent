package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/dmethi/ladder-agent/internal/fetch"
)

const Socket = "/run/ladder-agent/fetch.sock"
const MaxInput = 8192
const MaxOutput = 16 << 20

func Handler(run func(context.Context, fetch.Input) fetch.Result) http.Handler {
	slots := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/fetch" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxInput)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input fetch.Input
		if err := decoder.Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(fetch.Failure("blocked", "invalid fetch request"))
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(fetch.Failure("blocked", "expected one JSON object"))
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(fetch.Failure("error", "service is busy; retry later"))
			return
		}
		_ = json.NewEncoder(w).Encode(run(r.Context(), input))
	})
}

func Serve(ctx context.Context) error {
	listener, err := net.Listen("unix", Socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(Socket, 0660); err != nil {
		return err
	}
	server := &http.Server{Handler: Handler(fetch.Fetch), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: MaxInput}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func Call(ctx context.Context, input fetch.Input) (fetch.Result, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return fetch.Result{}, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", Socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/fetch", bytes.NewReader(data))
	if err != nil {
		return fetch.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fetch.Result{}, err
	}
	defer resp.Body.Close()
	var result fetch.Result
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxOutput)).Decode(&result); err != nil {
		return fetch.Result{}, err
	}
	return result, nil
}
