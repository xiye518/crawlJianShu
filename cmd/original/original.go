package main

import (
	"io/ioutil"
	"log"
	"net/http"

	"github.com/xiye518/crawjianshu/internal/tools/console/color"
	"github.com/xiye518/crawjianshu/internal/transfer"
)

func init() {
	color.NoColor = false
}

func main() {
	color.LogAndPrintln(color.HiCyan("this is a crawlJianshu test\n"))

	//此处通过http请求获取到简书首页的文本html
	body, err := getHtml()
	if err != nil {
		log.Fatal(err)
	}

	//color.LogAndPrintln(color.HiGreen(body))
	//ToDo  也可以用ioutil将文本写出
	//此处为使用正则从文本中切取想要的内容
	arts, err := transfer.ParseArticles(body)
	if err != nil {
		log.Fatal(err)
	}

	for i, a := range arts {
		//color.LogAndPrintln(i, color.HiGreen(a.Title), "https://www.jianshu.com"+a.Url,a.Abstract)
		a.String(i)
	}
}

func getHtml() (body string, err error) {
	//v:=url.Values{}
	//v.Add("","")
	client := &http.Client{}

	req, err := http.NewRequest("GET", "https://www.jianshu.com/", nil)
	if err != nil {
		return body, err
	}

	req.Header.Set(`Accept`, `text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8`)
	//req.Header.Set(`Accept-Encoding`, `gzip, deflate, br`)
	req.Header.Set(`Accept-Language`, `zh-CN,zh;q=0.9`)
	req.Header.Set(`Connection`, `keep-alive`)
	req.Header.Set(`Upgrade-Insecure-Requests`, `1`)
	req.Header.Set(`User-Agent`, `Mozilla/5.0 (Windows NT 6.3; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/63.0.3239.132 Safari/537.36`)

	resp, err := client.Do(req)
	if err != nil {
		return body, err
	}

	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, err
	}

	body = string(bytes)

	//color.LogAndPrintln("body:",color.HiCyan(body))

	return body, nil
}
