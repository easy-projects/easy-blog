// implements a easy blog server
package eb

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cncsmonster/fspider"
	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	lru "github.com/hashicorp/golang-lru"
	ignore "github.com/sabhiram/go-gitignore"
	"github.com/texttheater/golang-levenshtein/levenshtein"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
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
	// 使用go-gitignore来忽略隐藏文件
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
	lmt1 := tollbooth.NewLimiter(5, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Second})  // 每秒最多5次
	lmt2 := tollbooth.NewLimiter(30, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Minute}) // 每分钟最多30次
	lmt3 := tollbooth.NewLimiter(1000, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour}) // 每小时最多1000次
	r.Use(LimitMiddleware(lmt1), LimitMiddleware(lmt2), LimitMiddleware(lmt3))
	// router

	// blog
	blog := r.Group(BLOG_ROUTER)
	blog.Use(FileUpdateMiddleware(fileCache, fileManager, fileManagerLock, config))
	blog.Use(PrivateMiddleWare(privateMatcher, config))
	blog.Use(FileCacheMiddleware(fileCache))
	blog.Use(GenMiddleWare(config))
	blog.Use(LoadFileMiddleware(hideMatcher, privateMatcher, config))
	blog.GET("/*any")

	// api
	api := r.Group("/api")
	searcher := NewSearcherByTitleEditDistance(fileManager, hideMatcher, privateMatcher)
	// searcher := NewSearcherByPlugin(fileManager, hideMatcher, privateMatcher, &config)
	// searcher := NewSearcherByKeywork(fileManager, hideMatcher, privateMatcher)
	api.GET("/search", SearchMiddleWare(searcher, searcherCache, fileManager, config))
	port := fmt.Sprintf(":%d", config.PORT)
	if err := r.Run(port); err != nil {
		log.Fatal(err)
	}
}

func Version() {
	fmt.Println(string(DEFAULT_VERSION))
}

// ====== config =====

const (
	BLOG_ROUTER string = "/blog"
	API_ROUTER  string = "/api"
)

type Config struct {
	// 通过结构体标签忽略对 sync.RWMutex 的序列化
	sync.RWMutex  `yaml:"-"`
	PORT          int
	BLOG_PATH     string
	GEN_PATH      string
	HIDE_PATHS    []string
	PRIVATE_PATHS []string
	TEMPLATE_PATH string
	APP_DATA_PATH string
	SEARCH_NUM    int
	SEARCH_PLUGIN string
	// interface
}

func LoadConfig(file string) *Config {
	var config Config
	// load config from yaml
	data, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatal(err)
	}
	config.BLOG_PATH = SimplifyPath(config.BLOG_PATH)
	config.GEN_PATH = SimplifyPath(config.GEN_PATH)
	if config.SEARCH_NUM == 0 {
		config.SEARCH_NUM = 12
	}
	data, err = yaml.Marshal(&config)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[config] config:\n", string(data))
	if _, err := os.Stat(config.BLOG_PATH); err != nil {
		if os.IsNotExist(err) {
			log.Fatal("blog path not exist")
		} else {
			log.Fatal(err)
		}
	}
	if _, err := os.Stat(config.TEMPLATE_PATH); err != nil {
		if os.IsNotExist(err) {
			log.Fatal("template path not exist")
		} else {
			log.Fatal(err)
		}
	}
	return &config
}

// ====== components =====

// === cache ===

type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	Remove(key string)
}
type cache struct {
	arc *lru.ARCCache
}

func (c cache) Get(key string) (interface{}, bool) {
	return c.arc.Get(key)
}
func (c cache) Set(key string, value interface{}) {
	c.arc.Add(key, value)
}
func (c cache) Remove(key string) {
	c.arc.Remove(key)
}

func NewCache(size int) Cache {
	arc, err := lru.NewARC(size)
	if err != nil {
		panic(err)
	}
	return cache{arc: arc}
}

// === index ===

