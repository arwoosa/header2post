package header2post

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		notifyHeader string
		notifyUrl    string
		expectErr    error
	}{
		{
			name:         "empty notify url",
			notifyHeader: "X-Notify",
			expectErr:    errors.New("notifyurl cannot be empty"),
		},
		{
			name:      "empty notify header",
			notifyUrl: "http://localhost:8000",
			expectErr: errors.New("notifyheader cannot be empty"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := CreateConfig()
			config.NotifyHeader = tt.notifyHeader
			config.NotifyUrl = tt.notifyUrl
			_, err := New(context.Background(), nil, config, tt.name)
			if tt.expectErr != nil && err == nil {
				t.Errorf("New() error = %v, wantErr %v", err, tt.expectErr)
			}
			if tt.expectErr == nil && err != nil {
				t.Errorf("New() error = %v, wantErr %v", err, tt.expectErr)
			}
			if tt.expectErr == nil && err == nil {
				return
			}
			if tt.expectErr.Error() != err.Error() {
				t.Errorf("New() error = %v, wantErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	notifyHeaderKey := "X-Notify"
	tests := []struct {
		name         string
		nextHandler  http.Handler
		expectedCode int
		expectedBody string
		mockPost     func(t *testing.T, req *http.Request) (*http.Response, error)
		mockRead     func(r io.Reader) ([]byte, error)
		expectHeader map[string]string
	}{
		{
			name: "empty notify header",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
		},
		{
			name: "invalid base64 encoded notify header value",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(notifyHeaderKey, "invalid-base64-value")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
		},
		{
			name: "valid base64 encoded notify header value and successful POST request",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(notifyHeaderKey, base64.StdEncoding.EncodeToString([]byte("hello world")))
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("hello world"))
			}),
			mockPost: func(t *testing.T, req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusAccepted,
				}, nil
			},
			expectedCode: http.StatusBadRequest,
			expectedBody: "hello world",
		},
		{
			name: "valid base64 encoded notify header value and failed POST request",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(notifyHeaderKey, base64.StdEncoding.EncodeToString([]byte("hello world")))
				w.Header().Add("Content-Type", "text/plain")
				w.Header().Add("Test", "1")
				w.Header().Add("Test", "2")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			mockPost: func(t *testing.T, req *http.Request) (*http.Response, error) {
				return nil, errors.New("post error")
			},
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
			expectHeader: map[string]string{
				"Content-Type": "text/plain",
				"Test":         "1",
			},
		},
		{
			name: "valid base64 encoded notify header value and invalid notify URL",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(notifyHeaderKey, base64.StdEncoding.EncodeToString([]byte("hello world")))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			mockPost: func(t *testing.T, req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("invalid notify url")),
				}, nil
			},
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
		},
		{
			name: "valid base64 encoded notify header value and read body error",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(notifyHeaderKey, base64.StdEncoding.EncodeToString([]byte("hello world")))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			mockPost: func(t *testing.T, req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("invalid notify url")),
				}, nil
			},
			mockRead: func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read body error")
			},
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
		},
	}

	for _, tt := range tests {
		defer func() {
			mockPost = nil
			mockRead = nil
		}()
		t.Run(tt.name, func(t *testing.T) {
			logBuf := &bytes.Buffer{}
			log.SetOutput(logBuf)
			notify, err := New(nil, tt.nextHandler, &Config{NotifyHeader: notifyHeaderKey, NotifyUrl: "https://example.com/notification"}, "header2post")
			if err != nil {
				t.Errorf("failed to create notify: %v", err)
			}
			mockPost = tt.mockPost
			mockRead = tt.mockRead
			req, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()

			notify.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			if tt.expectHeader != nil {
				for key, value := range tt.expectHeader {
					if w.Header().Get(key) != value {
						t.Errorf("expected header %q to be %q, got %q", key, value, w.Header().Get(key))
					}
				}
			}
		})
	}
}

func TestServeHTTPWithForardHeaders(t *testing.T) {
	apiT = t
	defer func() { apiT = nil }()
	notifyHeaderKey := "X-Notify"
	tests := []struct {
		name           string
		nextHandler    http.Handler
		forwardHeaders string
		expectedCode   int
		expectedBody   string
		mockPost       func(t *testing.T, req *http.Request) (*http.Response, error)
		mockRead       func(r io.Reader) ([]byte, error)
		expectHeader   map[string]string
	}{
		{
			name: "test forward headers",
			nextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Add("X-Test-Forward-A", "a")
				r.Header.Add("X-Test-Forward-B", "b")
				w.Header().Add(notifyHeaderKey, base64.StdEncoding.EncodeToString([]byte("hello world")))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello world"))
			}),
			forwardHeaders: "X-Test-Forward-A,X-Test-Forward-B,X-Test-Forward-C",
			mockPost: func(t *testing.T, req *http.Request) (*http.Response, error) {
				if req.Header.Get("X-Test-Forward-A") != "a" {
					t.Errorf("X-Test-Forward-A header not forwarded")
				}
				if req.Header.Get("X-Test-Forward-B") != "b" {
					t.Errorf("X-Test-Forward-B header not forwarded")
				}
				if req.Header.Get("X-Test-Forward-C") != "" {
					t.Errorf("X-Test-Forward-C header forwarded: %s", req.Header.Get("X-Test-Forward-C"))
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("ok")),
				}, nil
			},
			mockRead: func(r io.Reader) ([]byte, error) {
				return nil, errors.New("read body error")
			},
			expectedCode: http.StatusOK,
			expectedBody: "hello world",
		},
	}

	for _, tt := range tests {
		defer func() {
			mockPost = nil
			mockRead = nil
		}()
		t.Run(tt.name, func(t *testing.T) {
			logBuf := &bytes.Buffer{}
			log.SetOutput(logBuf)

			notify, err := New(nil, tt.nextHandler, &Config{NotifyHeader: notifyHeaderKey, NotifyUrl: "https://example.com/notification", ForwardHeaders: strings.Split(tt.forwardHeaders, ",")}, "header2post")
			if err != nil {
				t.Errorf("failed to create notify: %v", err)
			}
			mockPost = tt.mockPost
			mockRead = tt.mockRead
			req, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()

			notify.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status code %d, got %d", tt.expectedCode, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			if tt.expectHeader != nil {
				for key, value := range tt.expectHeader {
					if w.Header().Get(key) != value {
						t.Errorf("expected header %q to be %q, got %q", key, value, w.Header().Get(key))
					}
				}
			}
		})
	}
}
