package main

import (
	"fmt"
	"os"

	"log"

	"github.com/gin-contrib/cors"

	"github.com/cncsmonster/fspider"
	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/easy-projects/easyblog/internal"
	"github.com/easy-projects/easyblog/pkg"

	. "github.com/easy-projects/easyblog/pkg"
	"github.com/gin-gonic/gin"
)

func main() {
	// 加入命令行解析,如果有-h参数,打印帮助信息
	// 如果有 -g 参数,则生成静态网页
	// 如果没有参数,则启动服务器
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "-s")
	}
	switch os.Args[1] {
	case "-h":
		Help()
	case "-n":
		New()
	case "-s":
		Serve(pkg.LoadConfig("eb.yaml"))
	case "-v":
		Version()
	default:
		fmt.Println("unknown command")
	}
}

func Help() {
	help_message := string(DEFAULT_HELP)
	fmt.Println(help_message)
}

func New() {
	fsutil.MustWrite("eb.yaml", DEFAULT_CONFIG)
	fsutil.MustWrite("blog/intro.md", DEFAULT_BLOG)
	fsutil.MustWrite("blog/private.md", DEFAULT_PRIVATE)
	fsutil.MustWrite("blog/keyword.md", DEFAULT_KEYWORD)
	fsutil.MustWrite("blog/hide.md", DEFAULT_HIDE)
	fsutil.MustWrite("./template.html", DEFAULT_TEMPLATE)
	fsutil.MustWrite("blog/favicon.ico", DEFAULT_FAVICON)
	fsutil.MustWrite("blog/vue.js", DEFAULT_VUE_JS)
}

func Serve(config *Config) {
	r := gin.Default()
	r.Use(cors.Default())
	spider := fspider.NewSpider()
	defer spider.Stop()
	spider.Spide(config.BLOG_PATH)
	internal.RouteApp(r, config, spider)
	port := fmt.Sprintf(":%d", config.PORT)
	if err := r.Run(port); err != nil {
		log.Fatal(err)
	}
}

func Version() {
	fmt.Println(string(DEFAULT_VERSION))
}
