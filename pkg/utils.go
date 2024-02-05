package pkg

import (
	"bytes"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// === path process ===
func SimplifyPath(path string) string {
	path = filepath.Clean(path)
	path = filepath.ToSlash(path)
	return path
}

// === path match ===
func PathMatch(path string, matcher ...GitIgnorer) bool {
	path = SimplifyPath(path)
	for _, m := range matcher {
		if m.Match(path) {
			return true
		}
	}
	return false
}

// === path generate ===
func GenPath(url string, config *Config) string {
	path := config.GEN_PATH + "/" + url[len(config.BLOG_ROUTER)+1:]
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
