package pkg

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/easy-projects/easyblog/pkg/log"
	"github.com/texttheater/golang-levenshtein/levenshtein"
)

type Searcher interface {
	Search(keyword string, num int) ([]string, error)
	Name() string
	Brief() string
}

type SearcherPlugin struct {
	Name    string
	Brief   string
	Type    string
	Command string
	Url     string
}

// searcherImpl
type searcherImpl struct {
	f     func(keyword string, num int) ([]string, error)
	name  string
	brief string
}

// SearcherFunc implements Searcher interface
func (s searcherImpl) Search(keyword string, num int) ([]string, error) {
	return s.f(keyword, num)
}

func (s searcherImpl) Name() string {
	return s.name
}
func (s searcherImpl) Brief() string {
	return s.brief
}

// searcher according to title edit distance
func NewSearcherByTitleEditDistance(name, brief string, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer) Searcher {
	f := func(keyword string, num int) ([]string, error) {
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
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}

// searcher according to title word2vec
func NewSearcherByTitleWord2Vec(name, brief string, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer) Searcher {
	f := func(keyword string, num int) ([]string, error) {
		// TODO
		return nil, nil
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}

// searcher according to plugin
func NewSearcherByPlugin(plugin SearcherPlugin, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer, config *Config) Searcher {
	f := func(keyword string, num int) ([]string, error) {
		cmd := exec.Command(plugin.Command)
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
	}
	return searcherImpl{
		f:     f,
		name:  plugin.Name,
		brief: plugin.Brief,
	}
}

// searcher according to search-keyword and keywords in meta
func NewSearcherByKeywork(name, brief string, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer) Searcher {
	f := func(keyword string, num int) ([]string, error) {
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
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}

// according to the times of keyword in {content, title, meta}
func NewSearchByContentMatch(name, brief string, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer) Searcher {
	f := func(keyword string, num int) ([]string, error) {
		// TODO

		return nil, nil
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}

// searcher according to bleve file index engine
func NewSearcherByBleve(name, brief string, blogIndexer BlogIndexer, config *Config) Searcher {

	f := func(keyword string, num int) ([]string, error) {
		results, err := blogIndexer.Search(keyword, num)
		if err != nil {
			return nil, err
		}
		return results, nil
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}

// searcher according to big language model
func NewSearcherByLLM(name, brief string, fileManager FileManager, hideMatcher, privateMatcher GitIgnorer) Searcher {
	f := func(keyword string, num int) ([]string, error) {
		// TODO
		return nil, nil
	}
	return searcherImpl{
		f:     f,
		name:  name,
		brief: brief,
	}
}
