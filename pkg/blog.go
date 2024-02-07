package pkg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/easy-projects/easyblog/pkg/log"
	"github.com/google/shlex"
	"gopkg.in/yaml.v3"
)

type BlogItem struct {
	// Path 作为唯一标识符
	Path string
	Meta
	Md   string
	Html string
}

// === meta data for md ===
type Meta struct {
	Title       string   `yaml:"title"`
	KeyWords    []string `yaml:"keywords"`
	Description string   `yaml:"description"`
}

func LoadBlog(path string, hide, private GitIgnorer, config *Config) (*BlogItem, error) {
	// 从文件中加载 blog
	path = SimplifyPath(path)
	if !fsutil.IsExist(path) {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	var md []byte
	var err error
	var stat os.FileInfo
	stat, err = os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		md, err = RenderDir(path, hide, private, config)
	} else if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".markdown") {
		err = fmt.Errorf("file is not a markdown file: %s", path)
	} else {
		md, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	meta, err := MdMeta(md)
	if err != nil {
		return nil, err
	}
	if meta.Title == "" {
		meta.Title = filepath.Base(path)
		meta.Title = meta.Title[:len(meta.Title)-len(filepath.Ext(meta.Title))]
	}
	html, err := Md2Html(md, meta.Title, config)
	if err != nil {
		return nil, err
	}
	return &BlogItem{
		Path: path,
		Meta: meta,
		Md:   string(md),
		Html: string(html),
	}, nil
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
func Md2Html(md []byte, title string, config *Config) (html []byte, err error) {
	// pandoc -s --template=template.html --toc  --mathjax -f markdown -t html --metadata title="title"
	args := []string{"pandoc", "-s", "--template=" + config.TEMPLATE_PATH, "--toc", "--mathjax", "-f", "markdown", "-t", "html", "--metadata", "title=" + title}
	if config.RENDER_COMMAND != "" {
		args, err = shlex.Split(config.RENDER_COMMAND)
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
func RenderDir(path string, hide, private GitIgnorer, config *Config) (md []byte, err error) {
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
		url := config.BLOG_ROUTER + full_path[len(config.BLOG_PATH):]
		name = filepath.ToSlash(name)
		dir.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a><br>", url, name))
	}
	md = dir.Bytes()
	return md, err
}
