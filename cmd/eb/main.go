package main

import (
	"fmt"
	"os"

	eb "github.com/easy-projects/easyblog"
)

func main() {
	// 加入命令行解析,如果有-h参数,打印帮助信息
	// 如果有 -g 参数,则生成静态网页
	// 如果没有参数,则启动服务器
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "-s")
	}
	switch os.Args[1] {
	case "-h":
		eb.Help()
	case "-n":
		eb.New()
	case "-s":
		eb.Serve(eb.LoadConfig("eb.yaml"))
	case "-v":
		eb.Version()
	default:
		fmt.Println("unknown command")
	}
}
