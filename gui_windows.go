//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func runWalkGUI() error {
	var (
		mainWindow *walk.MainWindow
		inputLE    *walk.LineEdit
		outputLE   *walk.LineEdit
		logTE      *walk.TextEdit
		logBuf     *logBuffer
		startBtn   *walk.PushButton
	)
	logBuf = newLogBuffer()

	type fileEntry struct {
		Path string
		Name string
	}

	if err := (MainWindow{
		AssignTo: &mainWindow,
		Title:    fmt.Sprintf("PDF 水印清除工具 v%s", version),
		MinSize:  Size{680, 480},
		Size:     Size{820, 560},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
		Children: []Widget{
			// 标题
			Label{
				Text: "PDF 平铺文本水印清除（深信服技术支持类）",
				Font: Font{PointSize: 11, Bold: true},
			},
			// 输入行
			Composite{
				Layout: HBox{Spacing: 6},
				Children: []Widget{
					Label{Text: "输入 PDF：", MinSize: Size{72, 0}},
					LineEdit{
						AssignTo: &inputLE,
						ReadOnly: true,
					},
					PushButton{
						Text: "选择文档…",
						MinSize: Size{90, 0},
						OnClicked: func() {
							dlg := new(walk.FileDialog)
							dlg.Title = "选择要处理的 PDF"
							dlg.Filter = "PDF 文件 (*.pdf)|*.pdf|所有文件 (*.*)|*.*"
							if ok, err := dlg.ShowOpen(mainWindow); err != nil {
								walk.MsgBox(mainWindow, "错误", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
								return
							} else if !ok {
								return
							}
							inputLE.SetText(dlg.FilePath)
							// 默认输出路径
							if outputLE.Text() == "" {
								dir := filepath.Dir(dlg.FilePath)
								base := strings.TrimSuffix(filepath.Base(dlg.FilePath), filepath.Ext(dlg.FilePath))
								outputLE.SetText(filepath.Join(dir, base+"_clean.pdf"))
							}
							startBtn.SetEnabled(true)
						},
					},
				},
			},
			// 输出行
			Composite{
				Layout: HBox{Spacing: 6},
				Children: []Widget{
					Label{Text: "输出 PDF：", MinSize: Size{72, 0}},
					LineEdit{
						AssignTo: &outputLE,
					},
				},
			},
			// 操作行
			Composite{
				Layout: HBox{Spacing: 6},
				Children: []Widget{
					PushButton{
						AssignTo: &startBtn,
						Text:     "开始处理",
						MinSize:  Size{110, 30},
						Enabled:  false,
						OnClicked: func() {
							src := inputLE.Text()
							dst := outputLE.Text()
							if src == "" {
								walk.MsgBox(mainWindow, "提示", "请先选择要处理的 PDF。", walk.MsgBoxOK|walk.MsgBoxIconInformation)
								return
							}
							if dst == "" {
								dir := filepath.Dir(src)
								base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
								dst = filepath.Join(dir, base+"_clean.pdf")
								outputLE.SetText(dst)
							}
							startBtn.SetEnabled(false)
							go func() {
								defer func() {
									mainWindow.Synchronize(func() { startBtn.SetEnabled(true) })
								}()
								err := processFile(src, dst, true, logBuf.appendf)
								mainWindow.Synchronize(func() {
									if err != nil {
										walk.MsgBox(mainWindow, "失败", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
									} else {
										walk.MsgBox(mainWindow, "完成", "水印已清除。\n\n输出文件："+dst, walk.MsgBoxOK|walk.MsgBoxIconInformation)
									}
								})
							}()
						},
					},
					HSpacer{},
				},
			},
			// 日志
			GroupBox{
				Title:  "处理日志",
				Layout: VBox{Margins: Margins{Left: 4, Top: 4, Right: 4, Bottom: 4}},
				Children: []Widget{
					TextEdit{
						AssignTo: &logTE,
						ReadOnly: true,
						VScroll:  true,
						Text:     "",
					},
				},
			},
		},
	}.Create()); err != nil {
		return err
	}

	// 把 logBuf 写入到 TextEdit
	go func() {
		for line := range logBuf.ch {
			mainWindow.Synchronize(func() {
				logTE.AppendText(line)
			})
		}
	}()

	mainWindow.Run()
	return nil
}

// logBuffer 把日志转发到 GUI 的 TextEdit
type logBuffer struct {
	ch chan string
}

func newLogBuffer() *logBuffer {
	return &logBuffer{ch: make(chan string, 200)}
}

func (l *logBuffer) appendf(format string, a ...any) {
	line := fmt.Sprintf(format+"\n", a...)
	select {
	case l.ch <- line:
	default:
		// 满了，丢最早一条
		select {
		case <-l.ch:
		default:
		}
		l.ch <- line
	}
}
