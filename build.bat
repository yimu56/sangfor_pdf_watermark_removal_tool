@echo off
REM ============================================================
REM  wm_remover - PDF 平铺文本水印清除工具 (Go 版本)
REM  一键重新构建 exe 的脚本
REM ============================================================
setlocal

REM 建议使用本地可用的 Go。如未配置请先安装 Go 1.21+。
where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未找到 Go 工具链，请先安装 Go https://go.dev/dl/
    exit /b 1
)

echo.
echo [1/2] 下载依赖 (pdfcpu, walk) ...
set GOPROXY=https://goproxy.cn,direct
go mod tidy
if errorlevel 1 (
    echo [错误] 依赖下载失败，请检查网络或 Go 代理配置。
    exit /b 1
)

echo.
echo [2/2] 编译 (含界面清单 + 图标) ...
go build -ldflags="-s -w" -o wm_remover.exe .
if errorlevel 1 (
    echo [错误] 编译失败，请查看上方错误信息。
    exit /b 1
)

echo.
echo 构建完成: %cd%\wm_remover.exe
echo.
echo 如需把 exe 放到桌面或 "发送到" 菜单，直接复制该文件即可。
endlocal
