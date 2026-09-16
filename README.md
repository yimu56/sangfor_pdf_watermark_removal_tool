# PDF 平铺文本水印清除工具

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8.svg)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-0078D6.svg)](#环境要求)

一个用 **Go** 编写的 Windows 小工具，用于清除 PDF 中**平铺、倾斜、小字号**的文本水印（典型场景：流转文档里反复出现的「技术支持:编号」类水印）。

单文件 exe，**双击即用**，不依赖 Python 或任何运行时。

---

## 特性

- **按特征识别，不写死水印内容** —— 水印里的编号、账号变了照样能清，无需改代码。
- **不伤正文** —— 只清空水印对象，正文、图片、页眉页脚、书签全部原样保留。
- **大文件友好** —— 水印由单个共享对象承载，处理耗时与页数几乎无关，几百页文档也能秒级完成。
- **三种用法** —— 图形界面 / 命令行批量 / 把 PDF 拖到图标上。
- **自动校验** —— 处理完会重新读取输出，确认水印已归零。
- **自动备份** —— 处理前生成 `.bak`，可随时回退。

## 工作原理

这类水印通常**不是**逐页画在内容流里，而是封装成一个 **Form XObject**，再由所有页面通过 `/Fm1 Do` 之类的指令引用。因此：

1. 扫描 PDF 中所有 Form XObject；
2. 用启发式规则判断哪个是水印：
   - 对象内含**大量 `BT` 文本块**（平铺的每个副本一份）；
   - `cm` / `Tm` 变换矩阵带**旋转**（即倾斜，`|b| > 0.1` 且 `a²+b² ≈ 1`）；
   - 文字**字号偏小**（< 20）；
   - 被多页共享引用。
3. 命中后**只清空该对象的内容流**（替换为一段合法的空操作 `q Q`），其余对象零改动；
4. 写回后重新读取输出，校验水印文本已归零。

因为只需修改一个对象，**处理耗时基本不随页数增长**，且写回走原地重写，内存占用低。

## 环境要求

- Windows 10 / 11（64 位）
- 仅**从源码构建**时需要 Go 1.21+；直接使用 exe 无需任何环境

## 快速开始

### 方式一：直接使用 exe（推荐）

从 [Releases](../../releases) 页面下载 `wm_remover.exe`，双击运行：

1. 点「选择文档…」选中要处理的 PDF；
2. 输出路径会自动填好（原文件名加 `_clean` 后缀），可自行修改；
3. 点「开始处理」，日志区会实时显示进度。

### 方式二：从源码构建

在项目目录运行一键脚本：

```bat
build.bat
```

或手动执行：

```bat
set GOPROXY=https://goproxy.cn,direct
go mod tidy
go build -ldflags="-s -w" -o wm_remover.exe .
```

### 命令行 / 批量

```bat
:: 单个文件，输出 input_clean.pdf
wm_remover.exe -i input.pdf

:: 指定输出路径
wm_remover.exe -i input.pdf -o output.pdf

:: 批量处理（命令行多个文件）
wm_remover.exe a.pdf b.pdf c.pdf

:: 静默模式（仅输出错误）
wm_remover.exe -q -i input.pdf
```

> 也可以直接把一个或多个 PDF **拖到 `wm_remover.exe` 图标上**，松手即自动处理。

| 参数 | 说明 |
| --- | --- |
| `-i` | 输入 PDF 路径 |
| `-o` | 输出 PDF 路径（默认 `<输入名>_clean.pdf`） |
| `-q` | 静默模式，仅输出错误 |
| `-no-backup` | 不生成 `.bak` 备份文件 |
| `-gui` | 强制打开图形界面 |
| `-v` | 显示版本 |

不带任何参数运行会打开图形界面。退出码：`0` 成功 / `1` 处理失败 / `2` 参数错误。

## 目录结构

```
.
├─ main.go                  # 核心逻辑：CLI、水印检测 / 清除 / 校验
├─ gui_windows.go           # Windows 图形界面（lxn/walk）
├─ rsrc_windows_amd64.syso  # 界面清单 + 图标资源（编译时自动链接）
├─ build.bat                # 一键构建脚本
├─ winres/
│  └─ icon.png              # 图标源文件
├─ go.mod / go.sum
├─ LICENSE
└─ README.md
```

## 重新生成界面资源（可选）

`rsrc_windows_amd64.syso` 由 [go-winres](https://github.com/tc-hib/go-winres) 生成，内含 Windows 应用清单（comctl32 v6）与图标。**缺少它编译出的 exe 界面会启动失败**（报 `TTM_ADDTOOL failed`），因此请保留该文件。

如需自行重新生成：

```bat
go install github.com/tc-hib/go-winres@latest
go-winres simply --manifest gui --icon winres/icon.png --arch amd64 --no-suffix
ren rsrc rsrc_windows_amd64.syso
```

## 已知限制

- 仅针对**平铺、倾斜、小字号**这一类的**文本**水印；其他形式（如整页半透明大字水印、图片水印、页脚固定水印）不适用。
- 仅支持 Windows。
- 加密或已损坏的 PDF 可能读取失败。

## 免责声明

本工具仅用于清理**你有权修改**的文档（例如你自己流转、归档的内部资料）。请勿用于去除他人作品的版权标识，或用于任何侵权用途。使用本工具产生的后果由使用者自行承担。

## 许可证

[MIT License](LICENSE) © 2026 yimu56

---

## English

A single-file **Go** utility for Windows that removes **tiled, tilted, small-font text watermarks** from PDF files — for example the recurring "technical support + ID" watermarks stamped on circulated documents.

**Highlights**

- **Signature-based detection** — the watermark text/number may change; detection relies on structure, not on hard-coded strings (many `BT` text blocks, rotated `cm`/`Tm` matrices, font size < 20).
- **Non-destructive** — only the watermark `Form XObject` is emptied. Body text, images, headers, footers and bookmarks are left untouched.
- **Fast on large files** — the watermark lives in a single shared object, so runtime is essentially independent of page count.
- **GUI, CLI and drag-and-drop.**

**Build** (Windows, Go 1.21+)

```bat
build.bat
```

or

```bat
go build -ldflags="-s -w" -o wm_remover.exe .
```

**Usage**

```bat
wm_remover.exe -i input.pdf [-o output.pdf]   :: single file
wm_remover.exe a.pdf b.pdf                    :: batch
```

Run with no arguments to open the GUI. You can also drop PDF files directly onto `wm_remover.exe`.

**License** — [MIT](LICENSE) © 2026 yimu56
