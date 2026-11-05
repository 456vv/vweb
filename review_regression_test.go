package vweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReviewSessionReturnsActualNilInterface(t *testing.T) {
	ss := new(Sessions)
	rr := httptest.NewRecorder()

	if got := ss.Session(rr, nil); got != nil {
		t.Fatal("disabled session must return a nil interface")
	}
}

func TestReviewRouteRegexMatchesDirectory(t *testing.T) {
	rt := new(Route)
	rt.HandleFunc("^/items/.*/$", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		_, _ = w.Write([]byte("matched"))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items/one/", nil)
	rt.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "matched" {
		t.Fatalf(
			"status=%d body=%q",
			rr.Code,
			rr.Body.String(),
		)
	}
}

func TestReviewPlaceholderRepeatedSuffix(t *testing.T) {
	parts, ok := parsePlaceholderSegment("{name}.json")
	if !ok {
		t.Fatal("failed to parse placeholder")
	}

	params, ok := extractParamsFromSegmentPreparsed(
		parts,
		"a.json.json",
	)
	if !ok {
		t.Fatal("repeated suffix must match")
	}
	if got := params["name"]; got != "a.json" {
		t.Fatalf("name=%q, want a.json", got)
	}
}
