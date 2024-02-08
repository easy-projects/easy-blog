package pkg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/easy-projects/easyblog/pkg/log"
	"github.com/google/shlex"
	"gopkg.in/yaml.v3"
)

// === meta data for md ===
type Meta struct {
	Title       string   `yaml:"title"`
	KeyWords    []string `yaml:"keywords"`
	Description string   `yaml:"description"`
}
type BlogItem struct {
	// Path 作为唯一标识符
	Path string
	Meta
	Kind int
	File string
	Html string
}

// === blog loader ===
type BlogLoader interface {
	SetBlogPath(blogPath string) BlogLoader
	SetBlogRouter(blogRouter string) BlogLoader
	SetBlogTemplatePath(templatePath string) BlogLoader
	SetRenderCommand(renderCommand string) BlogLoader
	SetHide(hide GitIgnorer) BlogLoader
	SetPrivate(private GitIgnorer) BlogLoader

	GetHide() GitIgnorer
	GetPrivate() GitIgnorer

	Url2Path(url string) string
	Path2Url(path string) string
	LoadBlog(path string) (*BlogItem, error)
}
type blogLoaderImpl struct {
	*sync.RWMutex
	blogPath      string
	blogRouter    string
	templatePath  string
	renderCommand string
	hide          GitIgnorer
	private       GitIgnorer
}

func NewBlogLoader() BlogLoader {
	return &blogLoaderImpl{
		RWMutex: &sync.RWMutex{},
	}
}
func (loader *blogLoaderImpl) SetBlogPath(blogPath string) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.blogPath = blogPath
	return loader
}
func (loader *blogLoaderImpl) SetBlogRouter(blogRouter string) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.blogRouter = blogRouter
	return loader
}
func (loader *blogLoaderImpl) SetBlogTemplatePath(templatePath string) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.templatePath = templatePath
	return loader
}
func (loader *blogLoaderImpl) SetRenderCommand(renderCommand string) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.renderCommand = renderCommand
	return loader
}
func (loader *blogLoaderImpl) SetHide(hide GitIgnorer) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.hide = hide
	return loader
}
func (loader *blogLoaderImpl) SetPrivate(private GitIgnorer) BlogLoader {
	loader.Lock()
	defer loader.Unlock()
	loader.private = private
	return loader
}
func (loader *blogLoaderImpl) LoadBlog(path string) (*BlogItem, error) {
	loader.RLock()
	defer loader.RUnlock()
	var blogRouter, blogPath, templatePath, renderCommand string = loader.blogRouter, loader.blogPath, loader.templatePath, loader.renderCommand
	var hide, private GitIgnorer = loader.hide, loader.private
	path = SimplifyPath(path)
	if !fsutil.IsExist(path) {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	var blogItemType int
	var meta Meta
	var file []byte
	var html []byte
	var err error
	var stat os.FileInfo
	stat, err = os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		file, err = RenderDir(path, hide, private, blogRouter, blogPath)
		blogItemType = BLOG_ITEM_KIND_DIR
	} else if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".markdown") {
		file, err = os.ReadFile(path)
		blogItemType = BLOG_ITEM_KIND_OTHER
	} else {
		file, err = os.ReadFile(path)
		blogItemType = BLOG_ITEM_KIND_MD
	}
	if err != nil {
		return nil, err
	}
	if (blogItemType&BLOG_ITEM_KIND_DIR + blogItemType&BLOG_ITEM_KIND_MD) != 0 {
		if meta, err = MdMeta(file); err != nil {
			return nil, err
		} else if meta.Title == "" {
			meta.Title = filepath.Base(path)
			meta.Title = meta.Title[:len(meta.Title)-len(filepath.Ext(meta.Title))]
		}
		if html, err = Md2Html(file, meta.Title, templatePath, renderCommand); err != nil {
			return nil, err
		}
	} else {
		html = file
	}
	return &BlogItem{
		Path: path,
		Meta: meta,
		Kind: blogItemType,
		File: string(file),
		Html: string(html),
	}, nil
}
func (loader *blogLoaderImpl) Url2Path(url string) string {
	loader.RLock()
	defer loader.RUnlock()
	path := loader.blogPath + "/" + url[len(loader.blogRouter):]
	return SimplifyPath(path)
}
func (loader *blogLoaderImpl) Path2Url(path string) string {
	loader.RLock()
	defer loader.RUnlock()
	return loader.blogRouter + path[len(loader.blogPath):]
}

func (loader *blogLoaderImpl) GetHide() GitIgnorer {
	loader.RLock()
	defer loader.RUnlock()
	return loader.hide
}
func (loader *blogLoaderImpl) GetPrivate() GitIgnorer {
	loader.RLock()
	defer loader.RUnlock()
	return loader.private
}

const (
	BLOG_ITEM_KIND_DIR = 1 << iota
	BLOG_ITEM_KIND_MD
	BLOG_ITEM_KIND_OTHER
)

func (item *BlogItem) IsDir() bool {
	return (item.Kind & BLOG_ITEM_KIND_DIR) != 0
}
func (item *BlogItem) IsMd() bool {
	return (item.Kind & BLOG_ITEM_KIND_MD) != 0
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

// === md2html ===

// use pandoc to convert md to html
func Md2Html(md []byte, title string, templatePath, renderCommand string) (html []byte, err error) {
	// pandoc -s --template=template.html --toc  --mathjax -f markdown -t html --metadata title="title"
	args := []string{"pandoc", "-s", "--template=" + templatePath, "--toc", "--mathjax", "-f", "markdown", "-t", "html", "--metadata", "title=" + title}
	if renderCommand != "" {
		args, err = shlex.Split(renderCommand)
		if err != nil {
			return nil, err
		}
	}
	log.Println("[md2html] render command:", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(md)
	bs, err := cmd.Output()
	return bs, err
}

// load md
func RenderDir(path string, hide, private GitIgnorer, blogRouter, blogPath string) (md []byte, err error) {
	path = SimplifyPath(path)
	log.Println("[load md] path is dir:", path)
	items, err := os.ReadDir(path)
	if err != nil {
		return nil, err
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
		url := blogRouter + full_path[len(blogPath):]
		name = filepath.ToSlash(name)
		dir.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a><br>", url, name))
	}
	md = dir.Bytes()
	return md, err
}
