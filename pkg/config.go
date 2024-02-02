package pkg

import (
	"log"
	"os"
	"sync"

	fsutil "github.com/cncsmonster/gofsutil"
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
	HIDE_PATHS     []string
	PRIVATE_PATHS  []string
	TEMPLATE_PATH  string
	APP_DATA_PATH  string
	SEARCH_NUM     int
	SEARCH_PLUGINS [][2]string
	RENDER_COMMAND string
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
	if !fsutil.IsExist(config.GEN_PATH) {
		log.Println("[config] gen path not exist, create it")
	}
	if !fsutil.IsExist(config.APP_DATA_PATH) {
		log.Println("[config] app data path not exist, create it")
	}
	if !fsutil.IsExist(config.TEMPLATE_PATH) {
		log.Println("[config] template path not exist, create it")
	}
	return &config
}
