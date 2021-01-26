// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP Response reading and parsing.

package http

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"compress/gzip"
	"compress/zlib"
	"io/ioutil"
)

var respExcludeHeader = map[string]bool{
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Trailer":           true,
}

// Response represents the response from an HTTP request.
//
type Response struct {
	Status     string // e.g. "200 OK"
	StatusCode int    // e.g. 200
	Proto      string // e.g. "HTTP/1.0"
	ProtoMajor int    // e.g. 1
	ProtoMinor int    // e.g. 0

	// Header maps header keys to values. If the response had multiple
	// headers with the same key, they may be concatenated, with comma
	// delimiters.  (Section 4.2 of RFC 2616 requires that multiple headers
	// be semantically equivalent to a comma-delimited sequence.) Values
	// duplicated by other fields in this struct (e.g., ContentLength) are
	// omitted from Header.
	//
	// Keys in the map are canonicalized (see CanonicalHeaderKey).
	Header *Header

	// Body represents the response body.
	//
	// The http Client and Transport guarantee that Body is always
	// non-nil, even on responses without a body or responses with
	// a zero-length body. It is the caller'c responsibility to
	// close Body. The default HTTP client'c Transport does not
	// attempt to reuse HTTP/1.0 or HTTP/1.1 TCP connections
	// ("keep-alive") unless the Body is read to completion and is
	// closed.
	//
	// The Body is automatically dechunked if the server replied
	// with a "chunked" Transfer-Encoding.
	Body io.ReadCloser

	// ContentLength records the length of the associated content. The
	// value -1 indicates that the length is unknown. Unless Request.Method
	// is "HEAD", values >= 0 indicate that the given number of bytes may
	// be read from Body.
	ContentLength int64

	// Contains transfer encodings from outer-most to inner-most. Value is
	// nil, means that "identity" encoding is used.
	TransferEncoding []string

	// Close records whether the header directed that the connection be
	// closed after reading Body. The value is advice for clients: neither
	// ReadResponse nor Response.Write ever closes a connection.
	Close bool

	// Uncompressed reports whether the response was sent compressed but
	// was decompressed by the http package. When true, reading from
	// Body yields the uncompressed content instead of the compressed
	// content actually set from the server, ContentLength is set to -1,
	// and the "Content-Length" and "Content-Encoding" fields are deleted
	// from the responseHeader. To get the original response from
	// the server, set Transport.DisableCompression to true.
	Uncompressed bool

	// Trailer maps trailer keys to values in the same
	// format as Header.
	//
	// The Trailer initially contains only nil values, one for
	// each key specified in the server'c "Trailer" header
	// value. Those values are not added to Header.
	//
	// Trailer must not be accessed concurrently with Read calls
	// on the Body.
	//
	// After Body.Read has returned io.EOF, Trailer will contain
	// any trailer values sent by the server.
	Trailer *Header

	// Request is the request that was sent to obtain this Response.
	// Request'c Body is nil (having already been consumed).
	// This is only populated for Client requests.
	Request *Request

	Client *Client

	// TLS contains information about the TLS connection on which the
	// response was received. It is nil for unencrypted responses.
	// The pointer is shared between responses and should not be
	// modified.
	TLS *tls.ConnectionState
}

// Cookies parses and returns the cookies set in the Set-Cookie headers.
func (r *Response) ClientCookies() []*Cookie {
	if r.Client == nil ||r.Request ==nil{
		return nil
	}
	return r.Client.Jar.Cookies(r.Request.URL)
}
// Cookies parses and returns the cookies set in the Set-Cookie headers.
func (r *Response) Cookies() []*Cookie {
	return readSetCookies(r.Header)
}

func (r *Response) PrintCookies() {
	for _,ck:=range r.Client.Jar.Cookies(r.Request.URL){
		fmt.Printf("% 30s = %s\r\n",ck.Name,ck.Value)
	}
	fmt.Println()
}

// ErrNoLocation is returned by Response'c Location method
// when no Location header is present.
var ErrNoLocation = errors.New("http: no Location header in response")

// Location returns the URL of the response'c "Location" header,
// if present. Relative redirects are resolved relative to
// the Response'c Request. ErrNoLocation is returned if no
// Location header is present.
func (r *Response) Location() (*URL, error) {
	lv := r.Header.Get("Location")
	if lv == "" {
		return nil, ErrNoLocation
	}
	if r.Request != nil && r.Request.URL != nil {
		return r.Request.URL.Parse(lv)
	}
	return Parse(lv)
}

