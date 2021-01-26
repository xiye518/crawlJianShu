package http

import (
	"fmt"
	"errors"
	"net"
	"strings"
)

type HttpClientError struct {
	err error
}

func NewHttpClientError(err interface{})*HttpClientError{
	switch v := err.(type) {
	case string:
		return &HttpClientError{errors.New(v)}
	case fmt.Stringer:
		return &HttpClientError{errors.New(v.String())}
	case fmt.GoStringer:
		return &HttpClientError{errors.New(v.GoString())}
	case error:
		return &HttpClientError{v}
	default:
		return &HttpClientError{fmt.Errorf("%v",v)}
	}
}
func (e *HttpClientError)Error()string{
	return e.err.Error()
}
func (e *HttpClientError)IsDnsError()bool{
	if _, ok :=e.err.(*net.DNSError);ok{
		return true
	}
	return false
}
func (e *HttpClientError)IsDialFailed()bool{
	return strings.Contains(e.Error(),"dial tcp")

}