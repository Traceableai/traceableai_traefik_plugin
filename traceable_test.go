package traceableai_traefik_plugin

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
	"time"

	"bytes"
	"net"
	"net/http"
)

func TestServeHTTP(t *testing.T) {
	config := &Config{}

	plugin, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, "traefik")
	assert.NoError(t, err)

	requestBody := `{"key": "value"}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(requestBody))
	rw := httptest.NewRecorder()

	plugin.ServeHTTP(rw, req)
}

func TestMakeRequest(t *testing.T) {
	config := &Config{
		TpaEndpoint: "http://example.com",
		ServiceName: "test-service",
	}

	extCapData := ExtCapReqRes{
		Request: HttpRequest{
			Method: "POST",
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"X-Test-Header": "test-value",
			},
			Scheme:        "http",
			Path:          "/test/path",
			Host:          "example.com",
			Body:          []byte(`{"test": "body"}`),
			SourceAddress: "123.432.123.123",
			SourcePort:    8080,
		},
		Response: HttpResponse{
			Headers: map[string]string{
				"Content-Type":           "application/json",
				"X-Test-Response-Header": "test-value",
			},
			Body:       []byte(`{"test": "response body"}`),
			RequestUrl: "http://example.com/test/path?test=foo",
			StatusCode: 200,
		},
	}
	duration := 100 * time.Millisecond

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ext_cap/v1/req_res_cap" {
			t.Errorf("unexpected URL path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected HTTP method: %s", r.Method)
		}

		var extCapData ExtCapReqRes
		json.NewDecoder(r.Body).Decode(&extCapData)
		assert.Equal(t, extCapData.Request.Method, "POST")
		assert.Equal(t, extCapData.Request.Scheme, "http")
		assert.Equal(t, extCapData.Request.Path, "/test/path")
		assert.Equal(t, extCapData.Request.Host, "example.com")
		assert.Equal(t, extCapData.Request.SourceAddress, "123.432.123.123")
		assert.Equal(t, extCapData.Request.SourcePort, int32(8080))
		assert.Equal(t, string(extCapData.Request.Body), "{\"test\": \"body\"}")
		assert.Equal(t, extCapData.Request.Headers["Content-Type"], "application/json")
		assert.Equal(t, extCapData.Request.Headers["X-Test-Header"], "test-value")

		assert.Equal(t, extCapData.Response.StatusCode, int32(200))
		assert.Equal(t, extCapData.Response.RequestUrl, "http://example.com/test/path?test=foo")
		assert.Equal(t, string(extCapData.Response.Body), "{\"test\": \"response body\"}")
		assert.Equal(t, extCapData.Response.Headers["Content-Type"], "application/json")
		assert.Equal(t, extCapData.Response.Headers["X-Test-Response-Header"], "test-value")

		expectedHeaders := map[string]string{
			"traceableai.module.name":          "traefik",
			"traceableai-module-name":          "traefik",
			"traceableai-service-name":         "test-service",
			"traceableai.service.name":         "test-service",
			"traceableai.total_duration_nanos": "100000000",
			"traceableai-total-duration-nanos": "100000000",
			"traceableai.module.version":       VERSION,
			"traceableai-module-version":       VERSION,
			"Content-Type":                     "application/json",
		}
		for k, v := range expectedHeaders {
			if r.Header.Get(k) != v {
				t.Errorf("unexpected header value for %s: %s", k, r.Header.Get(k))
			}
		}
	}))
	defer ts.Close()

	config.TpaEndpoint = ts.URL

	MakeRequest(config, extCapData, duration)
}

func TestCanRecordBody(t *testing.T) {
	testCases := []struct {
		contentType    string
		allowedContent []string
		expected       bool
	}{
		{
			contentType:    "application/json",
			allowedContent: []string{"application/json"},
			expected:       true,
		},
		{
			contentType:    "application/xml",
			allowedContent: []string{"application/json", "application/xml"},
			expected:       true,
		},
		{
			contentType:    "application/grpc",
			allowedContent: []string{"application/json", "application/xml", "application/grpc"},
			expected:       true,
		},
		{
			contentType:    "text/plain",
			allowedContent: []string{"application/json", "application/xml", "application/grpc"},
			expected:       false,
		},
	}

	for _, tc := range testCases {
		headers := make(map[string]string)
		headers["content-type"] = tc.contentType
		config := &Config{
			AllowedContentTypes: tc.allowedContent,
		}
		assert.Equal(t, tc.expected, canRecordBody(headers, config))
	}
}

func TestReadRequestBody(t *testing.T) {
	testBody := "test body"
	req, err := http.NewRequest("POST", "/test", bytes.NewBuffer([]byte(testBody)))
	config := &Config{BodyCaptureSize: 100}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	bodyBytes, err := readRequestBody(req, config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if string(bodyBytes) != testBody {
		t.Errorf("unexpected body, got: %v, want: %v", string(bodyBytes), testBody)
	}
}

func TestSplitIPAndPort(t *testing.T) {
	testAddr := "127.0.0.1:8080"
	testHost := "127.0.0.1"
	testPort := 8080
	host, port, err := splitIPAndPort(testAddr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if host != testHost {
		t.Errorf("unexpected host, got: %v, want: %v", host, testHost)
	}
	if port != testPort {
		t.Errorf("unexpected port, got: %v, want: %v", port, testPort)
	}
}

func TestSplitIPAndPortInvalidInput(t *testing.T) {
	testAddr := "invalid_address"
	_, _, err := splitIPAndPort(testAddr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if _, ok := err.(*net.AddrError); !ok {
		t.Errorf("unexpected error type, got: %T, want: *net.AddrError", err)
	}
}

func TestIsGrpc(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{
			name:    "Content-Type with grpc",
			headers: map[string]string{"content-type": "application/grpc"},
			want:    true,
		},
		{
			name:    "Content-Type without grpc",
			headers: map[string]string{"content-type": "application/json"},
			want:    false,
		},
		{
			name:    "No Content-Type header",
			headers: map[string]string{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGrpc(tt.headers); got != tt.want {
				t.Errorf("isGrpc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRequestUrl(t *testing.T) {
	type testData struct {
		req      *http.Request
		extcap   ExtCapReqRes
		expected string
		ok       bool
	}

	tests := []testData{
		{
			req:      &http.Request{Method: "GET", Host: "example.com", RequestURI: "/path?foo=bar"},
			extcap:   ExtCapReqRes{Request: HttpRequest{Scheme: "https", Host: "example.net"}},
			expected: "https://example.net/path?foo=bar",
			ok:       true,
		},
		{
			req:      &http.Request{Method: "GET", Host: "example.com", RequestURI: "/path?foo=bar"},
			extcap:   ExtCapReqRes{},
			expected: "",
			ok:       false,
		},
		{
			req:      &http.Request{Method: "GET", Host: "example.com", RequestURI: ""},
			extcap:   ExtCapReqRes{},
			expected: "",
			ok:       false,
		},
	}

	for _, test := range tests {
		url, ok := buildRequestUrl(test.req, test.extcap)
		assert.Equal(t, test.expected, url)
		assert.Equal(t, test.ok, ok)
	}
}

func TestGrpcStatusCode(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		copy    bool
	}{
		{
			name:    "trailer:grpc-status with valid status code",
			headers: map[string]string{"trailer:grpc-status": "3"},
			copy:    true,
		},
		{
			name:    "trailer:grpc-status with invalid status code",
			headers: map[string]string{"trailer:grpc-status": "invalid"},
			copy:    true,
		},
		{
			name:    "No trailer:grpc-status header",
			headers: map[string]string{},
			copy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.headers["trailer:grpc-status"]
			setGrpcStatus(tt.headers)
			if tt.copy {
				assert.Equal(t, tt.headers["grpc-status"], original)
				assert.Empty(t, tt.headers["trailer:grpc-status"])
			} else {
				assert.Empty(t, tt.headers["grpc-status"])
			}

		})
	}
}