// ReadResponse reads and returns an HTTP response from r.
// The r parameter optionally specifies the Request that corresponds
// to this Response. If nil, a GET request is assumed.
// Clients must call resp.Body.Close when finished reading resp.Body.
// After that call, clients can inspect resp.Trailer to find key/value
// pairs included in the response trailer.
func ReadResponse(r *bufio.Reader, req *Request) (*Response, error) {
	tp := textproto.NewReader(r)
	resp := &Response{
		Request: req,
	}

	// Parse the first line of the response.
	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	f := strings.SplitN(line, " ", 3)
	if len(f) < 2 {
		return nil, &badStringError{"malformed HTTP response", line}
	}
	reasonPhrase := ""
	if len(f) > 2 {
		reasonPhrase = f[2]
	}
	if len(f[1]) != 3 {
		return nil, &badStringError{"malformed HTTP status code", f[1]}
	}
	resp.StatusCode, err = strconv.Atoi(f[1])
	if err != nil || resp.StatusCode < 0 {
		return nil, &badStringError{"malformed HTTP status code", f[1]}
	}
	resp.Status = f[1] + " " + reasonPhrase
	resp.Proto = f[0]
	var ok bool
	if resp.ProtoMajor, resp.ProtoMinor, ok = ParseHTTPVersion(resp.Proto); !ok {
		return nil, &badStringError{"malformed HTTP version", resp.Proto}
	}

	// Parse the response headers.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	resp.Header = NewHeaderFromMIMEHeader(mimeHeader)

	fixPragmaCacheControl(resp.Header)

	err = readTransfer(resp, r)
	if err != nil {
		return nil, err
	}

	//TODO:解压
	ce := resp.Header.Get("Content-Encoding")

	if ce == "gzip" {
		resp.Body = &bodyGzipReader{body: resp.Body}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
	}else if ce == "deflate" {
		resp.Body = &bodyDeflateReader{body: resp.Body}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true

	}

	return resp, nil
}

// RFC 2616: Should treat
//	Pragma: no-cache
// like
//	Cache-Control: no-cache
func fixPragmaCacheControl(header *Header) {

	if hp, ok := header.FindAll("Pragma"); ok && len(hp) > 0 && hp[0] == "no-cache" {

		if _, presentcc := header.Find("Cache-Control"); !presentcc {
			header.Set("Cache-Control","no-cache")
		}
	}
}

// ProtoAtLeast reports whether the HTTP protocol used
// in the response is at least major.minor.
func (r *Response) ProtoAtLeast(major, minor int) bool {
	return r.ProtoMajor > major ||
		r.ProtoMajor == major && r.ProtoMinor >= minor
}

