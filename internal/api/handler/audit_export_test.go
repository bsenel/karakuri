package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/feature/audit"
)

// exportBytes is deliberately not what encoding/json would write: odd spacing
// and unsorted keys, so a handler that decodes and re-encodes is caught.
const exportBytes = `{"b":1,  "a":2}`

// fakeExporter records the window it was asked for and answers fixed bytes or
// a fixed error.
type fakeExporter struct {
	calls         int
	from, to, now time.Time
	err           error
}

func (f *fakeExporter) Export(_ context.Context, from, to, now time.Time) ([]byte, error) {
	f.calls++
	f.from, f.to, f.now = from, to, now
	if f.err != nil {
		return nil, f.err
	}
	return []byte(exportBytes), nil
}

const (
	exportFrom = "2026-01-01T00:00:00Z"
	exportTo   = "2026-02-01T00:00:00Z"
)

func getExport(exp *fakeExporter, q url.Values) *httptest.ResponseRecorder {
	h := &handler.AuditHandler{Export: exp}
	target := "/api/v1/audit/export"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	rec := httptest.NewRecorder()
	h.ExportWindow(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func assertRefusedBeforeExport(t *testing.T, q url.Values) {
	t.Helper()
	exp := &fakeExporter{}
	rec := getExport(exp, q)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if exp.calls != 0 {
		t.Errorf("exporter called %d times, want 0", exp.calls)
	}
}

func TestAuditExportMissingFromIs400(t *testing.T) {
	assertRefusedBeforeExport(t, url.Values{"to": {exportTo}})
}

func TestAuditExportMissingToIs400(t *testing.T) {
	assertRefusedBeforeExport(t, url.Values{"from": {exportFrom}})
}

func TestAuditExportUnparseableFromIs400(t *testing.T) {
	assertRefusedBeforeExport(t, url.Values{"from": {"last tuesday"}, "to": {exportTo}})
}

func TestAuditExportUnparseableToIs400(t *testing.T) {
	assertRefusedBeforeExport(t, url.Values{"from": {exportFrom}, "to": {"2026-02-01"}})
}

// A refused window is the caller's mistake, and the body says which.
func TestAuditExportWindowErrorIs400WithTheMessage(t *testing.T) {
	err := fmt.Errorf("%w: from is not before to", audit.ErrWindow)
	rec := getExport(&fakeExporter{err: err}, url.Values{"from": {exportTo}, "to": {exportFrom}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), err.Error()) {
		t.Errorf("body = %q, want it to carry %q", rec.Body.String(), err.Error())
	}
}

func TestAuditExportOtherErrorIs500(t *testing.T) {
	rec := getExport(&fakeExporter{err: errors.New("database is closed")}, url.Values{"from": {exportFrom}, "to": {exportTo}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %q)", rec.Code, rec.Body.String())
	}
}

// The handler assembles nothing: the exporter's bytes are the body.
func TestAuditExportWritesTheExportersBytesVerbatim(t *testing.T) {
	exp := &fakeExporter{}
	before := time.Now()
	rec := getExport(exp, url.Values{"from": {exportFrom}, "to": {exportTo}})
	after := time.Now()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := rec.Body.String(); got != exportBytes {
		t.Errorf("body = %q, want %q byte for byte", got, exportBytes)
	}
	if exp.calls != 1 {
		t.Fatalf("exporter called %d times, want 1", exp.calls)
	}
	wantFrom, _ := time.Parse(time.RFC3339, exportFrom)
	wantTo, _ := time.Parse(time.RFC3339, exportTo)
	if !exp.from.Equal(wantFrom) || !exp.to.Equal(wantTo) {
		t.Errorf("exporter got [%s, %s), want [%s, %s)", exp.from, exp.to, wantFrom, wantTo)
	}
	if exp.now.Before(before) || exp.now.After(after) {
		t.Errorf("exporter got now = %s, want the time of the request", exp.now)
	}
}
