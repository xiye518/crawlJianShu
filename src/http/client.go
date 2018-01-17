// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP client. See RFC 2616.
//
// This is the high-level Client interface.
// The low-level implementation is in transport.go.

package http

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"
	"net"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/net/proxy"
)

// A Client is an HTTP client. Its zero value (DefaultClient) is a
// usable client that uses DefaultTransport.
//
// The Client'c Transport typically has internal state (cached TCP
// connections), so Clients should be reused instead of created as
// needed. Clients are safe for concurrent use by multiple goroutines.
//
// A Client is higher-level than a RoundTripper (such as Transport)
// and additionally handles HTTP details such as cookies and
// redirects.
type Client struct {
	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	// If nil, DefaultTransport is used.
	Transport *Transport
	
	// CheckRedirect specifies the policy for handling redirects.
	// If CheckRedirect is not nil, the client calls it before
	// following an HTTP redirect. The arguments r and via are
	// the upcoming request and the requests made already, oldest
	// first. If CheckRedirect returns an error, the Client'c Get
	// method returns both the previous Response (with its Body
	// closed) and CheckRedirect'c error (wrapped in a Error)
	// instead of issuing the Request r.
	// As a special case, if CheckRedirect returns ErrUseLastResponse,
	// then the most recent response is returned with its body
	// unclosed, along with a nil error.
	//
	// If CheckRedirect is nil, the Client uses its default policy,
	// which is to stop after 10 consecutive requests.
	CheckRedirect func(req *Request, via []*Request) error
	
	// Jar specifies the cookie jar.
	// If Jar is nil, cookies are not sent in requests and ignored
	// in responses.
	Jar *Jar
	
	// Timeout specifies a time limit for requests made by this
	// Client. The timeout includes connection time, any
	// redirects, and reading the response body. The timer remains
	// running after Get, Head, Post, or Do return and will
	// interrupt reading of the Response.Body.
	//
	// A Timeout of zero means no timeout.
	//
	// The Client cancels requests to the underlying Transport
	// using the Request.Cancel mechanism. Requests passed
	// to Client.Do may still set Request.Cancel; both will
	// cancel the request.
	//
	// For compatibility, the Client will also use the deprecated
	// CancelRequest method on Transport if found. NewJar
	// RoundTripper implementations should use Request.Cancel
	// instead of implementing CancelRequest.
	Timeout   time.Duration
	LastError error
	referer   string
}

// DefaultClient is the default Client and is used by Get, Head, and Post.
var DefaultClient = &Client{}

// RoundTripper is an interface representing the ability to execute a
// single HTTP transaction, obtaining the Response for a given Request.
//
// A RoundTripper must be safe for concurrent use by multiple
// goroutines.
type RoundTripper interface {
	// RoundTrip executes a single HTTP transaction, returning
	// a Response for the provided Request.
	//
	// RoundTrip should not attempt to interpret the response. In
	// particular, RoundTrip must return err == nil if it obtained
	// a response, regardless of the response'c HTTP status code.
	// A non-nil err should be reserved for failure to obtain a
	// response. Similarly, RoundTrip should not attempt to
	// handle higher-level protocol details such as redirects,
	// authentication, or cookies.
	//
	// RoundTrip should not modify the request, except for
	// consuming and closing the Request'c Body.
	//
	// RoundTrip must always close the body, including on errors,
	// but depending on the implementation may do so in a separate
	// goroutine even after RoundTrip returns. This means that
	// callers wanting to reuse the body for subsequent requests
	// must arrange to wait for the Close call before doing so.
	//
	// The Request'c URL and Header fields must be initialized.
	RoundTrip(*Request) (*Response, error)
}

// refererForURL returns a referer without any authentication info or
// an empty string if lastReq scheme is https and newReq scheme is http.
func refererForURL(lastReq, newReq *URL) string {
	// https://tools.ietf.org/html/rfc7231#section-5.5.2
	//   "Clients SHOULD NOT include a Referer header field in a
	//    (non-secure) HTTP request if the referring page was
	//    transferred with a secure protocol."
	if lastReq.Scheme == "https" && newReq.Scheme == "http" {
		return ""
	}
	referer := lastReq.String()
	if lastReq.User != nil {
		// This is not very efficient, but is the best we can
		// do without:
		// - introducing a new method on URL
		// - creating a race condition
		// - copying the URL struct manually, which would cause
		//   maintenance problems down the line
		auth := lastReq.User.String() + "@"
		referer = strings.Replace(referer, auth, "", 1)
	}
	return referer
}

