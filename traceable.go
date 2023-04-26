package traceableai_traefik_plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const VERSION = "0.0.1-dev"

type Config struct {
	AllowedContentTypes []string
	BodyCaptureSize     int
	ServiceName         string
	TpaEndpoint         string
}

type Traceable struct {
	next   http.Handler
	config *Config
	name   string
}

type ExtCapReqRes struct {
	RequestTimeStampInMs uint64       `json:"request_timestamp_in_ms"`
	Request              HttpRequest  `json:"request"`
	Response             HttpResponse `json:"response"`
}

type HttpResponse struct {
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	RequestUrl string            `json:"request_url"`
	StatusCode int32             `json:"status_code"`
}

type HttpRequest struct {
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Scheme        string            `json:"scheme"`
	Path          string            `json:"path"`
	Host          string            `json:"host"`
	Body          []byte            `json:"body"`
	SourceAddress string            `json:"source_address"`
	SourcePort    int32             `json:"source_port"`
}

type responseWriter struct {
	buffer      bytes.Buffer
	wroteHeader bool
	statusCode  int

	http.ResponseWriter
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &Traceable{
		config: config,
		next:   next,
		name:   name,
	}, nil
}

func CreateConfig() *Config {
	return &Config{}
}

func (plugin *Traceable) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	extCap := ExtCapReqRes{}
	extCap.RequestTimeStampInMs = uint64(startTime.UnixMilli())

	extCap.Request = HttpRequest{}
	plainPath := req.URL.Path
	extCap.Request.Path = req.URL.RequestURI()
	extCap.Request.Host = req.Host
	extCap.Request.Method = req.Method

	ip, port, err := splitIPAndPort(req.RemoteAddr)
	if err == nil {
		extCap.Request.SourceAddress = ip
		extCap.Request.SourcePort = int32(port)
	}

	extCap.Request.Headers = make(map[string]string)
	for key, values := range req.Header {
		extCap.Request.Headers[strings.ToLower(key)] = strings.Join(values, ";")
	}

	// use captured headers so they are all normalized
	extCap.Request.Scheme = extCap.Request.Headers["x-forwarded-proto"]

	if canRecordBody(extCap.Request.Headers, plugin.config) {
		body, err := readRequestBody(req, plugin.config)
		if err == nil {
			extCap.Request.Body = body
		}
	}

	extCap.Response = HttpResponse{}
	if len(extCap.Request.Scheme) > 0 && len(extCap.Request.Host) > 0 && len(extCap.Request.Path) > 0 {
		extCap.Response.RequestUrl = extCap.Request.Scheme + "://" + extCap.Request.Host + plainPath
	}

	wrappedWriter := &responseWriter{
		ResponseWriter: rw,
	}

	plugin.next.ServeHTTP(wrappedWriter, req)

	extCap.Response.Headers = make(map[string]string)
	for header, values := range wrappedWriter.ResponseWriter.Header() {
		extCap.Response.Headers[strings.ToLower(header)] = strings.Join(values, ";")
	}

	if canRecordBody(extCap.Response.Headers, plugin.config) {
		extCap.Response.Body = wrappedWriter.buffer.Bytes()
	}

	if isGrpc(extCap.Response.Headers) {
		setGrpcStatus(extCap.Response.Headers)
	} else {
		extCap.Response.StatusCode = int32(wrappedWriter.statusCode)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	MakeRequest(plugin.config, extCap, duration)
}

func isGrpc(headers map[string]string) bool {
	if contentType, ok := headers["content-type"]; ok {
		return strings.Contains(contentType, "grpc")
	}
	return false
}
func setGrpcStatus(headers map[string]string) {
	if statusCode, ok := headers["trailer:grpc-status"]; ok {
		delete(headers, "trailer:grpc-status")
		headers["grpc-status"] = statusCode
	}
}

func MakeRequest(config *Config, extCapData ExtCapReqRes, duration time.Duration) {
	url := fmt.Sprintf("%s/ext_cap/v1/req_res_cap", config.TpaEndpoint)

	nanoSeconds := strconv.Itoa(int(duration.Nanoseconds()))
	data, err := json.Marshal(extCapData)
	if err != nil {
		return
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Add("Content-Type", "application/json")

	req.Header.Add("traceableai.module.name", "traefik")
	req.Header.Add("traceableai-module-name", "traefik")
	req.Header.Add("traceableai-service-name", config.ServiceName)
	req.Header.Add("traceableai.service.name", config.ServiceName)
	req.Header.Add("traceableai.total_duration_nanos", nanoSeconds)
	req.Header.Add("traceableai-total-duration-nanos", nanoSeconds)
	req.Header.Add("traceableai.module.version", VERSION)
	req.Header.Add("traceableai-module-version", VERSION)

	_, _ = http.DefaultClient.Do(req)
}

func readRequestBody(req *http.Request, _ *Config) ([]byte, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes, nil
}

func splitIPAndPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func canRecordBody(headers map[string]string, config *Config) bool {
	value := headers["content-type"]
	if len(value) == 0 {
		return false
	}

	for _, contentType := range config.AllowedContentTypes {
		if strings.Contains(value, contentType) {
			return true
		}
	}
	return false
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.wroteHeader = true
	r.statusCode = statusCode
	r.ResponseWriter.Header().Del("Content-Length")

	r.ResponseWriter.WriteHeader(statusCode)
}
func (r *responseWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	// write back response to client
	r.ResponseWriter.Write(p)

	// capture response segments
	return r.buffer.Write(p)
}
