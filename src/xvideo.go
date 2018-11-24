package main

import (
	//"dev-data/utils"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"log"
)

var exit = make(chan bool, 1024)
//翻墙下载1
func GetProxyPage(target string) *goquery.Document {
	proxyUri, _ := url.Parse("http://127.0.0.1:1080")
	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUri),
		},
	}
	response, err := client.Get(target)
	if err != nil {
		log.Println("请求地址失败", zap.String("错误信息", err.Error()))
	}
	defer response.Body.Close()
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		log.Println("解析页面失败", zap.String("错误信息", err.Error()))
		return nil
	}
	return doc
}

func VideoList(base string, pageSize int) {
	urls := make(chan string, 1024)

	go func() {
		for i := 0; i < pageSize; i++ {
			//url := fmt.Sprintf("https://www.xvideos.com/?k=china&p=%d", i)
			url := fmt.Sprintf("https://www.xnxx.com/search/学生/%d/", i)
			fmt.Println(fmt.Sprintf("开始抓取第%d页视频", i))
			GetProxyPage(url).Find(".mozaique .thumb a").Each(func(i int, selection *goquery.Selection) {
				value, err := selection.Attr("href")
				if !err {
					log.Println("解析页面参数出错", zap.Bool("错误信息", err))
				}
				urls <- fmt.Sprintf(base, value)
			})
		}
	}()

	for {
		if url, ok := <-urls; ok {
			target := GetProxyPage(url).Find("script:contains(setVideoUrlHigh)").Text()
			if len(target) != 0 {
				rex, _ := regexp.Compile(`setVideoUrlHigh\(\'[a-zA-z]+://[^\s]*`)
				exit <- true
				go VideoDownload(strings.Split(rex.FindStringSubmatch(target)[0], "'")[1])
			}
		} else {
			break
		}
	}
}

func VideoDownload(target string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(fmt.Sprintf("gouroutine崩溃，错误信息：%s", err))
		}
	}()

	fmt.Printf("\n开始下载URL地址为：%s的视频", target)
	res, _ := http.Get(target)
	data, _ := ioutil.ReadAll(res.Body)
	fileName := fmt.Sprintf("F:/video/%s.MP4", time.Now().Format("20060102150405"))

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	defer file.Close()

	file.Write(data)

	select {
	case <-exit:
		fmt.Println(fmt.Sprintf("\n下载URL地址为：%s的视频成功, 退出Goroutine！\n", target))
		return
	}
}