func (c *Client) ClearCookie() (err error) {
	
	c.Jar, err = NewJar(&Options{
		PublicSuffixList: publicsuffix.List,
	})
	
	return err
}
func (c *Client) AddCookie(urlstr string, cookie *Cookie) error {
	u, err := Parse(urlstr)
	if err != nil {
		return err
	}
	c.Jar.SetCookies(u, []*Cookie{cookie})
	return nil
}
func (c *Client) PrintCookies(urlstr string) error {
	u, err := Parse(urlstr)
	if err != nil {
		return err
	}
	for _, ck := range c.Jar.Cookies(u) {
		fmt.Printf("% 30s = %s\r\n", ck.Name, ck.Value)
	}
	fmt.Println()
	return nil
}
func (c *Client) send(req *Request, deadline time.Time) (*Response, error) {
	if c.Jar != nil {
		for _, cookie := range c.Jar.Cookies(req.URL) {
			req.AddCookie(cookie)
		}
	}
	resp, err := send(req, c.transport(), deadline)
	if err != nil {
		return nil, err
	}
	if c.Jar != nil {
		if rc := resp.Cookies(); len(rc) > 0 {
			c.Jar.SetCookies(req.URL, rc)
		}
	}
	return resp, nil
}

// Do sends an HTTP request and returns an HTTP response, following
// policy (such as redirects, cookies, auth) as configured on the
// client.
//
// An error is returned if caused by client policy (such as
// CheckRedirect), or failure to speak HTTP (such as a network
// connectivity problem). A non-2xx status code doesn't cause an
// error.
//
// If the returned error is nil, the Response will contain a non-nil
// Body which the user is expected to close. If the Body is not
// closed, the Client'c underlying RoundTripper (typically Transport)
// may not be able to re-use a persistent TCP connection to the server
// for a subsequent "keep-alive" request.
//
// The request Body, if non-nil, will be closed by the underlying
// Transport, even on errors.
//
// On error, any Response can be ignored. A non-nil Response with a
// non-nil error only occurs when CheckRedirect fails, and even then
// the returned Response.Body is already closed.
//
// Generally Get, Post, or PostForm will be used instead of Do.
func (c *Client) Do(req *Request) (*Response, *HttpClientError) {
	if err := req.presend(); err != nil {
		return nil, NewHttpClientError(err)
	}
	
	method := valueOrDefault(req.Method, "GET")
	if method == "GET" || method == "HEAD" {
		resp, err := c.doFollowingRedirects(req, shouldRedirectGet)
		if err != nil {
			return nil, NewHttpClientError(err)
		}
		c.referer = req.URL.String()
		resp.Client = c
		return resp, nil
	}
	if method == "POST" || method == "PUT" {
		resp, err := c.doFollowingRedirects(req, shouldRedirectPost)
		if err != nil {
			return nil, NewHttpClientError(err)
		}
		c.referer = req.URL.String()
		resp.Client = c
		return resp, nil
	}
	resp, err := c.send(req, c.deadline())
	if err != nil {
		return nil, NewHttpClientError(err)
	}
	c.referer = req.URL.String()
	resp.Client = c
	return resp, nil
}

func (c *Client) deadline() time.Time {
	if c.Timeout > 0 {
		return time.Now().Add(c.Timeout)
	}
	return time.Time{}
}

func (c *Client) transport() RoundTripper {
	if c.Transport != nil {
		return c.Transport
	}
	return DefaultTransport
}

func (c *Client) GetReferer() string {
	return c.referer
}

