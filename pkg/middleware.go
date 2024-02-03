package pkg

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gin-gonic/gin"
	ignore "github.com/sabhiram/go-gitignore"
)

// ===== middlewares =====

// === rate limit ===
func LimitMiddleware(lmt *limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request)
		if httpError != nil {
			c.AbortWithStatusJSON(httpError.StatusCode, gin.H{
				"error": httpError.Message,
			})
			return
		}
		c.Next()
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
			cache.Remove("html:" + url)
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
		content, found := cache.Get("html:" + url)
		contentType := func() string {
			if strings.HasSuffix(c.Request.URL.Path, ".png") || strings.HasSuffix(c.Request.URL.Path, ".jpg") || strings.HasSuffix(c.Request.URL.Path, ".jpeg") {
				return fmt.Sprintf("image/%s", filepath.Ext(c.Request.URL.Path)[1:])
			}
			return "text/html; charset=utf-8"
		}
		if found {
			log.Println("[cache] hit:", url)
			c.Data(http.StatusOK, contentType(), content.([]byte))
			c.Abort()
			return
		}
		c.Next()
		c.Abort()
		if c.Writer.Status() == http.StatusOK {
			contentI, found := c.Get("html")
			if found {
				cache.Set("html:"+url, contentI)
			}
			content, found = c.Get("file")
			if found {
				cache.Set("file:"+url, content)
			}
			meta, found := c.Get("meta")
			if found {
				cache.Set("meta:"+url, meta)
			}
		}
	}
}

// === handle content ===

// if file is dir or md,then render to html
func RenderMdMiddleware(config *Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Next()
		c.Abort()
		url := c.Request.URL.Path
		file, found := c.Get("file")
		found = found && (strings.HasSuffix(url, ".md") || strings.HasSuffix(url, "/"))
		if !found {
			return
		}
		meta, err := MdMeta(file.([]byte))
		if err != nil {
			log.Println("[render md] get meta error:", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if meta.Title == "" {
			meta.Title = filepath.Base(url)
			meta.Title = meta.Title[:len(meta.Title)-len(filepath.Ext(meta.Title))]
		}
		c.Set("meta", meta)
		log.Println("[render md] render md:", url)
		html, err := Md2Html(file.([]byte), meta.Title, config)
		if err != nil {
			log.Println("[render md] render md error:", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		log.Println("[render md] render md success:", url)
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
		c.Set("html", html)
	}
}

func LoadFileMiddleware(hide, private *ignore.GitIgnore, config *Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		filePath := config.BLOG_PATH + "/" + url[len(BLOG_ROUTER)+1:]
		stat, err := os.Stat(filePath)
		if err != nil {
			log.Println("[load file] load file error:", err)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		var bs []byte
		var action string
		if stat.IsDir() {
			bs, err = RenderDir(filePath, hide, private, config)
			action = "load dir"
		} else {
			bs, err = os.ReadFile(filePath)
			action = "load file"
		}
		if err != nil {
			log.Printf("[load file] %s error: %e\n", action, err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Set("file", bs)
		c.Data(http.StatusOK, "text/html; charset=utf-8", bs)
	}
}

// === handle private ===
func PrivateMiddleWare(private *ignore.GitIgnore, config *Config) gin.HandlerFunc {
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
		c.Next()
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
			var data []byte
			dataI, found := c.Get("html")
			if found {
				data = dataI.([]byte)
				log.Println("[gen] transform links in:", URL)
				data = TransformLinks(data, config)
			} else {
				dataI := c.MustGet("file")
				data = dataI.([]byte)
			}
			if err := fsutil.MustWrite(gen_path, data); err != nil {
				panic(err)
			}
		}
	}
}

// === handle search ===
func SearchMiddleWare(searchers map[string]Searcher, cache Cache, fMgr FileManager, config *Config) func(c *gin.Context) {
	// cache := NewCache(1000)
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
		cache.Set(keyword, results)
		c.JSON(http.StatusOK, results)
	}
}
