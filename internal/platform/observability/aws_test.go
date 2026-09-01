package observability

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// The AWS exporter is the one adapter CI cannot reach: it needs a region and
// credentials, so a deployment without them never exercises it and a
// dependency bump can change what PutMetricData or PutObject expects with
// nothing here to notice. These tests close that gap by pointing the real SDK
// clients at a local server, so the request the SDK actually builds from our
// inputs is the thing under test rather than a hand-written imitation of it.
//
// They are deliberately not a CloudWatch simulator. What they pin is the part
// this package owns: that our inputs serialize, that batching splits where we
// say it does, that log keys and bodies have the shape an operator's bucket
// policy and log reader depend on, and that a failed call surfaces as an error
// instead of silence.

// recorder captures what reached the fake AWS endpoint. The SDK issues
// requests from its own goroutines, so every field is mutex-guarded — under
// -race an unguarded slice here fails the suite rather than the code.
type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	status   int // response status; 0 means 200
}

type recordedRequest struct {
	method      string
	path        string
	contentType string
	encoding    string
	body        []byte
}

func (rec *recorder) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		rec.mu.Lock()
		rec.requests = append(rec.requests, recordedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			encoding:    r.Header.Get("Content-Encoding"),
			body:        body,
		})
		status := rec.status
		rec.mu.Unlock()

		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		// CloudWatch speaks Smithy's RPC v2 CBOR protocol; an empty CBOR map is
		// the smallest well-formed success body it will deserialize. S3 ignores
		// this and is happy with the 200 alone.
		w.Header().Set("Content-Type", "application/cbor")
		w.Header().Set("smithy-protocol", "rpc-v2-cbor")
		_, _ = w.Write([]byte{0xa0})
	})
}

func (rec *recorder) taken() []recordedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]recordedRequest(nil), rec.requests...)
}

// exporterAgainst builds the exporter the way NewAWSExporter would, except
// pointed at the test server. It bypasses the constructor because that reads
// the environment and resolves the real credential chain; the clients it
// produces are otherwise the same ones, built from the same config.
//
// Retries are capped at one attempt so a deliberate 500 fails immediately
// rather than after the SDK's default backoff, which would otherwise put
// seconds of sleep into the error tests.
func exporterAgainst(srv *httptest.Server, bucket string) *AWSExporter {
	cfg := aws.Config{
		Region: "eu-west-1",
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "id", SecretAccessKey: "secret", Source: "test"}, nil
		}),
	}
	return &AWSExporter{
		region:    "eu-west-1",
		namespace: "Karakuri",
		logBucket: bucket,
		cw: cloudwatch.NewFromConfig(cfg, func(o *cloudwatch.Options) {
			o.BaseEndpoint = aws.String(srv.URL)
			o.RetryMaxAttempts = 1
		}),
		s3: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(srv.URL)
			o.UsePathStyle = true
			o.RetryMaxAttempts = 1
		}),
	}
}

// gunzip returns the request body as the service sees it. PutMetricData is
// sent gzipped, so asserting on the raw bytes would assert on the compressor.
func gunzip(t *testing.T, body []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return plain
}

// An exporter with no clients is the shape NewAWSExporter returns when
// AWS_REGION is unset or the credential chain fails, and it must stay silent:
// the OTel chain keeps flowing through the other exporters, so an error here
// would fail an export that other backends completed.
func TestAWSExporterWithoutClientsExportsNothing(t *testing.T) {
	e := &AWSExporter{namespace: "Karakuri"}
	if e.Active() {
		t.Error("Active() = true with no CloudWatch client")
	}
	if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1}}); err != nil {
		t.Errorf("ExportMetrics = %v, want nil", err)
	}
	if err := e.ExportLogs(context.Background(), []LogRecord{{Message: "x"}}); err != nil {
		t.Errorf("ExportLogs = %v, want nil", err)
	}
}

func TestNewAWSExporterDeclinesWithoutRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("CLOUDWATCH_NAMESPACE", "")

	e := NewAWSExporter()
	if e.Active() {
		t.Error("Active() = true without AWS_REGION")
	}
	if e.namespace != "Karakuri" {
		t.Errorf("namespace = %q, want the Karakuri default", e.namespace)
	}
	if e.Name() != "aws" {
		t.Errorf("Name() = %q, want aws", e.Name())
	}
}

// What our inputs turn into on the wire. The assertions read the CBOR body as
// bytes rather than decoding it: CBOR encodes text as literal UTF-8, so the
// names are findable, and decoding properly would mean taking a CBOR
// dependency to test a payload the SDK is responsible for shaping. The claim
// here is narrower and is the one that matters — our namespace, metric name
// and labels reach the request instead of being dropped on the way.
func TestAWSExporterSendsMetricsToCloudWatch(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	e := exporterAgainst(srv, "logs-bucket")
	err := e.ExportMetrics(context.Background(), []MetricRecord{{
		Name:      "loop_runs",
		Value:     3,
		Labels:    map[string]string{"domain": "software"},
		Timestamp: time.Unix(1700000000, 0).UTC(),
	}})
	if err != nil {
		t.Fatalf("ExportMetrics: %v", err)
	}

	got := rec.taken()
	if len(got) != 1 {
		t.Fatalf("made %d requests, want 1", len(got))
	}
	req := got[0]
	if req.method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.method)
	}
	// The operation is named in the path under RPC v2 CBOR, so this is what
	// says PutMetricData was called rather than something else.
	if !strings.HasSuffix(req.path, "/operation/PutMetricData") {
		t.Errorf("path = %q, want it to name the PutMetricData operation", req.path)
	}
	if req.contentType != "application/cbor" {
		t.Errorf("content-type = %q, want application/cbor", req.contentType)
	}

	body := string(gunzip(t, req.body))
	for _, want := range []string{"Karakuri", "loop_runs", "domain", "software"} {
		if !strings.Contains(body, want) {
			t.Errorf("request body does not carry %q", want)
		}
	}
}

