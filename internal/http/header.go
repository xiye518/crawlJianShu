// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"io"
	"net/textproto"
	"strings"
	"time"
	"container/list"
)

type KeyValue struct {
	Key    string
	Value  string
}
// A Header represents the key-value pairs in an HTTP header.
type Header struct {
	List *list.List
}
func NewHeader()*Header{
	return &Header{
		List:list.New(),
	}
}
func NewHeaderFromMIMEHeader(mh textproto.MIMEHeader)*Header{
	h:= &Header{
		List:list.New(),
	}
	for k,vs := range mh{
		for _,v :=range vs{
			h.Add(k,v)
		}
	}
	return h
}

// Add adds the key, value pair to the header.
// It appends to any existing values associated with key.
func (h *Header) Len()int {
	return h.List.Len()
}
// Add adds the key, value pair to the header.
// It appends to any existing values associated with key.
func (h *Header) Add(key, value string) {
	h.List.PushBack(&KeyValue{key,value})
}

// Set sets the header entries associated with key to
// the single element value. It replaces any existing
// values associated with key.
func (h *Header) Set(key, value string) {
	done:=false
	var next *list.Element
	for e := h.List.Front(); e != nil; {
		kv := e.Value.(*KeyValue)
		if kv.Key == key {
			if done{
				next = e.Next()
				h.List.Remove(e)
				e = next
			}else{
				kv.Value = value
				done=true
				e = e.Next()
			}
		} else {
			e = e.Next()
		}
	}

	if !done{
		h.Add(key,value)
	}
}
func (h *Header) FindAll(key string) ([]string,bool) {
	rst := []string{}
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		if kv.Key == key{
			rst =append(rst,kv.Value)
		}
	}
	if len(rst)>0{
		return rst,true
	}
	return nil,false
}
func (h *Header) Find(key string) (string,bool) {
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		if kv.Key == key{
			return kv.Value,true
		}
	}
	return "",false
}
// Get gets the first value associated with the given key.
// If there are no values associated with the key, Get returns "".
// To access multiple values of a key, access the map directly
// with CanonicalHeaderKey.
func (h *Header) Get(key string) string {
	return h.get(key)
}

// get is like Get, but key must already be in CanonicalHeaderKey form.
func (h *Header) get(key string) string {
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		if kv.Key == key{
			return kv.Value
		}
	}
	return ""
}

// Del deletes the values associated with key.
func (h *Header) Del(key string) {

	var next *list.Element
	for e := h.List.Front(); e != nil; {
		if e.Value.(*KeyValue).Key == key {
			next = e.Next()
			h.List.Remove(e)
			e = next
		} else {
			e = e.Next()
		}
	}

}

// Write writes a header in wire format.
func (h *Header) Write(w io.Writer) error {
	return h.WriteSubset(w, nil)
}

func (h *Header) clone() *Header {

	h2 := NewHeader()
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		h2.Add(kv.Key,kv.Value)
	}
	return h2
}
const TimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

var timeFormats = []string{
	TimeFormat,
	time.RFC850,
	time.ANSIC,
}

// ParseTime parses a time header (such as the Date: header),
// trying each of the three formats allowed by HTTP/1.1:
// TimeFormat, time.RFC850, and time.ANSIC.
func ParseTime(text string) (t time.Time, err error) {
	for _, layout := range timeFormats {
		t, err = time.Parse(layout, text)
		if err == nil {
			return
		}
	}
	return
}

var headerNewlineToSpace = strings.NewReplacer("\n", " ", "\r", " ")

type writeStringer interface {
	WriteString(string) (int, error)
}

// stringWriter implements WriteString on a Writer.
type stringWriter struct {
	w io.Writer
}

