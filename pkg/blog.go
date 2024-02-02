package pkg

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/google/shlex"
	ignore "github.com/sabhiram/go-gitignore"
	"gopkg.in/yaml.v3"
)

type Blog struct {
	Meta
	Md   []byte
	Html []byte
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
func RenderDir(path string, hide, private *ignore.GitIgnore, config *Config) (md []byte, err error) {
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
		url := BLOG_ROUTER + full_path[len(config.BLOG_PATH):]
		name = filepath.ToSlash(name)
		dir.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a><br>", url, name))
	}
	md = dir.Bytes()
	return md, err
}
