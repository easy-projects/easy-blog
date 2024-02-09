
@REM 配置 windows cmd下 功能


echo "using make.cmd"

@echo off
@REM 如果第一个命令行参数是build
if "%1"=="build" (
    call :build
)else if "%1"=="clean" (
    call :clean
)else if "%1"=="install" (
    call :install   
)
goto :eof

@echo off
:preprocess
    echo Preprocessing...
    @REM 构建构建工具
    go build ./cmd/ebbuilder
    @REM 使用构建工具构建 代码资源
    ebbuilder.exe
    del ebbuilder.exe
    echo Done.
    goto :eof

@echo off
:build
    echo Building...
    @REM 预处理
    call :preprocess
    go build ./cmd/eb
    echo Done.
    goto :eof


@echo off
:clean
    echo Cleaning...
    @REM 删除构建产物 eb.exe
    del eb.exe
    del ebcli.exe
    del ebbuilder.exe
    echo Done.
    goto :eof


@echo off
:install
    echo Installing...
    @REM 预处理
    call :preprocess
    @REM 安装项目
    go install ./cmd/eb
    go install ./cmd/ebcli
    go install ./cmd/ebbuilder
    echo Done.
    goto :eof

@echo off
:install-release
    echo Installing...
    @REM 预处理
    call :preprocess
    @REM 安装项目
    go install -ldflags "-s -w" -tags=release ./cmd/eb
    go install -ldflags "-s -w" -tags=release ./cmd/ebcli
    go install -ldflags "-s -w" -tags=release ./cmd/ebbuilder
    echo Done.
    goto :eof
