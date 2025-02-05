package header2post

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func init() {
	log.SetOutput(os.Stdout)
}

// Config the plugin configuration.
type Config struct {
	NotifyHeader string `yaml:"notifyheader"`
	NotifyUrl    string `yaml:"notifyurl"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// Demo a Demo plugin.
type notify struct {
	next         http.Handler
	notifyHeader string
	notifyUrl    string
	name         string
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
		next:         next,
		name:         name,
		notifyHeader: config.NotifyHeader,
		notifyUrl:    config.NotifyUrl,
	}, nil
}

type injectResponseWriter struct {
	header http.Header
	code   int
	data   []byte
}

func (inject *injectResponseWriter) copy(w http.ResponseWriter) {
	for key, value := range inject.Header() {
		for i, v := range value {
			if i == 0 {
				w.Header().Set(key, v)
				continue
			}
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(inject.code)
	_, err := w.Write(inject.data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newInjectWriter() *injectResponseWriter {
	return &injectResponseWriter{
		header: make(http.Header),
		code:   http.StatusOK,
	}
}

func (i *injectResponseWriter) Header() http.Header {
	return i.header
}
func (i *injectResponseWriter) Write(b []byte) (int, error) {
	i.data = b
	return len(b), nil
}
func (i *injectResponseWriter) WriteHeader(statusCode int) {
	i.code = statusCode
}

// checks for a specific header in the response, extracts its value,
// sends a notification POST request, and logs the result.
func (a *notify) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	respWriter := rw
	a.next.ServeHTTP(respWriter, req)

	value := respWriter.Header().Get(a.notifyHeader)
	if value == "" {
		// respWriter.Header().Del(a.notifyHeader)
		// respWriter.copy(rw)
		return
	}

	// defer func() {
	// 	respWriter.Header().Del(a.notifyHeader)
	// 	respWriter.copy(rw)
	// }()
	// base64 decode
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		log.Println("base64 decode error:", err)
		return
	}
	// post data to notify url
	resp, err := a.post(bytes.NewBuffer(data))
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

func readBody(r io.Reader) ([]byte, error) {
	if mockRead != nil {
		return mockRead(r)
	}
	return io.ReadAll(r)
}

func (a *notify) post(body io.Reader) (*http.Response, error) {
	if mockPost != nil {
		return mockPost(a.notifyUrl, "application/json", body)
	}
	return http.Post(a.notifyUrl, "application/json", body)
}

var mockPost func(url string, contentType string, body io.Reader) (*http.Response, error)
var mockRead func(r io.Reader) ([]byte, error)
