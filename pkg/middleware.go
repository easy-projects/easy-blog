package pkg

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
func FileUpdateMiddleware(cache Cache, fileManager FileManager, fileManagerLock *sync.RWMutex, config *Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(BLOG_ROUTER)+1:]
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
func FileCacheMiddleware(cache Cache) gin.HandlerFunc {
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

func LoadFileMiddleware(hide, private GitIgnorer, cache Cache, config *Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		filePath := config.BLOG_PATH + "/" + url[len(BLOG_ROUTER)+1:]
		filePath = SimplifyPath(filePath)
		log.Println("[load blog] path:", filePath)
		if !fsutil.IsExist(filepath.Clean(filePath)) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "blog not found",
			})
			return
		}
		var file []byte
		if strings.HasSuffix(filePath, ".md") || strings.HasSuffix(filePath, ".markdown") || strings.HasSuffix(filePath, ".MD") || strings.HasSuffix(filePath, ".MARKDOWN") || strings.HasSuffix(filePath, "/") {
			blog, err := LoadBlog(filePath, hide, private, config)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}
			cache.Set("blog:"+url, blog)
			file = []byte(blog.Html)
		} else {
			var err error
			file, err = os.ReadFile(filePath)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}
		}
		cache.Set("file:"+url, []byte(file))
		c.Data(http.StatusOK, "text/html; charset=utf-8", file)
	}
}

// === handle private ===
func PrivateMiddleWare(private GitIgnorer, config *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(BLOG_ROUTER)+1:]
		path = SimplifyPath(path)
		log.Println("[check private] path:", path)
		if PathMatch(path, private) {
			log.Println("[check private] path match private:", path)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
	}
}

// === handle gen ===
func GenMiddleWare(config *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		URL := c.Request.URL.Path
		c.Next()
		c.Abort()
		if c.Writer.Status() == http.StatusOK {
			gen_path := GenPath(URL, config)
			log.Println("[gen] gen:", gen_path)
			blogI := c.MustGet("blog")
			blog := blogI.(*BlogItem)
			if err := fsutil.MustWrite(gen_path, []byte(blog.Html)); err != nil {
				panic(err)
			}
		}
	}
}

// === handle search ===
func SearchMiddleWare(searchers map[string]Searcher, cache Cache, config *Config) func(c *gin.Context) {
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
			results[i] = BLOG_ROUTER + path[len(config.BLOG_PATH):]
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