// for  check if  a file is changed locally
type FileManager interface {
	// Paths 返回所有有记录的文件路径,(也就是所有追踪中的文件的路径)
	Paths() []string
	// Changed 返回path对应的文件是否被修改过,
	// 如果输入的是一个目录,则返回目录下的文件是否被修改过,如果输入的是一个文件,则返回该文件是否被修改过，
	// 如果输入的是空字符串,则判断是否监控的文件 有 没有被处理过 的修改通知
	Changed(path string) bool
	// SetChanged 设置path对应的文件是否被修改过
	SetChanged(path string, changed bool)
}
type fileManagerImpl struct {
	pathChanged map[string]bool
	changedNum  atomic.Int32
	spider      *fspider.Spider
	sync.RWMutex
}

func NewFileManager(rootPath string) FileManager {
	fileManager := &fileManagerImpl{
		pathChanged: make(map[string]bool, 100),
		changedNum:  atomic.Int32{},
		spider:      fspider.NewSpider(),
		RWMutex:     sync.RWMutex{},
	}
	fileManager.changedNum.Store(0)
	fileManager.spider.Spide(rootPath)
	go func() {
		for path := range fileManager.spider.FilesChanged() {
			path = filepath.ToSlash(filepath.Clean(path))
			dir := filepath.ToSlash(filepath.Clean(path))
			log.Println(path, "changed")
			fileManager.SetChanged(path, true)
			fileManager.SetChanged(dir, true)
		}
	}()
	return fileManager
}
func (fMgr *fileManagerImpl) Paths() []string {
	fMgr.RLock()
	defer fMgr.RUnlock()
	paths := fMgr.spider.AllFiles()
	for _, path := range fMgr.spider.AllDirs() {
		if strings.HasSuffix(path, "/") {
			continue
		}
		paths = append(paths, path+"/")
	}
	return paths
}

// get if file changed
func (fMgr *fileManagerImpl) Changed(path string) bool {
	if path == "" {
		return fMgr.changedNum.Load() != 0
	}
	fMgr.RLock()
	changed := fMgr.pathChanged[path]
	fMgr.RUnlock()
	return changed
}

// set file changed
func (fMgr *fileManagerImpl) SetChanged(path string, changed bool) {
	fMgr.Lock()
	if pastChanged, found := fMgr.pathChanged[path]; found && pastChanged == changed {
		log.Println("[file manager] no need to set changed on path:", path)
		fMgr.Unlock()
		return
	}
	fMgr.pathChanged[path] = changed
	if changed {
		log.Println("[file manager] publish changed on path:", path)
		fMgr.changedNum.Add(1)
	} else {
		log.Println("[file manager] process changed on path:", path)
		fMgr.changedNum.Add(-1)
	}
	fMgr.Unlock()
}

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
			cache.Remove(url)
			fileManager.SetChanged(path, false)
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
			md := c.MustGet("md").([]byte)
			html := c.MustGet("html").([]byte)
			meta := c.MustGet("meta").(Meta)
			cache.Set("html:"+url, html)
			cache.Set("md:"+url, md)
			cache.Set("meta:"+url, meta)
		}
	}
}

// === handle content ===
func LoadFileMiddleware(hide, private *ignore.GitIgnore, config *Config) func(c *gin.Context) {
	return func(c *gin.Context) {
		url := c.Request.URL.Path
		path := config.BLOG_PATH + "/" + url[len(BLOG_ROUTER)+1:]
		md, html, meta, err := LoadBlog(path, hide, private, config)
		if err != nil {
			log.Println("[load md] load blog error:", err)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Set("meta", meta)
		c.Set("md", md)
		c.Set("html", html)
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
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
			data := c.MustGet("html").([]byte)
			gen_path := GenPath(URL, config)
			log.Println("[gen] gen:", gen_path)
			log.Println("[gen] transform links in:", URL)
			data = TransformLinks(data, config)
			if err := fsutil.MustWrite(gen_path, data); err != nil {
				panic(err)
			}
		}
	}
}

