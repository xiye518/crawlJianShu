package http

import (
	"fmt"
	"crypto/rand"
	"errors"
	"bytes"
	"strings"
	"io/ioutil"
	"os"
	"path/filepath"
)




type Part struct {
	Filename  string
	Fieldname string
	ContentType string
	Data      []byte
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

func (p *Part)WriteToBuffer(buf *bytes.Buffer)error{
	var err error
	if p.Filename ==""{
		_,err=fmt.Fprintf(buf, "Content-Disposition: form-data; name=\"%s\"\r\n",
			escapeQuotes(p.Fieldname))
		if err!=nil{
			return err
		}
	}else{
		p.ContentType = "application/octet-stream"
		_,err=fmt.Fprintf(buf, "Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n",
			escapeQuotes(p.Fieldname),escapeQuotes(p.Filename))
		if err!=nil{
			return err
		}
	}
	if p.ContentType!=""{
		_,err=fmt.Fprintf(buf, "Content-Type: %s\r\n", escapeQuotes(p.ContentType))
		if err!=nil{
			return err
		}
	}

	_,err=buf.Write(p.Data)
	if err!=nil{
		return err
	}
	return nil
}


type MultiPart struct {
	boundary string
	buf      *bytes.Buffer
	parts    []*Part
	partserr error//记录添加part时发生的错误，主要是添加文件时，如果有错误，后续的添加都不执行，最后获取buffer时直接返回错误
}
func NewMultiPart() *MultiPart {
	buf :=make([]byte,30)
	rand.Read(buf)
	return &MultiPart{
		buf:      new(bytes.Buffer),
		parts:    make([]*Part,0), //当Parts == nil 表示已读完
		boundary: fmt.Sprintf("%x", buf),
	}
}
func(m *MultiPart)GetBuffer()(*bytes.Buffer,error) {
	if m.partserr!=nil{
		return m.buf,m.partserr
	}
	var err error
	if m.parts != nil{
		for i,part:=range m.parts{
			if i>0 {
				_, err =fmt.Fprintf(m.buf, "\r\n--%s\r\n", m.boundary)
			} else {
				_, err =fmt.Fprintf(m.buf, "--%s\r\n", m.boundary)
			}
			if err!=nil{
				return m.buf,err
			}
			err=part.WriteToBuffer(m.buf)
			if err!=nil{
				return m.buf,err
			}
			_, err =m.buf.WriteString("\r\n")
			if err!=nil{
				return m.buf,err
			}
		}
	}
	//if len(m.parts)> 0{
	//	m.buf.WriteString("\r\n")
	//}
	_, err = fmt.Fprintf(m.buf, "\r\n--%s--\r\n", m.boundary)
	return m.buf,err
}

//返回文件名，文件内容，错误
func readFile(ifile interface{})(name string,data []byte,err error){
	switch v := ifile.(type) {
	case string:
		var pathToFile string
		pathToFile, err = filepath.Abs(v)
		if err != nil {
			return
		}
		name = filepath.Base(pathToFile)
		data,err= ioutil.ReadFile(v)
		return
	case []byte:
		return "File",v,nil
	case *os.File:

		buf := bytes.NewBuffer(make([]byte, 0))
		_,err=buf.ReadFrom(v)
		if err!=nil{
			return "",nil,err
		}
		return filepath.Base(v.Name()),buf.Bytes(),nil
	case os.File:
		return readFile(&v)
	}
	return "",nil,errors.New("Unsupports file param type.")
}

func(m *MultiPart)AppendFormFile(fieldname,filename string, ifile interface{}) {
	if m.partserr!=nil{
		return
	}
	filename1,data,err:=readFile(ifile)
	if err!=nil{
		m.partserr = err
		return
	}
	if filename =="" && filename1!=""{
		filename = filename1
	}

	//switch v := ifile.(type) {
	//case string:
	//	m.buf.WriteString(v)
	//case []byte:
	//	m.buf.Write(v)
	//case io.Reader:
	//	io.Copy(m.buf,v)
	//}
	m.parts = append(m.parts,&Part{
		Filename:filename,
		Fieldname:fieldname,
		ContentType:"application/octet-stream",
		Data:data,
	})


}

func(m *MultiPart)AppendFormField(fieldname ,contenttype string, data []byte) {
	if m.partserr!=nil{
		return
	}
	m.parts = append(m.parts,&Part{
		Fieldname:fieldname,
		ContentType:contenttype,
		Data:data,
	})
}

func (m *MultiPart) SetBoundary(boundary string) error {

	// rfc2046#section-5.1.1
	if len(boundary) < 1 || len(boundary) > 69 {
		return errors.New("mime: invalid boundary length")
	}
	for _, b := range boundary {
		if 'A' <= b && b <= 'Z' || 'a' <= b && b <= 'z' || '0' <= b && b <= '9' {
			continue
		}
		switch b {
		case '\'', '(', ')', '+', '_', ',', '-', '.', '/', ':', '=', '?':
			continue
		}
		return errors.New("mime: invalid boundary character")
	}
	m.boundary = boundary
	return nil
}
// FormDataContentType returns the Content-Type for an HTTP
// multipart/form-data with this Writer'm Boundary.
func (m *MultiPart) FormDataContentType() string {
	return "multipart/form-data; boundary=" + m.boundary
}

