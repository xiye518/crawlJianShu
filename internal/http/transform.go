package http

import (
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
	"strings"
	"errors"
)

var Codec = map[string]encoding.Encoding{
	"utf-8":       encoding.Nop,
	"utf-16be":    unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM),
	"utf-16le":    unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM),
	"gb2312":      simplifiedchinese.HZGB2312,
	"gbk":         simplifiedchinese.GBK,
	"big5":        traditionalchinese.Big5,
	"gb18030":     simplifiedchinese.GB18030,
	"euc-kr":      korean.EUCKR,
	"euc-jp":      japanese.EUCJP,
	"iso-2022-jp": japanese.ISO2022JP,
	"shift-jis":   japanese.ShiftJIS,
}
var CacheDecoder = map[string]transform.Transformer{}
var Err_NotSupportEncode = errors.New("不支持的编码类型")
func Transform(src []byte,enc string)([]byte,error){
	enc = strings.ToLower(enc)
	var tf transform.Transformer
	//忽略不支持的编码
	if ed,ok:=Codec[enc];ok{
		tf ,ok= CacheDecoder[enc]
		if !ok{
			tf = ed.NewDecoder()
			CacheDecoder[enc] = tf
		}
		dst := make([]byte, len(src) * 2)
		n, _, err := tf.Transform(dst, src, true)
		return dst[:n],err

	}
	return nil,Err_NotSupportEncode
}