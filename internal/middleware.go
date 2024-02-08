package internal

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/easy-projects/easyblog/pkg"
	"github.com/easy-projects/easyblog/pkg/log"
	"github.com/gin-gonic/gin"
)

// ===== middlewares =====

// === redirect ===
func RedirectHomePageMiddleware(config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/" || c.Request.URL.Path == "/favicon.ico" {
			newUrl := config.BLOG_ROUTER + "/" + c.Request.URL.Path
			c.Redirect(http.StatusMovedPermanently, newUrl)
			c.Abort()
			return
		}
	}
}

// === rate limit ===
func LimitMiddleware(lmts ...*limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, lmt := range lmts {
			httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request)
			if httpError != nil {
				c.AbortWithStatusJSON(httpError.StatusCode, gin.H{
					"error": httpError.Message,
				})
				c.Abort()
				return
			}
		}
	}
}

// === index ===
func BlogUpdateMiddleware(blogCache pkg.Cache, fileManager pkg.FileManager, config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
		path = filepath.ToSlash(filepath.Clean(path))
		if fileManager.Changed(path) {
			log.Println("[file update] file changed:", path)
			blogCache.Remove(url)
			fileManager.SetChanged(path, false)
		} else {
			log.Println("[file update] file not changed:", path)
		}
	}
}

// === cache ===
func BlogCacheMiddleware(blogCache pkg.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		blogI, found := blogCache.Get(url)
		var contentType string
		if strings.HasSuffix(c.Request.URL.Path, ".png") || strings.HasSuffix(c.Request.URL.Path, ".jpg") || strings.HasSuffix(c.Request.URL.Path, ".jpeg") {
			contentType = fmt.Sprintf("image/%s", filepath.Ext(c.Request.URL.Path)[1:])
		} else {
			contentType = "text/html; charset=utf-8"
		}
		if found {
			log.Println("[cache] hit:", url)
			blog := blogI.(*pkg.BlogItem)
			c.Data(http.StatusOK, contentType, []byte(blog.Html))
			c.Abort()
			return
		}
		log.Println("[cache] miss:", url)
	}
}

// === handle content ===

func LoadBlogMiddleware(hide, private pkg.GitIgnorer, blogCache pkg.Cache, config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		filePath := config.BLOG_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
		log.Println("[load blog] path:", filePath)
		blog, err := pkg.LoadBlog(filePath, hide, private, config)
		if err != nil {
			c.AbortWithError(http.StatusNotFound, err)
			return
		}
		blogCache.Set(url, blog)
		file := []byte(blog.Html)
		c.Data(http.StatusOK, "text/html; charset=utf-8", file)
	}
}

// === handle private ===
func PrivateMiddleWare(private pkg.GitIgnorer, config *pkg.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
		path = pkg.SimplifyPath(path)
		log.Println("[check private] path:", path)
		if pkg.PathMatch(path, private) {
			log.Println("[check private] path match private:", path)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
	}
}

// === handle gen ===
func GenMiddleWare(blogCache pkg.Cache, config *pkg.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.NOT_GEN {
			return
		}
		URL := c.Request.URL.Path
		c.Next()
		c.Abort()
		if c.Writer.Status() == http.StatusOK {
			gen_path := pkg.GenPath(URL, config)
			log.Println("[gen] gen:", gen_path)
			blogI, found := blogCache.Get(URL)
			if !found {
				log.Println("[gen] blog not found in cache:", URL)
				return
			}
			blog := blogI.(*pkg.BlogItem)
			var file []byte = []byte(blog.Html)
			if blog.IsDir() || blog.IsMd() {
				file = pkg.TransformLinks(file, config)
			}
			if err := fsutil.MustWrite(gen_path, file); err != nil {
				panic(err)
			}
		}
	}
}

// === handle search ===
func SearchMiddleWare(searchers map[string]pkg.Searcher, cache pkg.Cache, config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		keyword := c.Query("keyword")
		if keyword == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "keyword is empty",
			})
			return
		}
		log.Println("[search] search:", keyword)
		num := config.SEARCH_NUM
		if n, find := c.GetQuery("num"); find {
			n, err := strconv.Atoi(n)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": "num must be int",
				})
				return
			}
			num = n
		}
		searchType := c.Query("searchType")
		if searchType == "" {
			searchType = "title"
		}
		log.Println("[search] search type:", searchType)
		searcher, found := searchers[searchType]
		if !found {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "search type not found",
			})
			return
		}
		results, err := searcher.Search(keyword, num)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		retResults := make([]string, 0, len(results))
		// convert file paths to links
		for _, path := range results {
			if path == "" || len(path) < len(config.BLOG_PATH) {
				log.Println("[search] result  path:", path, "is empty or too short")
				continue
			}
			path = filepath.ToSlash(path)
			path = config.BLOG_ROUTER + path[len(config.BLOG_PATH):]
			path = pkg.SimplifyPath(path)
			retResults = append(retResults, path)
		}
		c.JSON(http.StatusOK, retResults)
	}
}