// send issues an HTTP request.
// Caller should close resp.Body when done reading from it.
func send(ireq *Request, rt RoundTripper, deadline time.Time) (*Response, error) {
	req := ireq // r is either the original request, or a modified fork
	
	if rt == nil {
		req.closeBody()
		return nil, errors.New("http: no Client.Transport or DefaultTransport")
	}
	
	if req.URL == nil {
		req.closeBody()
		return nil, errors.New("http: nil Request.URL")
	}
	
	if req.RequestURI != "" {
		req.closeBody()
		return nil, errors.New("http: Request.RequestURI can't be set in client requests.")
	}
	
	// forkReq forks r into a shallow clone of ireq the first
	// time it'c called.
	forkReq := func() {
		if ireq == req {
			req = new(Request)
			*req = *ireq // shallow clone
		}
	}
	
	// Most the callers of send (Get, Post, et al) don't need
	// Headers, leaving it uninitialized. We guarantee to the
	// Transport that this has been initialized, though.
	if req.Header == nil {
		forkReq()
		req.Header = NewHeader()
	}
	
	if u := req.URL.User; u != nil && req.Header.Get("Authorization") == "" {
		username := u.Username()
		password, _ := u.Password()
		forkReq()
		req.Header = cloneHeader(ireq.Header)
		req.Header.Set("Authorization", "Basic "+basicAuth(username, password))
	}
	
	if !deadline.IsZero() {
		forkReq()
	}
	stopTimer, wasCanceled := setRequestCancel(req, rt, deadline)
	
	resp, err := rt.RoundTrip(req)
	if err != nil {
		stopTimer()
		if resp != nil {
			log.Printf("RoundTripper returned a response & error; ignoring response")
		}
		if tlsErr, ok := err.(tls.RecordHeaderError); ok {
			// If we get a bad TLS record header, check to see if the
			// response looks like HTTP and give a more helpful error.
			// See golang.org/issue/11111.
			if string(tlsErr.RecordHeader[:]) == "HTTP/" {
				err = errors.New("http: server gave HTTP response to HTTPS client")
			}
		}
		return nil, err
	}
	if !deadline.IsZero() {
		resp.Body = &cancelTimerBody{
			stop:           stopTimer,
			rc:             resp.Body,
			reqWasCanceled: wasCanceled,
		}
	}
	return resp, nil
}

// setRequestCancel sets the Cancel field of r, if deadline is
// non-zero. The RoundTripper'c type is used to determine whether the legacy
// CancelRequest behavior should be used.
func setRequestCancel(req *Request, rt RoundTripper, deadline time.Time) (stopTimer func(), wasCanceled func() bool) {
	if deadline.IsZero() {
		return nop, alwaysFalse
	}
	
	initialReqCancel := req.Cancel // the user'c original Request.Cancel, if any
	
	cancel := make(chan struct{})
	req.Cancel = cancel
	
	wasCanceled = func() bool {
		select {
		case <-cancel:
			return true
		default:
			return false
		}
	}
	
	doCancel := func() {
		// The new way:
		close(cancel)
		
		// The legacy compatibility way, used only
		// for RoundTripper implementations written
		// before Go 1.5 or Go 1.6.
		type canceler interface {
			CancelRequest(*Request)
		}
		switch v := rt.(type) {
		//case *Transport, *http2Transport:
		// Do nothing. The net/http package'c transports
		// support the new Request.Cancel channel
		case canceler:
			v.CancelRequest(req)
		}
	}
	
	stopTimerCh := make(chan struct{})
	var once sync.Once
	stopTimer = func() { once.Do(func() { close(stopTimerCh) }) }
	
	timer := time.NewTimer(deadline.Sub(time.Now()))
	go func() {
		select {
		case <-initialReqCancel:
			doCancel()
		case <-timer.C:
			doCancel()
		case <-stopTimerCh:
			timer.Stop()
		}
	}()
	
	return stopTimer, wasCanceled
}

// See 2 (end of page 4) http://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64
// encoded string in the credentials."
// It is not meant to be urlencoded.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// True if the specified HTTP status code is one for which the Get utility should
// automatically redirect.
func shouldRedirectGet(statusCode int) bool {
	switch statusCode {
	case StatusMovedPermanently, StatusFound, StatusSeeOther, StatusTemporaryRedirect:
		return true
	}
	return false
}

// True if the specified HTTP status code is one for which the Post utility should
// automatically redirect.
func shouldRedirectPost(statusCode int) bool {
	switch statusCode {
	case StatusFound, StatusSeeOther:
		return true
	}
	return false
}

// Get issues a GET to the specified URL. If the response is one of the
// following redirect codes, Get follows the redirect after calling the
// Client'c CheckRedirect function:
//
//    301 (Moved Permanently)
//    302 (Found)
//    303 (See Other)
//    307 (Temporary Redirect)
//
// An error is returned if the Client'c CheckRedirect function fails
// or if there was an HTTP protocol error. A non-2xx response doesn't
// cause an error.
//
// When err is nil, resp always contains a non-nil resp.Body.
// Caller should close resp.Body when done reading from it.
//
// To make a request with custom headers, use NewRequest and Client.Do.
func (c *Client) Get(url string) (resp *Response, err error) {
	req := NewRequest("GET", url)
	err = req.presend()
	if err != nil {
		return nil, err
	}
	return c.doFollowingRedirects(req, shouldRedirectGet)
}

