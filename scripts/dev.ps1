# Yearning 本地开发辅助脚本（Windows PowerShell）
# 用法：
#   .\scripts\dev.ps1 prepare    # 生成 conf.toml、补齐前端 embed 占位、拉取依赖
#   .\scripts\dev.ps1 install    # 初始化数据库（install）
#   .\scripts\dev.ps1 run        # 启动开发服务（默认端口 8000）
#   .\scripts\dev.ps1 run -p 9000

param(
    [Parameter(Position = 0)]
    [ValidateSet("prepare", "install", "run")]
    [string]$Action = "run",

    [string]$Config = "conf.toml",
    [int]$Port = 8000
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Assert-Go {
    try { go version | Out-Null } catch {
        Write-Host "[X] 未找到 go，请先安装 Go 1.22+ 并加入 PATH。" -ForegroundColor Red
        exit 1
    }
}

function Set-GoChinaMirror {
    # 统一使用国内 Go 依赖加速源（写入本机 go env，全局生效）
    go env -w GOPROXY=https://goproxy.cn,direct
    go env -w GOSUMDB=sum.golang.google.cn
    Write-Host "  [mirror] Go 依赖源：GOPROXY=https://goproxy.cn,direct（国内加速）" -ForegroundColor Green
}

function Write-Step($msg) {
    Write-Host "==> $msg" -ForegroundColor Cyan
}

function New-EmbedPlaceholder {
    # 无前端产物时放入最小占位，让 go:embed 可编译（页面为空壳）
    $dist = "src/service/dist"
    $chat = "src/service/chat/server/app"
    if (-not (Test-Path "$dist/index.html")) {
        New-Item -ItemType Directory -Force -Path $dist | Out-Null
        Set-Content -Path "$dist/index.html" -Encoding UTF8 "<!DOCTYPE html><html><head><title>Yearning</title></head><body><div id=""app""></div></body></html>"
        Write-Host "  [embed] 已生成 $dist/index.html（占位，接真实前端产物后删除）"
    }
    if (-not (Test-Path "$chat/index.html")) {
        New-Item -ItemType Directory -Force -Path $chat | Out-Null
        Set-Content -Path "$chat/index.html" -Encoding UTF8 "<!DOCTYPE html><html><head><title>Yearning Chat</title></head><body><div id=""app""></div></body></html>"
        Write-Host "  [embed] 已生成 $chat/index.html（占位，接真实前端产物后删除）"
    }
}

function Invoke-Prepare {
    Assert-Go
    if (-not (Test-Path $Config)) {
        Write-Step "生成配置文件 $Config"
        Copy-Item "conf.toml.template" $Config
        Write-Host "  [warn] 请编辑 $Config 并替换 [General].SecretKey 为随机串！" -ForegroundColor Yellow
        Write-Host "         生成：openssl rand -base64 32"
    } else {
        Write-Step "配置文件已存在：$Config"
    }
    Write-Step "补齐前端 embed 占位"
    New-EmbedPlaceholder
    Write-Step "统一 Go 依赖国内加速源"
    Set-GoChinaMirror
    Write-Step "拉取依赖 go mod tidy"
    go mod tidy
    Write-Host "完成。下一步：.\scripts\dev.ps1 install" -ForegroundColor Green
}

function Invoke-Install {
    Assert-Go
    if (-not (Test-Path $Config)) {
        Write-Host "[X] 缺少 $Config，请先运行 .\scripts\dev.ps1 prepare" -ForegroundColor Red
        exit 1
    }
    Write-Step "初始化数据库"
    go run . install -c $Config
}

function Invoke-Run {
    Assert-Go
    if (-not (Test-Path $Config)) {
        Write-Host "[X] 缺少 $Config，请先运行 .\scripts\dev.ps1 prepare" -ForegroundColor Red
        exit 1
    }
    if (-not (Test-Path "src/service/dist/index.html")) {
        Write-Host "[warn] 缺少前端 embed 产物，将使用占位（页面为空壳）。详见 docs/DEVELOPMENT.md" -ForegroundColor Yellow
        New-EmbedPlaceholder
    }
    Write-Step "启动服务 (port=$Port, config=$Config)"
    go run . run -c $Config -p $Port
}

switch ($Action) {
    "prepare" { Invoke-Prepare }
    "install" { Invoke-Install }
    "run"     { Invoke-Run }
}
