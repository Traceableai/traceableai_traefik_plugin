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

const VERSION = "1.0.0"
const WORKERS = 20

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
	queue  chan *http.Request
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

	queue := make(chan *http.Request, 10000)
	client := CreateClient()

	for i := 0; i < WORKERS; i++ {
		go processQueue(queue, client)
	}

	return &Traceable{
		config: config,
		next:   next,
		name:   name,
		queue:  queue,
	}, nil
}

func processQueue(queue chan *http.Request, client *http.Client) {
	for req := range queue {
		sendRequest(client, req, queue)
	}
}

func sendRequest(client *http.Client, req *http.Request, queue chan *http.Request) {
	resp, err := client.Do(req)
	if err != nil {
		// if a request fails, attempt to re-queue it
		enqueue(queue, req)
	}
	if resp != nil {
		// discord the body, otherwise many conns will stay in TIME_WAIT state
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func CreateClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        WORKERS,
		MaxIdleConnsPerHost: WORKERS,
	}

	return &http.Client{Transport: tr}
}

func CreateConfig() *Config {
	return &Config{}
}

func (plugin *Traceable) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	extCap := ExtCapReqRes{}
	extCap.RequestTimeStampInMs = uint64(startTime.UnixMilli())

	extCap.Request = HttpRequest{}
	extCap.Request.Path = req.URL.Path
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

	url, valid := buildRequestUrl(req, extCap)
	if valid {
		extCap.Response.RequestUrl = url
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
		bodyBytes := wrappedWriter.buffer.Bytes()
		limitSize := len(bodyBytes)
		if limitSize > plugin.config.BodyCaptureSize {
			limitSize = plugin.config.BodyCaptureSize
		}
		bodyBytes = bodyBytes[:limitSize]
		extCap.Response.Body = bodyBytes
	}

	if isGrpc(extCap.Response.Headers) {
		setGrpcStatus(extCap.Response.Headers)
	} else {
		extCap.Response.StatusCode = int32(wrappedWriter.statusCode)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	extCapRequest := MakeRequest(plugin.config, extCap, duration)
	if extCapRequest != nil {
		enqueue(plugin.queue, extCapRequest)
	}

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

func MakeRequest(config *Config, extCapData ExtCapReqRes, duration time.Duration) *http.Request {
	url := fmt.Sprintf("%s/ext_cap/v1/req_res_cap", config.TpaEndpoint)

	nanoSeconds := strconv.Itoa(int(duration.Nanoseconds()))
	data, err := json.Marshal(extCapData)
	if err != nil {
		return nil
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil
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

	return req
}

func readRequestBody(req *http.Request, cfg *Config) ([]byte, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	limitSize := len(bodyBytes)
	if limitSize > cfg.BodyCaptureSize {
		limitSize = cfg.BodyCaptureSize
	}
	bodyBytesMax := bodyBytes[:limitSize]
	return bodyBytesMax, nil
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

func enqueue(queue chan *http.Request, req *http.Request) {
	select {
	case queue <- req:
	default:
		// dropped request, otherwise we will block
	}
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

func buildRequestUrl(req *http.Request, extcap ExtCapReqRes) (string, bool) {
	pathWithQueryString := req.RequestURI
	if len(extcap.Request.Scheme) > 0 && len(extcap.Request.Host) > 0 && len(pathWithQueryString) > 0 {
		return extcap.Request.Scheme + "://" + extcap.Request.Host + pathWithQueryString, true
	}
	return "", false
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
