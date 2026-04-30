package forwarder

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"syscall"
	"time"
)

type onFailureFunc = func(serviceName, failedNodePath string, shouldDeleteFile bool)

type Forwarder struct {
	transports sync.Map // map[string]*http.Transport
	onFailure  onFailureFunc
}

func NewForwarder(onFailure onFailureFunc) *Forwarder {
	return &Forwarder{
		onFailure: onFailure,
	}
}

func (f *Forwarder) ForwardMiddleware(w http.ResponseWriter, r *http.Request, mwPath, nodePath string) bool {
	transport := f.getTransport(nodePath)
	client := &http.Client{
		Transport: transport,
		Timeout:   1 * time.Second,
	}

	if len(mwPath) == 0 || mwPath[0] != '/' {
		mwPath = "/" + mwPath
	}
	mwReq, err := http.NewRequestWithContext(r.Context(), "GET", "http://localhost"+mwPath, nil)
	if err != nil {
		http.Error(w, "Middleware Init Error", http.StatusInternalServerError)
		return false
	}

	for k, vv := range r.Header {
		for _, v := range vv {
			mwReq.Header.Add(k, v)
		}
	}

	mwReq.Host = r.Host

	mwReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	mwReq.Header.Set("X-Forwarded-Host", r.Host)
	mwReq.Header.Set("X-Proxy-Agent", "Go-Forwarder")

	resp, err := client.Do(mwReq)
	if err != nil {
		shouldDelete := false
		var opErr *net.OpError
		if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			shouldDelete = true
		}

		if f.onFailure != nil {
			f.onFailure("middleware", nodePath, shouldDelete)
			f.transports.Delete(nodePath)
		}

		http.Error(w, "Middleware Service Unavailable", http.StatusServiceUnavailable)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return false
	}

	for k, vv := range resp.Header {
		if len(vv) > 0 {
			r.Header.Set(k, vv[0])
		}
	}

	return true
}

func (f *Forwarder) Forward(w http.ResponseWriter, r *http.Request, serviceName, nodePath string) {

	target, _ := url.Parse("http://nautilus-internal")
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = f.getTransport(nodePath)

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		if req.Header.Get("X-Forwarded-Host") == "" {
			req.Header.Set("X-Forwarded-Host", r.Host)
		}
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("forwarding error: service=%s node=%s err=%v", serviceName, nodePath, err)
		shouldDelete := false
		var opErr *net.OpError

		if errors.As(err, &opErr) {
			if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
				shouldDelete = true
			}
		}

		if f.onFailure != nil {
			f.onFailure(serviceName, nodePath, shouldDelete)
			f.transports.Delete(nodePath)
		}

		http.Error(w, "Node Failure", http.StatusBadGateway)
	}

	rp.ServeHTTP(w, r)
}

func (f *Forwarder) getTransport(socketPath string) *http.Transport {
	if val, ok := f.transports.Load(socketPath); ok {
		return val.(*http.Transport)
	}

	t := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		MaxIdleConnsPerHost: 100,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	}

	actual, _ := f.transports.LoadOrStore(socketPath, t)
	return actual.(*http.Transport)
}