// The batching is ours, not the SDK's: CloudWatch rejects more than 1000
// datapoints per call and ExportMetrics splits at 500. A regression here is
// invisible until a deployment gets busy enough to exceed the limit, which is
// exactly when nobody wants to find it.
func TestAWSExporterBatchesMetricDataAtFiveHundred(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	records := make([]MetricRecord, 501)
	for i := range records {
		records[i] = MetricRecord{Name: "m", Value: float64(i), Timestamp: time.Unix(1700000000, 0).UTC()}
	}

	e := exporterAgainst(srv, "logs-bucket")
	if err := e.ExportMetrics(context.Background(), records); err != nil {
		t.Fatalf("ExportMetrics: %v", err)
	}

	if got := len(rec.taken()); got != 2 {
		t.Errorf("501 records made %d calls, want 2 — the 500-datapoint batch boundary moved", got)
	}
}

func TestAWSExporterSendsNothingForNoRecords(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	e := exporterAgainst(srv, "logs-bucket")
	if err := e.ExportMetrics(context.Background(), nil); err != nil {
		t.Errorf("ExportMetrics(nil) = %v, want nil", err)
	}
	if err := e.ExportLogs(context.Background(), nil); err != nil {
		t.Errorf("ExportLogs(nil) = %v, want nil", err)
	}
	if got := len(rec.taken()); got != 0 {
		t.Errorf("made %d requests for empty batches, want 0", got)
	}
}

// The log archive's shape is a contract with whoever reads the bucket: the key
// layout is what a lifecycle rule and a date-scoped query match on, and NDJSON
// is what makes the object greppable a line at a time.
func TestAWSExporterWritesLogsAsNDJSONToS3(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	e := exporterAgainst(srv, "logs-bucket")
	err := e.ExportLogs(context.Background(), []LogRecord{
		{Level: "info", Message: "first", Timestamp: time.Unix(1700000000, 0).UTC()},
		{Level: "error", Message: "second", Timestamp: time.Unix(1700000001, 0).UTC()},
	})
	if err != nil {
		t.Fatalf("ExportLogs: %v", err)
	}

	got := rec.taken()
	if len(got) != 1 {
		t.Fatalf("made %d requests, want 1", len(got))
	}
	req := got[0]
	if req.method != http.MethodPut {
		t.Errorf("method = %s, want PUT", req.method)
	}
	if req.contentType != "application/x-ndjson" {
		t.Errorf("content-type = %q, want application/x-ndjson", req.contentType)
	}
	// Path-style addressing puts the bucket first; the key is the rest. Matched
	// as a pattern rather than compared against a second time.Now() so the test
	// cannot fail for being run across midnight UTC.
	keyPattern := regexp.MustCompile(`^/logs-bucket/logs/\d{4}-\d{2}-\d{2}/karakuri-\d+\.ndjson$`)
	if !keyPattern.MatchString(req.path) {
		t.Errorf("object path = %q, want /logs-bucket/logs/<date>/karakuri-<n>.ndjson", req.path)
	}

	lines := strings.Split(strings.TrimSuffix(string(req.body), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("body has %d lines, want one JSON object per record", len(lines))
	}
	for i, want := range []string{"first", "second"} {
		var r LogRecord
		if err := json.Unmarshal([]byte(lines[i]), &r); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
		if r.Message != want {
			t.Errorf("line %d message = %q, want %q", i, r.Message, want)
		}
	}
}

// Without a bucket there is nowhere to put logs, and the exporter is still
// live for metrics — so this is a skip, not an error, and it must not reach
// S3 with an empty bucket name.
func TestAWSExporterSkipsLogsWithoutABucket(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	e := exporterAgainst(srv, "")
	if err := e.ExportLogs(context.Background(), []LogRecord{{Message: "x"}}); err != nil {
		t.Errorf("ExportLogs = %v, want nil when no bucket is configured", err)
	}
	if got := len(rec.taken()); got != 0 {
		t.Errorf("made %d requests with no bucket configured, want 0", got)
	}
}

// A rejected export has to be loud. These paths are how an operator learns
// their credentials expired or their bucket policy says no, and returning nil
// would strand that in the dark.
func TestAWSExporterReportsFailedCalls(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*AWSExporter) error
		want string
	}{
		{
			name: "metrics",
			call: func(e *AWSExporter) error {
				return e.ExportMetrics(context.Background(),
					[]MetricRecord{{Name: "m", Value: 1, Timestamp: time.Unix(1700000000, 0).UTC()}})
			},
			want: "PutMetricData",
		},
		{
			name: "logs",
			call: func(e *AWSExporter) error {
				return e.ExportLogs(context.Background(),
					[]LogRecord{{Message: "x", Timestamp: time.Unix(1700000000, 0).UTC()}})
			},
			want: "PutObject",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{status: http.StatusInternalServerError}
			srv := httptest.NewServer(rec.handler())
			defer srv.Close()

			err := tc.call(exporterAgainst(srv, "logs-bucket"))
			if err == nil {
				t.Fatalf("a 500 from AWS returned no error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to name %s", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "aws: ") {
				t.Errorf("error = %v, want it to say which exporter failed", err)
			}
		})
	}
}