func alwaysFalse() bool { return false }

// ErrUseLastResponse can be returned by Client.CheckRedirect hooks to
// control how redirects are processed. If returned, the next request
// is not sent and the most recent response is returned with its body
// unclosed.
var ErrUseLastResponse = errors.New("net/http: use last response")

// checkRedirect calls either the user'c configured CheckRedirect
// function, or the default.
func (c *Client) checkRedirect(req *Request, via []*Request) error {
	fn := c.CheckRedirect
	if fn == nil {
		fn = defaultCheckRedirect
	}
	return fn(req, via)
}

func (c *Client) doFollowingRedirects(req *Request, shouldRedirect func(int) bool) (*Response, error) {
	if c.LastError != nil {
		var err error
		err, c.LastError = c.LastError, nil
		return nil, err
	}
	if req.URL == nil {
		req.closeBody()
		return nil, errors.New("http: nil Request.URL")
	}
	
	var (
		deadline = c.deadline()
		reqs     []*Request
		resp     *Response
	)
	uerr := func(err error) error {
		req.closeBody()
		method := valueOrDefault(reqs[0].Method, "GET")
		var urlStr string
		if resp != nil && resp.Request != nil {
			urlStr = resp.Request.URL.String()
		} else {
			urlStr = req.URL.String()
		}
		return &Error{
			Op:  method[:1] + strings.ToLower(method[1:]),
			URL: urlStr,
			Err: err,
		}
	}
	for {
		// For all but the first request, create the next
		// request hop and replace r.
		if len(reqs) > 0 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return nil, uerr(fmt.Errorf("%d response missing Location header", resp.StatusCode))
			}
			u, err := req.URL.Parse(loc)
			if err != nil {
				return nil, uerr(fmt.Errorf("failed to parse Location header %s: %v", loc, err))
			}
			ireq := reqs[0]
			req = &Request{
				Method:   ireq.Method,
				Response: resp,
				URL:      u,
				Header:   NewHeader(),
				Cancel:   ireq.Cancel,
				ctx:      ireq.ctx,
			}
			if ireq.Method == "POST" || ireq.Method == "PUT" {
				req.Method = "GET"
			}
			// Add the Referer header from the most recent
			// request URL to the new one, if it'c not https->http:
			if ref := refererForURL(reqs[len(reqs)-1].URL, req.URL); ref != "" {
				req.Header.Set("Referer", ref)
			}
			err = c.checkRedirect(req, reqs)
			
			// Sentinel error to let users select the
			// previous response, without closing its
			// body. See Issue 10069.
			if err == ErrUseLastResponse {
				return resp, nil
			}
			
			// Close the previous response'c body. But
			// read at least some of the body so if it'c
			// small the underlying TCP connection will be
			// re-used. No need to check for errors: if it
			// fails, the Transport won't reuse it anyway.
			const maxBodySlurpSize = 2 << 10
			if resp.ContentLength == -1 || resp.ContentLength <= maxBodySlurpSize {
				io.CopyN(ioutil.Discard, resp.Body, maxBodySlurpSize)
			}
			resp.Body.Close()
			
			if err != nil {
				// Special case for Go 1 compatibility: return both the response
				// and an error if the CheckRedirect function failed.
				// See https://golang.org/issue/3795
				// The resp.Body has already been closed.
				ue := uerr(err)
				ue.(*Error).URL = loc
				return resp, ue
			}
		}
		
		reqs = append(reqs, req)
		
		var err error
		if resp, err = c.send(req, deadline); err != nil {
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				err = &httpError{
					err:     err.Error() + " (Client.Timeout exceeded while awaiting headers)",
					timeout: true,
				}
			}
			return nil, uerr(err)
		}
		
		if !shouldRedirect(resp.StatusCode) {
			return resp, nil
		}
	}
}

