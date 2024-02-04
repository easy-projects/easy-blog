package pkg

import (
	"os"
	"sync"

	fsutil "github.com/cncsmonster/gofsutil"
	"github.com/easy-projects/easyblog/pkg/log"
	"gopkg.in/yaml.v2"
)

// ====== config =====

const (
	BLOG_ROUTER string = "/blog"
	API_ROUTER  string = "/api"
)

type Config struct {
	sync.RWMutex   `yaml:"-"`
	PORT           int
	BLOG_PATH      string
	GEN_PATH       string
	NOT_GEN        bool
	HIDE_PATHS     []string
	PRIVATE_PATHS  []string
	TEMPLATE_PATH  string
	APP_DATA_PATH  string
	SEARCH_NUM     int
	SEARCH_PLUGINS []SearcherPlugin
	RENDER_COMMAND string

	// for visit limit
	RATE_LIMITE_SECOND int
	RATE_LIMITE_MINUTE int
	RATE_LIMITE_HOUR   int
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
	if config.RATE_LIMITE_SECOND == 0 {
		config.RATE_LIMITE_SECOND = 5
	}
	if config.RATE_LIMITE_MINUTE == 0 {
		config.RATE_LIMITE_MINUTE = 30
	}
	if config.RATE_LIMITE_HOUR == 0 {
		config.RATE_LIMITE_HOUR = 1000
	}
	if !fsutil.IsExist(config.BLOG_PATH) {
		log.Fatal("[config] blog path not exist")
	}
	if !fsutil.IsExist(config.APP_DATA_PATH) {
		log.Println("[config] app data path not exist, create it")
	}
	if !fsutil.IsExist(config.TEMPLATE_PATH) {
		log.Println("[config] template path not exist, create it")
	}
	return &config
}
