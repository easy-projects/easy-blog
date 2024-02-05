package pkg

import "github.com/blevesearch/bleve"

// 使用bleve 对博客建立索引
type BlogIndexer interface {
	// 加入对一个博客内容的索引
	Add(blog *Blog) error
	// 删除对一个博客内容的索引
	Delete(blog *Blog) error
	// 搜索博客内容
	Search(keyword string, num int) ([]string, error)
	// 把对博客内容建立的索引保存到文件
	Close() error
}
type blogIndexerImpl struct {
	Indexer bleve.Index
}

// 加入对一个博客内容的索引
func (bi *blogIndexerImpl) Add(blog *Blog) error {
	return bi.Indexer.Index(blog.Url, blog)
}

// 删除对一个博客内容的索引
func (bi *blogIndexerImpl) Delete(blog *Blog) error {
	return bi.Indexer.Delete(blog.Url)
}

func NewBlogIndexer(indexPath string) BlogIndexer {
	index, err := bleve.Open("blog.bleve")
	if err != nil {
		mapping := bleve.NewIndexMapping()
		index, err = bleve.New("blog.bleve", mapping)
		if err != nil {
			return nil
		}
	}
	return &blogIndexerImpl{Indexer: index}
}

// 搜索博客内容
func (bi *blogIndexerImpl) Search(keyword string, num int) ([]string, error) {
	query := bleve.NewMatchQuery(keyword)
	search := bleve.NewSearchRequest(query)
	search.Size = num
	searchResults, err := bi.Indexer.Search(search)
	if err != nil {
		return nil, err
	}
	results := make([]string, 0, len(searchResults.Hits))
	for _, hit := range searchResults.Hits {
		results = append(results, hit.ID)
	}
	return results, nil
}

// 把对博客内容建立的索引保存到文件
func (bi *blogIndexerImpl) Close() error {
	return bi.Indexer.Close()
}