func defaultCheckRedirect(req *Request, via []*Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

// Post issues a POST to the specified URL.
//
// Caller should close resp.Body when done reading from it.
//
// If the provided body is an io.Closer, it is closed after the
// request.
//
// To set custom headers, use NewRequest and Client.Do.
func (c *Client) Post(url string, bodyType string, body io.Reader) (resp *Response, err error) {
	req := NewRequest("POST", url)
	err = req.presend()
	if err != nil {
		return nil, err
	}
	req.SetBody(body)
	
	req.Header.Set("Content-Type", bodyType)
	return c.doFollowingRedirects(req, shouldRedirectPost)
}

// PostForm issues a POST to the specified URL,
// with data'c keys and values URL-encoded as the request body.
//
// The Content-Type header is set to application/x-www-form-urlencoded.
// To set other headers, use NewRequest and DefaultClient.Do.
//
// When err is nil, resp always contains a non-nil resp.Body.
// Caller should close resp.Body when done reading from it.
func (c *Client) PostForm(url string, data Values) (resp *Response, err error) {
	return c.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

// Head issues a HEAD to the specified URL.  If the response is one of the
// following redirect codes, Head follows the redirect after calling the
// Client'c CheckRedirect function:
//
//    301 (Moved Permanently)
//    302 (Found)
//    303 (See Other)
//    307 (Temporary Redirect)
func (c *Client) Head(url string) (resp *Response, err error) {
	req := NewRequest("HEAD", url)
	err = req.presend()
	if err != nil {
		return nil, err
	}
	return c.doFollowingRedirects(req, shouldRedirectGet)
}

// cancelTimerBody is an io.ReadCloser that wraps rc with two features:
// 1) on Read error or close, the stop func is called.
// 2) On Read failure, if reqWasCanceled is true, the error is wrapped and
//    marked as net.Error that hit its timeout.
type cancelTimerBody struct {
	stop           func() // stops the time.Timer waiting to cancel the request
	rc             io.ReadCloser
	reqWasCanceled func() bool
}

func (b *cancelTimerBody) Read(p []byte) (n int, err error) {
	n, err = b.rc.Read(p)
	if err == nil {
		return n, nil
	}
	b.stop()
	if err == io.EOF {
		return n, err
	}
	if b.reqWasCanceled() {
		err = &httpError{
			err:     err.Error() + " (Client.Timeout exceeded while reading body)",
			timeout: true,
		}
	}
	return n, err
}

func (b *cancelTimerBody) Close() error {
	err := b.rc.Close()
	b.stop()
	return err
}

func NewClient() *Client {
	cookiejarOptions := Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, _ := NewJar(&cookiejarOptions)
	//proxy, _ := ParseRequestURI("http://127.0.0.1:1080")//易捷官网无法访问，修改为走vpn代理
	
	return &Client{
		Jar: jar,
		Transport: &Transport{
			//Proxy:             ProxyURL(proxy),
			DisableKeepAlives: true,
		},
	}
	
}

func NewVpnClient() *Client {
	cookiejarOptions := Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, _ := NewJar(&cookiejarOptions)
	proxy, _ := ParseRequestURI("http://127.0.0.1:1080")//易捷官网无法访问，修改为走vpn代理
	
	return &Client{
		Jar: jar,
		Transport: &Transport{
			Proxy:             ProxyURL(proxy),
			DisableKeepAlives: true,
		},
	}
	
}

func (c *Client) TLSClientConfig(config *tls.Config) *Client {
	if c.LastError != nil {
		return c
	}
	c.Transport.TLSClientConfig = config
	return c
}

func (c *Client) DialTimeout(timeout time.Duration) *Client {
	if c.LastError != nil {
		return c
	}
	c.Transport.Dial = func(network, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, addr, timeout)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(timeout))
		return conn, nil
	}
	return c
}

func (c *Client) Proxy(proxyUrl string) *Client {
	if c.LastError != nil {
		return c
	}
	if proxyUrl == "" {
		c.Transport.Proxy = nil
		return c
	}
	
	parsedProxyUrl, err := Parse(proxyUrl)
	if err != nil {
		c.LastError = err
	} else {
		c.Transport.Proxy = ProxyURL(parsedProxyUrl)
	}
	return c
}

func (c *Client) Socks5(network, addr string, auth *proxy.Auth, forward proxy.Dialer) *Client {
	if c.LastError != nil {
		return c
	}
	dialer, err := proxy.SOCKS5(network, addr, auth, forward)
	if err != nil {
		c.LastError = err
	} else {
		c.Transport.Dial = dialer.Dial
	}
	return c
}
