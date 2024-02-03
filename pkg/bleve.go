package pkg

import "github.com/blevesearch/bleve"

// 使用bleve 对博客建立索引
type BlogIndex struct {
	Indexer bleve.Index
}
