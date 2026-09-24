package client

import (
	"io"
	"net/http"
	"time"
)

const idempotencyKeyHeader = "Idempotency-Key"

type retrying struct {
	next    *http.Client
	retries int
	backoff time.Duration
}

func (r *retrying) Do(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := r.next.Do(req)

		if attempt >= r.retries || !safeToRepeat(req) || !transient(resp, err) {
			return resp, err
		}

		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		if err := rewind(req); err != nil {
			return nil, err
		}

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(r.backoff << attempt):
		}
	}
}

func safeToRepeat(req *http.Request) bool {
	return req.Method == http.MethodGet || req.Header.Get(idempotencyKeyHeader) != ""
}

func transient(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func rewind(req *http.Request) error {
	if req.GetBody == nil {
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return err
	}

	req.Body = body

	return nil
}