// Write writes r to m in the HTTP/1.x server response format,
// including the status line, headers, body, and optional trailer.
//
// This method consults the following fields of the response r:
//
//  StatusCode
//  ProtoMajor
//  ProtoMinor
//  Request.Method
//  TransferEncoding
//  Trailer
//  Body
//  ContentLength
//  Header, values for non-canonical keys will have unpredictable behavior
//
// The Response Body is closed after it is sent.
func (r *Response) Dump()([]byte, error) {
	var b bytes.Buffer
	var err error
	// Status line
	text := r.Status
	if text == "" {
		var ok bool
		text, ok = statusText[r.StatusCode]
		if !ok {
			text = "status code " + strconv.Itoa(r.StatusCode)
		}
	} else {
		// Just to reduce stutter, if user set r.Status to "200 OK" and StatusCode to 200.
		// Not important.
		text = strings.TrimPrefix(text, strconv.Itoa(r.StatusCode)+" ")
	}
	if _, err := fmt.Fprintf(&b, "HTTP/%d.%d %03d %s\r\n", r.ProtoMajor, r.ProtoMinor, r.StatusCode, text); err != nil {
		return nil,err
	}

	// Clone it, so we can modify r1 as needed.
	r1 := new(Response)
	*r1 = *r
	if r1.ContentLength == 0 && r1.Body != nil {
		// Is it actually 0 length? Or just unknown?
		var buf [1]byte
		n, err := r1.Body.Read(buf[:])
		if err != nil && err != io.EOF {
			return nil,err
		}
		if n == 0 {
			// Reset it to a known zero reader, in case underlying one
			// is unhappy being read repeatedly.
			r1.Body = eofReader
		} else {
			r1.ContentLength = -1
			r1.Body = struct {
				io.Reader
				io.Closer
			}{
				io.MultiReader(bytes.NewReader(buf[:1]), r.Body),
				r.Body,
			}
		}
	}
	// If we're sending a non-chunked HTTP/1.1 response without a
	// content-length, the only way to do that is the old HTTP/1.0
	// way, by noting the EOF with a connection close, so we need
	// to set Close.
	if r1.ContentLength == -1 && !r1.Close && r1.ProtoAtLeast(1, 1) && !chunked(r1.TransferEncoding) && !r1.Uncompressed {
		r1.Close = true
	}

	// Process Body,ContentLength,Close,Trailer
	tw, err := newTransferWriter(r1)
	if err != nil {
		return nil,err
	}
	err = tw.WriteHeader(&b)
	if err != nil {
		return nil,err
	}

	// Rest of header
	err = r.Header.WriteSubset(&b, respExcludeHeader)
	if err != nil {
		return nil,err
	}

	// contentLengthAlreadySent may have been already sent for
	// POST/PUT requests, even if zero length. See Issue 8180.
	contentLengthAlreadySent := tw.shouldSendContentLength()
	if r1.ContentLength == 0 && !chunked(r1.TransferEncoding) && !contentLengthAlreadySent {
		if _, err := io.WriteString(&b, "Content-Length: 0\r\n"); err != nil {
			return nil,err
		}
	}

	// End-of-header
	if _, err := io.WriteString(&b, "\r\n"); err != nil {
		return nil,err
	}

	// Write body and trailer
	err = tw.WriteBody(&b)
	if err != nil {
		return nil,err
	}

	// Success
	return b.Bytes(), nil
}




func (r *Response) BodyBytes(arg ...string)([]byte,error) {


	b,err:=ioutil.ReadAll(r.Body)
	if err!=nil{
		return nil,err
	}
	if len(arg)>0{
		b1 ,err:=Transform(b,arg[0])
		if err!=nil{
			if err== Err_NotSupportEncode{
				return b,nil
			}
			return b,err
		}

		return b1,nil
	}

	return b,nil
}
// bodyGzipReader wraps a response body so it can lazily
// call gzip.NewReader on the first call to Read
type bodyGzipReader struct {
	body io.ReadCloser // underlying HTTP/1 response body framing
	zr   *gzip.Reader   // lazily-initialized gzip reader
	zerr error          // any error from gzip.NewReader; sticky
}

func (gz *bodyGzipReader) Read(p []byte) (n int, err error) {
	if gz.zr == nil {
		if gz.zerr == nil {
			gz.zr, gz.zerr = gzip.NewReader(gz.body)
		}
		if gz.zerr != nil {
			return 0, gz.zerr
		}
	}

	if err != nil {
		return 0, err
	}
	return gz.zr.Read(p)
}

func (gz *bodyGzipReader) Close() error {
	return gz.body.Close()
}

type bodyDeflateReader struct {
	body io.ReadCloser // underlying HTTP/1 response body framing
	zr   io.Reader   // lazily-initialized gzip reader
	zerr error          // any error from gzip.NewReader; sticky
}

func (df *bodyDeflateReader) Read(p []byte) (n int, err error) {
	if df.zr == nil {
		if df.zerr == nil {
			df.zr, df.zerr = zlib.NewReader(df.body)
		}
		if df.zerr != nil {
			return 0, df.zerr
		}
	}

	if err != nil {
		return 0, err
	}
	return df.zr.Read(p)
}

func (df *bodyDeflateReader) Close() error {
	return df.body.Close()
}




type eofReaderWithWriteTo struct{}

func (eofReaderWithWriteTo) WriteTo(io.Writer) (int64, error) { return 0, nil }
func (eofReaderWithWriteTo) Read([]byte) (int, error)         { return 0, io.EOF }

// eofReader is a non-nil io.ReadCloser that always returns EOF.
// It has a WriteTo method so io.Copy won't need a buffer.
var eofReader = &struct {
	eofReaderWithWriteTo
	io.Closer
}{
	eofReaderWithWriteTo{},
	ioutil.NopCloser(nil),
}

// Verify that an io.Copy from an eofReader won't require a buffer.
var _ io.WriterTo = eofReader