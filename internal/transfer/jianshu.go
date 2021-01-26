package transfer

import (
	"regexp"

	"github.com/xiye518/crawjianshu/internal/tools/console/color"
)

func ParseArticles(body string) (arts []*Article, err error) {
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

func (a *Article) String(i int) {
	color.LogAndPrintln(i, color.HiGreen(a.Title), "https://www.jianshu.com"+a.Url, a.Abstract)
}
