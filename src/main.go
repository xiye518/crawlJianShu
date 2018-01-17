package main

import (
	"fmt"
	"log"
	"http"
	"time"
	"errors"
	"tools/console/color"
	"regexp"
)

func init() {
	color.NoColor = false
}

func main() {
	color.LogAndPrintln(color.HiCyan("this is a crawlJianshu test\n"))
	httpClient := http.NewClient().DialTimeout(20 * time.Second)
	
	//此处通过http请求获取到简书首页的文本html
	body, err := searchlJianShuHome(httpClient)
	if err != nil {
		log.Fatal(err)
	}
	
	//color.LogAndPrintln(color.HiGreen(body))
	//ToDo  也可以用ioutil将文本写出
	//此处为使用正则从文本中切取想要的内容
	arts, err := getArticles(body)
	if err != nil {
		log.Fatal(err)
	}
	
	for i, a := range arts {
		color.LogAndPrintln(i, color.HiGreen(a.Title), "https://www.jianshu.com"+a.Url,a.Abstract)
	}
}

func searchlJianShuHome(httpClient *http.Client) (body string, err error) {
	resp, hcerr := http.NewRequest(http.MethodGet,
		`https://www.jianshu.com/`).
		SetHeader(`Accept`, `text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8`).
		SetHeader(`Accept-Encoding`, `gzip, deflate, br`).
		SetHeader(`Accept-Language`, `zh-CN,zh;q=0.9`).
		SetHeader(`Connection`, `keep-alive`).
		SetHeader(`Upgrade-Insecure-Requests`, `1`).
		SetHeader(`User-Agent`, `Mozilla/5.0 (Windows NT 6.3; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/63.0.3239.132 Safari/537.36`).
		
		
		SendBy(httpClient)
	if hcerr != nil {
		return body, errors.New(fmt.Sprintf("%s", hcerr))
	}
	defer resp.Body.Close()
	
	bytes, err := resp.BodyBytes()
	if err != nil {
		return body, err
	}
	
	body = string(bytes)
	
	return body, nil
}

func getArticles(body string) (arts []*Article, err error) {
	arts = make([]*Article, 0)
	//reg:=regexp.MustCompile(`(?s)<a class="title" target="_blank" href="(.+?)">(.+?)</a>`)
	reg := regexp.MustCompile(`(?s)<a class="title" target="_blank" href="(.+?)">(.+?)</a>\s*<p class="abstract">\s*(.+?)</p>`)
	result := reg.FindAllStringSubmatch(body, -1) //...		/p/6603d0ad230f	大脑版本升级：练习三个思维模型，一下午就能让你聪明起来  描述...
	for _, r := range result {
		var a Article
		a.Url = r[1]
		a.Title = r[2]
		a.Abstract = r[3]
		arts = append(arts, &a)
	}
	
	return arts, err
}

type Article struct {
	Title      string
	AUthor     string
	Abstract   string
	Url        string
	Watched    string //已阅
	Comment    string //点评数
	Collection string //收藏数
}
