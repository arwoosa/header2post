package header2post

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

func init() {
	log.SetOutput(os.Stdout)
}

// Config the plugin configuration.
type Config struct {
	NotifyHeader   string   `yaml:"notifyheader"`
	NotifyUrl      string   `yaml:"notifyurl"`
	ForwardHeaders []string `yaml:"forwardheaders"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// Demo a Demo plugin.
type notify struct {
	next           http.Handler
	forwardHeaders []string
	notifyHeader   string
	notifyUrl      string
	name           string
}

// New created a new Demo plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.NotifyHeader) == 0 {
		return nil, fmt.Errorf("notifyheader cannot be empty")
	}
	if len(config.NotifyUrl) == 0 {
		return nil, fmt.Errorf("notifyurl cannot be empty")
	}

	return &notify{
		next:           next,
		name:           name,
		notifyHeader:   config.NotifyHeader,
		notifyUrl:      config.NotifyUrl,
		forwardHeaders: config.ForwardHeaders,
	}, nil
}

// checks for a specific header in the response, extracts its value,
// sends a notification POST request, and logs the result.
func (a *notify) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	respWriter := newResponseWriter(rw)
	defer func() {
		respWriter.Header().Del(a.notifyHeader)
		respWriter.Flush()
	}()

	a.next.ServeHTTP(respWriter, req)

	value := respWriter.Header().Get(a.notifyHeader)
	if value == "" {
		return
	}

	// base64 decode
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		log.Println("base64 decode error:", err)
		return
	}

	forwardHeaders := make(http.Header)
	for _, k := range a.forwardHeaders {
		v := req.Header.Get(k)
		if v != "" {
			forwardHeaders.Add(k, v)
		}
	}

	// create http request
	myreq, err := http.NewRequest("POST", a.notifyUrl, bytes.NewBuffer(data))
	if err != nil {
		log.Println("create http request error:", err)
		return
	}
	myreq.Header.Set("Content-Type", "application/json")
	var headerValu string
	for _, h := range a.forwardHeaders {
		headerValu = strings.TrimSpace(req.Header.Get(h))
		if headerValu == "" {
			continue
		}
		myreq.Header.Set(h, headerValu)
	}

	// post data to notify url
	resp, err := a.post(myreq)
	if err != nil {
		log.Println("post error:", err)
		return
	}
	if resp.StatusCode == http.StatusAccepted {
		log.Println("notify success")
	} else {
		// read resp bodyf
		bodyBytes, err := readBody(resp.Body)
		if err != nil {
			log.Println("read resp body error:", err)
			return
		}
		log.Println("notify failed: ", string(bodyBytes))
	}
}

var apiT *testing.T

func readBody(r io.Reader) ([]byte, error) {
	if mockRead != nil {
		return mockRead(r)
	}
	return io.ReadAll(r)
}

func (a *notify) post(req *http.Request) (*http.Response, error) {
	if mockPost != nil {
		return mockPost(apiT, req)
	}

	return http.DefaultClient.Do(req)
}

var mockPost func(t *testing.T, req *http.Request) (*http.Response, error)
var mockRead func(r io.Reader) ([]byte, error)

func newResponseWriter(w http.ResponseWriter) *wrappedResponseWriter {
	return &wrappedResponseWriter{w: w, buf: &bytes.Buffer{}, code: http.StatusOK}
}

type wrappedResponseWriter struct {
	w    http.ResponseWriter
	buf  *bytes.Buffer
	code int
}

func (w *wrappedResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *wrappedResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *wrappedResponseWriter) WriteHeader(code int) {
	w.code = code
}

func (w *wrappedResponseWriter) Flush() {
	w.w.WriteHeader(w.code)
	io.Copy(w.w, w.buf)
}

func (w *wrappedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("%T is not an http.Hijacker", w.w)
	}

	return hijacker.Hijack()
}

var (
	_ interface {
		http.ResponseWriter
		http.Hijacker
	} = &wrappedResponseWriter{}
)
