package easyblog

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	. "github.com/easy-projects/easyblog/pkg"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	ignore "github.com/sabhiram/go-gitignore"
)

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
}

func Serve(config *Config) {
	r := gin.Default()
	r.Use(cors.Default())
	fileManager := NewFileManager(config.BLOG_PATH)
	fileManagerLock := &sync.RWMutex{}
	fileCache := NewCache(1000)
	searcherCache := NewCache(1000)
	hideMatcher := ignore.CompileIgnoreLines(config.HIDE_PATHS...)
	privateMatcher := ignore.CompileIgnoreLines(config.PRIVATE_PATHS...)
	r.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/" {
			c.Redirect(http.StatusMovedPermanently, "/blog/")
			c.Abort()
			return
		}
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.File(config.BLOG_PATH + "/favicon.ico")
		c.Abort()
	})
	// set visit rate limit for each ip and each path
	lmt1 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_SECOND), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Second}) // 每秒最多5次
	lmt2 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_MINUTE), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Minute}) // 每分钟最多30次
	lmt3 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_HOUR), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})     // 每小时最多1000次
	r.Use(LimitMiddleware(lmt1), LimitMiddleware(lmt2), LimitMiddleware(lmt3))
	// router

	// blog
	blog := r.Group(BLOG_ROUTER)
	blog.Use(FileUpdateMiddleware(fileCache, fileManager, fileManagerLock, config))
	blog.Use(PrivateMiddleWare(privateMatcher, config))
	blog.Use(FileCacheMiddleware(fileCache))
	blog.Use(GenMiddleWare(config))
	blog.Use(RenderMdMiddleware(config))
	blog.Use(LoadFileMiddleware(hideMatcher, privateMatcher, config))
	blog.GET("/*any")

	// api
	api := r.Group(API_ROUTER)
	searchers := map[string]Searcher{
		"title":   NewSearcherByTitleEditDistance(fileManager, hideMatcher, privateMatcher),
		"content": NewSearchByContentMatch(fileManager, hideMatcher, privateMatcher),
		"keyword": NewSearcherByKeywork(fileManager, hideMatcher, privateMatcher),
	}
	// 根据配置文件 加载搜索器插件
	for _, plugin := range config.SEARCH_PLUGINS {
		searcher := NewSearcherByPlugin(fileManager, hideMatcher, privateMatcher, plugin[1], config)
		searchers[plugin[0]] = searcher
	}

	api.GET("/search", SearchMiddleWare(searchers, searcherCache, fileManager, config))
	api.GET("/searchers", func(c *gin.Context) {
		searcherNames := make([]string, 0, len(searchers))
		for k := range searchers {
			searcherNames = append(searcherNames, k)
		}
		c.JSON(http.StatusOK, searcherNames)
	})

	port := fmt.Sprintf(":%d", config.PORT)
	if err := r.Run(port); err != nil {
		log.Fatal(err)
	}
}

func Version() {
	fmt.Println(string(DEFAULT_VERSION))
}
