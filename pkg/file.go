package pkg

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cncsmonster/fspider"
)

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