// === handle search ===
func SearchMiddleWare(searcher Searcher, cache Cache, fMgr FileManager, config *Config) func(c *gin.Context) {
	// cache := NewCache(1000)
	return func(c *gin.Context) {
		keyword := c.Query("keyword")
		if keyword == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "keyword is empty",
			})
			return
		}
		// TODO,fix
		// if !fMgr.Changed("") {
		// 	if val, found := cache.Get(keyword); found {
		// 		log.Println("[search] hit cache:", keyword)
		// 		c.JSON(http.StatusOK, val)
		// 		return
		// 	}
		// }
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
		results, err := searcher.Search(keyword, num)
		// convert file paths to links
		for i, path := range results {
			// 如果路径是dir的 话,则生成的url 结尾要增加/
			results[i] = BLOG_ROUTER + path[len(config.BLOG_PATH):]
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

// ===== functionalities =====

// === path process ===
func SimplifyPath(path string) string {
	path = filepath.Clean(path)
	path = filepath.ToSlash(path)
	return path
}

// === md2html ===

// use pandoc to convert md to html
func Md2Html(md []byte, title string, config *Config) (html []byte, err error) {
	// pandoc -s --template=template.html --toc  --mathjax -f markdown -t html --metadata title="title"
	cmd := exec.Command("pandoc", "-s", "--template="+config.TEMPLATE_PATH, "--toc", "--mathjax", "-f", "markdown", "-t", "html", "--metadata", "title="+title)
	cmd.Stdin = bytes.NewReader(md)
	bs, err := cmd.Output()
	return bs, err
}

// load md
func LoadBlog(path string, hide, private *ignore.GitIgnore, config *Config) (md, html []byte, meta Meta, err error) {
	path = SimplifyPath(path)
	stat, err := os.Stat(path)
	meta = Meta{}
	if err != nil {
		return nil, nil, meta, err
	}
	if stat.IsDir() {
		log.Println("[load md] path is dir:", path)
		items, err := os.ReadDir(path)
		if err != nil {
			return nil, nil, meta, err
		}
		var dir bytes.Buffer
		for _, item := range items {
			name := item.Name()
			full_path := path + "/" + name
			full_path = SimplifyPath(full_path)
			if PathMatch(full_path, hide, private) {
				log.Println("[load md] path in dir ignored:", full_path)
				continue
			}
			isDir := item.IsDir()
			if isDir {
				full_path += "/"
				name += "/"
			}
			url := BLOG_ROUTER + full_path[len(config.BLOG_PATH):]
			name = filepath.ToSlash(name)
			dir.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a><br>", url, name))
		}
		md = dir.Bytes()
	} else {
		log.Println("[load md] path is file:", path)
		md, err = os.ReadFile(path)
		if err != nil {
			return nil, nil, meta, err
		}
		meta, err = MdMeta(md)
		if err != nil {
			return nil, nil, meta, err
		}
	}
	if meta.Title == "" {
		meta.Title = filepath.Base(path)
		meta.Title = meta.Title[:len(meta.Title)-len(filepath.Ext(meta.Title))]
	}
	html, err = Md2Html(md, meta.Title, config)
	return md, html, meta, err
}

// === search ===

// Searcher 搜索器
type Searcher interface {
	// Search returns the top num results related to keyword,
	// and the results are sorted from high to low by relevance
	// If the number of results found is less than num,
	// all results are returned
	// notice: the return paths are file paths
	Search(keyword string, num int) ([]string, error)
}

// SearcherFunc
type SearcherFunc func(keyword string, num int) ([]string, error)

// SearcherFunc implements Searcher interface
func (f SearcherFunc) Search(keyword string, num int) ([]string, error) {
	return f(keyword, num)
}

// searcher according to title edit distance
func NewSearcherByTitleEditDistance(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		paths := fileManager.Paths()
		results := make([]string, 0, num)
		type _Item struct {
			path string
			dist int
		}
		items := make([]_Item, 0, len(paths))
		for _, path := range paths {
			if PathMatch(path, hideMatcher, privateMatcher) {
				continue
			}
			title := filepath.Base(path)
			title = title[:len(title)-len(filepath.Ext(title))]
			dist := levenshtein.DistanceForStrings([]rune(keyword), []rune(title), levenshtein.DefaultOptions)
			items = append(items, _Item{path: path, dist: dist})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].dist < items[j].dist {
				return true
			} else if items[i].dist > items[j].dist {
				return false
			} else {
				return items[i].path < items[j].path
			}
		})

		for _, item := range items {
			results = append(results, item.path)
			if len(results) >= num {
				break
			}
		}
		return results, nil
	})
}