func (w stringWriter) WriteString(s string) (n int, err error) {
	return w.w.Write([]byte(s))
}
//
//type keyValues struct {
//	key    string
//	values []string
//}
//
//// A headerSorter implements sort.Interface by sorting a []keyValues
//// by key. It'c used as a pointer, so it can fit in a sort.Interface
//// interface value without allocation.
//type headerSorter struct {
//	kvs []keyValues
//}
//
//func (c *headerSorter) Len() int           { return len(c.kvs) }
//func (c *headerSorter) Swap(i, j int)      { c.kvs[i], c.kvs[j] = c.kvs[j], c.kvs[i] }
//func (c *headerSorter) Less(i, j int) bool { return c.kvs[i].key < c.kvs[j].key }
//
//var headerSorterPool = sync.Pool{
//	NewJar: func() interface{} { return new(headerSorter) },
//}
//
//// sortedKeyValues returns h'c keys sorted in the returned kvs
//// slice. The headerSorter used to sort is also returned, for possible
//// return to headerSorterCache.
//func (h *Header) sortedKeyValues(exclude map[string]bool) (kvs []keyValues, hs *headerSorter) {
//	hs = headerSorterPool.Get().(*headerSorter)
//	if cap(hs.kvs) < len(h) {
//		hs.kvs = make([]keyValues, 0, len(h))
//	}
//	kvs = hs.kvs[:0]
//	for k, vv := range h {
//		if !exclude[k] {
//			kvs = append(kvs, keyValues{k, vv})
//		}
//	}
//	hs.kvs = kvs
//	sort.Sort(hs)
//	return kvs, hs
//}

// WriteSubset writes a header in wire format.
// If exclude is not nil, keys where exclude[key] == true are not written.
func (h *Header) WriteSubset(w io.Writer, exclude map[string]bool) error {
	ws, ok := w.(writeStringer)
	if !ok {
		ws = stringWriter{w}
	}
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		if !exclude[kv.Key] {
			v := headerNewlineToSpace.Replace(kv.Value)
			v = textproto.TrimString(v)
			for _, s := range []string{kv.Key, ": ", v, "\r\n"} {
				if _, err := ws.WriteString(s); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// CanonicalHeaderKey returns the canonical format of the
// header key c. The canonicalization converts the first
// letter and any letter following a hyphen to upper case;
// the rest are converted to lowercase. For example, the
// canonical key for "accept-encoding" is "Accept-Encoding".
// If c contains a space or invalid header field bytes, it is
// returned without modifications.
func CanonicalHeaderKey(s string) string { return textproto.CanonicalMIMEHeaderKey(s) }

// hasToken reports whether token appears with v, ASCII
// case-insensitive, with space or comma boundaries.
// token must be all lowercase.
// v may contain mixed cased.
func hasToken(v, token string) bool {
	if len(token) > len(v) || token == "" {
		return false
	}
	if v == token {
		return true
	}
	for sp := 0; sp <= len(v)-len(token); sp++ {
		// Check that first character is good.
		// The token is ASCII, so checking only a single byte
		// is sufficient. We skip this potential starting
		// position if both the first byte and its potential
		// ASCII uppercase equivalent (b|0x20) don't match.
		// False positives ('^' => '~') are caught by EqualFold.
		if b := v[sp]; b != token[0] && b|0x20 != token[0] {
			continue
		}
		// Check that start pos is on a valid token boundary.
		if sp > 0 && !isTokenBoundary(v[sp-1]) {
			continue
		}
		// Check that end pos is on a valid token boundary.
		if endPos := sp + len(token); endPos != len(v) && !isTokenBoundary(v[endPos]) {
			continue
		}
		if strings.EqualFold(v[sp:sp+len(token)], token) {
			return true
		}
	}
	return false
}

func isTokenBoundary(b byte) bool {
	return b == ' ' || b == ',' || b == '\t'
}

func cloneHeader(h *Header) *Header {
	h2 := NewHeader()
	for e := h.List.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*KeyValue)
		h2.Add(kv.Key,kv.Value)
	}
	return h2

}
