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
	fsutil.MustWrite("blog/vue.js", DEFAULT_VUE_JS)
}

func Serve(config *Config) {
	r := gin.Default()
	r.Use(cors.Default())
	fileManager := NewFileManager(config.BLOG_PATH)
	defer fileManager.Close()
	fileManagerLock := &sync.RWMutex{}
	fileCache := NewCache(1000)
	searcherCache := NewCache(1000)
	hideMatcher := NewBlogIgnorer().AddPatterns(config.HIDE_PATHS...)
	privateMatcher := NewBlogIgnorer().AddPatterns(config.PRIVATE_PATHS...)
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

	// blog
	blog := r.Group(BLOG_ROUTER)
	blog.Use(FileUpdateMiddleware(fileCache, fileManager, fileManagerLock, config))
	blog.Use(PrivateMiddleWare(privateMatcher, config))
	blog.Use(FileCacheMiddleware(fileCache))
	if !config.NOT_GEN {
		blog.Use(GenMiddleWare(config))
	}
	blog.Use(RenderMdMiddleware(config))
	blog.Use(LoadFileMiddleware(hideMatcher, privateMatcher, config))
	blog.GET("/*any")

	// api
	api := r.Group(API_ROUTER)
	searchers := map[string]Searcher{
		"title":   NewSearcherByTitleEditDistance("title", "根据标题编辑距离搜索", fileManager, hideMatcher, privateMatcher),
		"content": NewSearchByContentMatch("content", "根据文本内容匹配搜索", fileManager, hideMatcher, privateMatcher),
		"keyword": NewSearcherByKeywork("keyword", "根据关键词搜索", fileManager, hideMatcher, privateMatcher),
	}
	// 根据配置文件 加载搜索器插件
	for _, plugin := range config.SEARCH_PLUGINS {
		searcher := NewSearcherByPlugin(plugin, fileManager, hideMatcher, privateMatcher, config)
		searchers[plugin.Name] = searcher
	}

	api.GET("/search", SearchMiddleWare(searchers, searcherCache, fileManager, config))
	api.GET("/searchers", func(c *gin.Context) {
		type JsonSearcher struct {
			Type  string `json:"type"`
			Brief string `json:"brief"`
		}
		jsonSearchers := make([]JsonSearcher, 0, len(searchers))
		for _, searcher := range searchers {
			jsonSearchers = append(jsonSearchers, JsonSearcher{
				Type:  searcher.Name(),
				Brief: searcher.Brief(),
			})
		}
		c.JSON(http.StatusOK, jsonSearchers)
	})

	port := fmt.Sprintf(":%d", config.PORT)
	if err := r.Run(port); err != nil {
		log.Fatal(err)
	}
}

func Version() {
	fmt.Println(string(DEFAULT_VERSION))
}