// searcher according to title word2vec
func NewSearcherByTitleWord2Vec(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		// TODO
		return nil, nil
	})
}

// searcher according to plugin
func NewSearcherByPlugin(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore, config *Config) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		plugin := config.SEARCH_PLUGIN
		cmd := exec.Command(plugin)
		cmd.Stdin = bytes.NewReader([]byte(keyword + "\n"))
		bs, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		bss := bytes.Split(bs, []byte("\n"))
		results := make([]string, 0, len(bss))
		for _, bs := range bss {
			path := string(bs)
			if PathMatch(path, hideMatcher, privateMatcher) {
				continue
			}
			results = append(results, path)
		}
		return results, nil
	})
}

// searcher according to search-keyword and keywords in meta
func NewSearcherByKeywork(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		log.Println("[search by keyword] keyword:", keyword)
		type _Item struct {
			path        string
			keyMatchNum int
		}
		paths := fileManager.Paths()
		items := make([]_Item, 0, len(paths))
		for _, path := range paths {
			if PathMatch(path, hideMatcher, privateMatcher) {
				continue
			}
			md, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			meta, err := MdMeta(md)
			if err != nil {
				return nil, err
			}
			keys := meta.KeyWords
			var keyMatchNum int
			for _, key := range keys {
				if strings.Contains(key, keyword) {
					keyMatchNum++
				}
			}
			// check how many keywords in content
			md_s := string(md)
			if strings.Contains(md_s, keyword) {
				keyMatchNum++
			}
			items = append(items, _Item{path: path, keyMatchNum: keyMatchNum})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].keyMatchNum > items[j].keyMatchNum {
				return true
			} else if items[i].keyMatchNum < items[j].keyMatchNum {
				return false
			} else {
				return items[i].path < items[j].path
			}
		})
		results := make([]string, 0, num)
		for _, item := range items {
			results = append(results, item.path)
			if len(results) >= num {
				break
			}
		}
		return results, nil
	})
}

// according to the times of keyword in {content, title, meta}
func NewSearchByContentMatch(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		// TODO

		return nil, nil
	})
}

// searcher according to big language model
func NewSearcherByLLM(fileManager FileManager, hideMatcher, privateMatcher *ignore.GitIgnore) Searcher {
	return SearcherFunc(func(keyword string, num int) ([]string, error) {
		// TODO
		return nil, nil
	})
}

// === meta data for md ===
type Meta struct {
	Title       string   `yaml:"title"`
	KeyWords    []string `yaml:"keywords"`
	Description string   `yaml:"description"`
}

func MdMeta(md []byte) (meta Meta, err error) {
	// 使用正则表达式匹配 md 中 开头的--- ---之间的内容
	re := regexp.MustCompile(`(?s)^\s*---(.*?)---`)
	metaBytes := re.Find(md)
	if err := yaml.Unmarshal(metaBytes, &meta); err != nil {
		return meta, err
	}
	return
}

// === genhtml ===

func GenPath(url string, config *Config) string {
	path := config.GEN_PATH + "/" + url[len(BLOG_ROUTER)+1:]
	if strings.HasSuffix(path, "/") {
		return path + "index.html"
	} else if strings.HasSuffix(path, ".md") {
		return path[:len(path)-len(filepath.Ext(path))] + ".html"
	}
	return path
}
func TransformLinks(oldhtml []byte, config *Config) []byte {
	doc, err := html.Parse(bytes.NewReader(oldhtml))
	if err != nil {
		panic(err)
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for i, attr := range n.Attr {
				if attr.Key == "href" {
					oldHref := attr.Val
					if strings.HasPrefix(oldHref, "/") && strings.HasSuffix(oldHref, ".md") {
						newHref := oldHref[:len(oldHref)-len("md")] + "html"
						n.Attr[i].Val = newHref
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	var buf bytes.Buffer
	html.Render(&buf, doc)
	data := buf.Bytes()
	return data
}

// === path match ===
func PathMatch(path string, matcher ...*ignore.GitIgnore) bool {
	path = SimplifyPath(path)
	for _, m := range matcher {
		if m.MatchesPath(path) {
			return true
		}
	}
	return false
}
