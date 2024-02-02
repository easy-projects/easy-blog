package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	fsutil "github.com/cncsmonster/gofsutil"
)

func ProcessIncludeFile(includePath string, var_name string, eb *bytes.Buffer) {
	bs, err := os.ReadFile(includePath)
	if err != nil {
		panic(err)
	}
	ProcessIncludeBytes(bs, var_name, eb)
}
func ProcessIncludeString(include string, var_name string, eb *bytes.Buffer) {
	ProcessIncludeBytes([]byte(include), var_name, eb)
}
func ProcessIncludeBytes(include []byte, var_name string, eb *bytes.Buffer) {
	eb.WriteString(fmt.Sprintf("var %s = []byte{", var_name))
	for _, byte := range include {
		eb.WriteString(fmt.Sprintf("%d,", byte))
	}
	eb.WriteString("}\n")
}

func main() {
	// 项目构建工具,
	// 首先,读取本地的 eb.yaml 文件
	var configPath string
	var templatePath string
	var blogPath string
	var hidePath string
	var privatePath string
	var outputPath string
	var helpPath string
	var versionPath string
	var keywordPath string
	var faviconPath string
	flag.StringVar(&configPath, "config", "./build_resources/eb.yaml", "config file path")
	flag.StringVar(&templatePath, "template", "./build_resources/template.yaml", "template file path")
	flag.StringVar(&blogPath, "intro", "./build_resources/intro.md", "intro file path")
	flag.StringVar(&hidePath, "hide", "./build_resources/hide.md", "hide file path")
	flag.StringVar(&privatePath, "private", "./build_resources/private.md", "private file path")
	flag.StringVar(&outputPath, "output", "./include_files.go", "output file path")
	flag.StringVar(&helpPath, "help", "./build_resources/help", "help file path")
	flag.StringVar(&versionPath, "version", "./build_resources/version", "version file path")
	flag.StringVar(&keywordPath, "keyword", "./build_resources/keyword.md", "keyword file path")
	flag.StringVar(&faviconPath, "favicon", "./build_resources/favicon.ico", "favicon file path")
	flag.Parse()
	eb := bytes.NewBufferString("package easyblog\n\n")
	ProcessIncludeFile(configPath, "DEFAULT_CONFIG", eb)
	ProcessIncludeFile(templatePath, "DEFAULT_TEMPLATE", eb)
	ProcessIncludeFile(blogPath, "DEFAULT_BLOG", eb)
	ProcessIncludeFile(hidePath, "DEFAULT_HIDE", eb)
	ProcessIncludeFile(privatePath, "DEFAULT_PRIVATE", eb)
	ProcessIncludeFile(helpPath, "DEFAULT_HELP", eb)
	ProcessIncludeFile(versionPath, "DEFAULT_VERSION", eb)
	ProcessIncludeFile(keywordPath, "DEFAULT_KEYWORD", eb)
	ProcessIncludeFile(faviconPath, "DEFAULT_FAVICON", eb)
	fsutil.MustWrite(outputPath, eb.Bytes())
}
