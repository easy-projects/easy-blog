package internal

import (
	"net/http"
	"sync"
	"time"

	"github.com/cncsmonster/fspider"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/easy-projects/easyblog/pkg"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func RouteApp(r *gin.Engine, config *pkg.Config, spider *fspider.Spider) {
	r.Use(cors.Default())
	fileManager := pkg.NewFileManager(spider)
	blogCache := pkg.NewCache(1000)
	searcherCache := pkg.NewCache(1000)
	searcherCacheLock := &sync.RWMutex{}
	hideMatcher := pkg.NewBlogIgnorer().AddPatterns(config.HIDE_PATHS...)
	privateMatcher := pkg.NewBlogIgnorer().AddPatterns(config.PRIVATE_PATHS...)
	r.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/" || c.Request.URL.Path == "/favicon.ico" {
			newUrl := config.BLOG_ROUTER + "/" + c.Request.URL.Path
			c.Redirect(http.StatusMovedPermanently, newUrl)
			c.Abort()
			return
		}
	})
	// set visit rate limit for each ip and each path
	lmt1 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_SECOND), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Second}) // 每秒最多5次
	lmt2 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_MINUTE), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Minute}) // 每分钟最多30次
	lmt3 := tollbooth.NewLimiter(float64(config.RATE_LIMITE_HOUR), &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})     // 每小时最多1000次
	r.Use(LimitMiddleware(lmt1, lmt2, lmt3))
	// blog
	blog := r.Group(config.BLOG_ROUTER)
	blog.Use(PrivateMiddleWare(privateMatcher, config))
	blog.Use(BlogUpdateMiddleware(blogCache, fileManager, config))
	blog.Use(BlogCacheMiddleware(blogCache))
	if !config.NOT_GEN {
		blog.Use(GenMiddleWare(blogCache, config))
	}
	blog.Use(LoadBlogMiddleware(hideMatcher, privateMatcher, blogCache, config))
	blog.GET("/*any")

	// api
	blogIndexer := pkg.NewBlogIndexer(config.APP_DATA_PATH + "/" + "blog.bleve")
	go func() {
		for _, path := range spider.AllPaths() {
			path = pkg.SimplifyPath(path)
			if pkg.PathMatch(path, hideMatcher, privateMatcher) {
				continue
			}
			blog, err := pkg.LoadBlog(path, hideMatcher, privateMatcher, config)
			if err == nil {
				blogIndexer.Add(blog)
			}
		}
		for path := range spider.FilesChanged() {
			searcherCacheLock.Lock()
			searcherCache.RemoveAll()
			searcherCacheLock.Unlock()
			path := pkg.SimplifyPath(path)
			if pkg.PathMatch(path, hideMatcher, privateMatcher) {
				blogIndexer.Delete(&pkg.BlogItem{Path: path})
				continue
			}
			blog, err := pkg.LoadBlog(path, hideMatcher, privateMatcher, config)
			if err == nil {
				blogIndexer.Add(blog)
			} else {
				blogIndexer.Delete(&pkg.BlogItem{Path: path})
			}
		}
	}()
	api := r.Group(config.API_ROUTER)
	searchers := map[string]pkg.Searcher{
		"title":   pkg.NewSearcherByTitle("title", "根据标题编辑距离搜索", spider, hideMatcher, privateMatcher),
		"content": pkg.NewSearchByContentMatch("content", "根据文本内容匹配搜索", spider, blogCache, hideMatcher, privateMatcher, config),
		"keyword": pkg.NewSearcherByKeywork("keyword", "根据关键词搜索", spider, blogCache, hideMatcher, privateMatcher, config),
		"bleve":   pkg.NewSearcherByBleve("bleve", "根据bleve搜索", blogIndexer),
	}
	for _, plugin := range config.SEARCH_PLUGINS {
		if plugin.Disable {
			delete(searchers, plugin.Name)
			continue
		}
		searcher := pkg.NewSearcherByPlugin(plugin, hideMatcher, privateMatcher, config)
		searchers[plugin.Name] = searcher
	}

	api.GET("/search", SearchMiddleWare(searchers, searcherCache, config))
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

}
