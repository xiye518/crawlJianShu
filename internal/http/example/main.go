package main

import (
	"fmt"
	"github.com/xiye518/crawjianshu/internal/http"
	"log"
	"time"
)

const (
	USER_AGENT      = `Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/54.0.2840.87 Safari/537.36`
	ACCEPT_TEXT     = `text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8`
	ACCEPT_JSON     = `application/json, text/javascript, */*; q=0.01`
	ACCEPT_ENCODING = `gzip, deflate`
)

func main() {
	httpClient := http.NewClient().DialTimeout(20 * time.Second)

	req := http.NewRequest(http.MethodPost, `http://httpbin.org/post`).
		SetHeader("Connection", `keep-alive`).
		SetHeader("User-Agent", USER_AGENT).
		SetHeader("Cache-Control", "max-age=0").
		SetHeader("Accept", ACCEPT_TEXT).
		SetHeader("Accept-Encoding", ACCEPT_ENCODING).
		SetHeader("Accept-Language", `zh-CN,zh;q=0.8`).
		SetHeader("Referer", `http://httpbin.org`).
		AddMultipartFormField("abc1", []byte("ddddddd")).
		AddMultipartFormField("abc2", []byte("dddddd33d"))
	//AddForm("__EVENTTARGET","").
	//AddForm("__EVENTARGUMENT","ssss")

	b1, err := req.Dump(true)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(string(b1))

	resp, hcerr := req.SendBy(httpClient)
	if hcerr != nil {

		if hcerr.IsDnsError() || hcerr.IsDialFailed() {
			log.Println("IP block")
			return
		}
		log.Println(hcerr)
		return
	}

	log.Println(resp.StatusCode)
	b, e := resp.BodyBytes()
	log.Println(string(b), e)

}
