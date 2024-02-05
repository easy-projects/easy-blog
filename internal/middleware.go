package internal

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/easy-projects/easyblog/pkg"
	"github.com/easy-projects/easyblog/pkg/log"
	"github.com/gin-gonic/gin"
)

// ===== middlewares =====

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
func FileUpdateMiddleware(cache pkg.Cache, fileManager pkg.FileManager, fileManagerLock *sync.RWMutex, config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
		path = filepath.ToSlash(filepath.Clean(path))
		fileManagerLock.Lock()
		if fileManager.Changed(path) {
			log.Println("[file update] file changed:", path)
			cache.Remove("file:" + url)
			fileManager.SetChanged(path, false)
		} else {
			log.Println("[file update] file not changed:", path)
		}
		fileManagerLock.Unlock()
	}
}

// === cache ===
func FileCacheMiddleware(cache pkg.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		fileI, found := cache.Get("file:" + url)
		var contentType string
		if strings.HasSuffix(c.Request.URL.Path, ".png") || strings.HasSuffix(c.Request.URL.Path, ".jpg") || strings.HasSuffix(c.Request.URL.Path, ".jpeg") {
			contentType = fmt.Sprintf("image/%s", filepath.Ext(c.Request.URL.Path)[1:])
		} else {
			contentType = "text/html; charset=utf-8"
		}
		if found {
			log.Println("[cache] hit:", url)
			c.Data(http.StatusOK, contentType, fileI.([]byte))
			c.Abort()
			return
		}
		log.Println("[cache] miss:", url)
	}
}

// === handle content ===

func LoadBlogMiddleware(hide, private pkg.GitIgnorer, cache pkg.Cache, config *pkg.Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		filePath := config.BLOG_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
		log.Println("[load blog] path:", filePath)
		if !fsutil.IsExist(filepath.Clean(filePath)) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "blog not found",
			})
			return
		}
		if !strings.HasSuffix(url, ".md") && !strings.HasSuffix(filePath, ".MARKDOWN") && !strings.HasSuffix(filePath, "/") {
			bs, err := os.ReadFile(filePath)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
			}
			c.File(filePath)
			c.Set("file", bs)
			return
		}
		blog, err := pkg.LoadBlog(filePath, hide, private, config)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		cache.Set("blog:"+url, blog)
		file := []byte(blog.Html)
		c.Set("file", []byte(file))
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
func GenMiddleWare(config *pkg.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		URL := c.Request.URL.Path
		c.Next()
		c.Abort()
		if c.Writer.Status() == http.StatusOK {
			gen_path := pkg.GenPath(URL, config)
			log.Println("[gen] gen:", gen_path)
			file := c.MustGet("file").([]byte)
			if strings.HasSuffix(gen_path, "/") || strings.HasSuffix(gen_path, ".md") || strings.HasSuffix(gen_path, ".md") {
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
		// convert file paths to links
		for i, path := range results {
			// 如果路径是dir的 话,则生成的url 结尾要增加/
			results[i] = config.BLOG_ROUTER + path[len(config.BLOG_PATH):]
			results[i] = filepath.ToSlash(results[i])
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, results)
	}
}
