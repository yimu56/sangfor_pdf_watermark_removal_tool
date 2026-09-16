// wm_remover - 清除 PDF 中的平铺文本水印（深信服技术支持水印等同类）
//
// 用法:
//   wm_remover -i input.pdf                  # 输出到 <input>_clean.pdf
//   wm_remover -i input.pdf -o out.pdf       # 自定义输出
//   wm_remover file1.pdf file2.pdf ...       # 批量处理（自动生成 _clean.pdf）
//   wm_remover --gui                         # 启动图形界面
//
// 原理：扫描所有 Form XObject，找到满足"平铺 + 倾斜 + 字号小 + 大量重复文本"
// 特征的对象，定位为水印后清空其内容流，再原样写回；其余对象零修改。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const version = "1.0.0"

func main() {
	var (
		input    = flag.String("i", "", "输入 PDF 路径")
		output   = flag.String("o", "", "输出 PDF 路径（可选）")
		quiet    = flag.Bool("q", false, "静默模式（仅输出错误）")
		noBackup = flag.Bool("no-backup", false, "不生成 .bak 备份文件")
		guiFlag  = flag.Bool("gui", false, "启动图形界面")
		showVer  = flag.Bool("v", false, "显示版本")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVer {
		fmt.Printf("wm_remover %s (pdfcpu-based)\n", version)
		return
	}

	logf := func(format string, a ...any) {
		if !*quiet {
			fmt.Printf(format+"\n", a...)
		}
	}

	args := flag.Args()
	if *guiFlag {
		if err := runGUI(); err != nil {
			fmt.Fprintf(os.Stderr, "无法启动图形界面: %v\n", err)
			os.Exit(2)
		}
		return
	}
	if *input != "" {
		out := *output
		if out == "" {
			out = defaultOutput(*input)
		}
		if err := processFile(*input, out, *noBackup, logf); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(args) > 0 {
		failures := 0
		for _, p := range args {
			if err := processFile(p, defaultOutput(p), *noBackup, logf); err != nil {
				fmt.Fprintf(os.Stderr, "错误: %s: %v\n", p, err)
				failures++
			}
		}
		if failures > 0 {
			os.Exit(1)
		}
		return
	}

	// 无参数：尝试启动 GUI
	if err := runGUI(); err != nil {
		fmt.Fprintf(os.Stderr, "无法启动图形界面: %v\n", err)
		fmt.Fprintln(os.Stderr, "")
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `wm_remover %s - 平铺文本水印清除工具

用法:
  wm_remover -i <input.pdf> [-o <output.pdf>]    处理单个文件
  wm_remover <file1.pdf> [file2.pdf ...]         批量处理（拖入或命令行）
  wm_remover --gui                                启动图形界面
  wm_remover -v                                   显示版本

选项:
  -i string        输入 PDF 路径
  -o string        输出 PDF 路径（默认 <input>_clean.pdf）
  -q               静默模式
  -no-backup       不生成 .bak 备份
`, version)
}

func defaultOutput(in string) string {
	dir := filepath.Dir(in)
	base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	ext := filepath.Ext(in)
	return filepath.Join(dir, base+"_clean"+ext)
}

type logFn func(string, ...any)

func processFile(src, dst string, noBackup bool, logf logFn) error {
	logf("[%s] 处理: %s", time.Now().Format("15:04:05"), src)

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("文件不存在: %s", src)
	}

	if !noBackup && src != dst {
		backup := src + ".bak"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			if err := copyFile(src, backup); err != nil {
				logf("  警告: 备份失败: %v", err)
			}
		}
	}

	ctx, err := api.ReadContextFile(src)
	if err != nil {
		return fmt.Errorf("读取失败: %w", err)
	}

	logf("  扫描水印...")
	xref, info, err := detectWatermark(ctx)
	if err != nil {
		return err
	}
	if xref < 0 {
		logf("  未检测到水印，跳过。")
		if src != dst {
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("复制失败: %w", err)
			}
		}
		return nil
	}
	logf("  定位水印: xref=%d, %s", xref, info)

	logf("  清除水印流...")
	if err := clearStream(ctx, xref); err != nil {
		return fmt.Errorf("清除失败: %w", err)
	}

	logf("  写入: %s", dst)
	if err := api.WriteContextFile(ctx, dst); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	logf("  验证输出...")
	if err := validate(src, dst); err != nil {
		return fmt.Errorf("验证失败: %w", err)
	}

	logf("  完成: %s", dst)
	return nil
}

func detectWatermark(ctx *model.Context) (int, string, error) {
	type candidate struct {
		xref  int
		score int
		info  string
	}
	var candidates []candidate

	for objNr := range ctx.XRefTable.Table {
		entry, ok := ctx.XRefTable.Find(objNr)
		if !ok || entry == nil || entry.Object == nil {
			continue
		}
		// pdfcpu stores StreamDict as a value, not a pointer
		sd, ok := entry.Object.(types.StreamDict)
		if !ok {
			continue
		}
		t := sd.Type()
		if t == nil || *t != "XObject" {
			continue
		}
		st := sd.Subtype()
		if st == nil || *st != "Form" {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}
		score, info := scoreContent(sd.Content)
		if score > 0 {
			candidates = append(candidates, candidate{objNr, score, info})
		}
	}

	if len(candidates) == 0 {
		return -1, "", nil
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}
	return best.xref, best.info, nil
}

// scoreContent 给一个流的内容打分，判断是否是水印
// 水印特征:
//  1. Form XObject 内有大量 BT...ET 文本块（每个水印一份）
//  2. 含 cm/Tm 矩阵带旋转（|b|>0.1）
//  3. 字号小（<20）
//  4. 大量 q...Q 保存/恢复对
// 返回 (score, info); score=0 表示不像水印
func scoreContent(content []byte) (int, string) {
	if len(content) == 0 {
		return 0, ""
	}
	s := string(content)

	btCount := countToken(s, "BT")
	if btCount < 6 {
		return 0, ""
	}

	score := btCount * 5
	reasons := []string{fmt.Sprintf("%d text blocks", btCount)}

	if hasRotation(s) {
		score += 100
		reasons = append(reasons, "tilted")
	}
	if hasSmallFont(s) {
		score += 60
		reasons = append(reasons, "small font")
	}
	qCount := countToken(s, "q")
	if qCount >= btCount {
		score += 40
		reasons = append(reasons, fmt.Sprintf("%d q/Q pairs", qCount))
	}

	return score, strings.Join(reasons, ", ")
}

// countToken 计数 PDF 操作符出现的次数（要求前导为空白或行首）
func countToken(s, tok string) int {
	n := 0
	for i := 0; i+len(tok) <= len(s); {
		j := strings.Index(s[i:], tok)
		if j < 0 {
			break
		}
		pos := i + j
		if pos == 0 || isWS(s[pos-1]) {
			n++
			i = pos + len(tok)
		} else {
			i = pos + 1
		}
	}
	return n
}

func isWS(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == 0
}

// hasRotation 检查 cm 或 Tm 矩阵中是否带旋转
func hasRotation(s string) bool {
	// 先看 cm
	idx := 0
	for {
		j := strings.Index(s[idx:], " cm")
		if j < 0 {
			break
		}
		pos := idx + j
		if isRotationMatrix(s[:pos]) {
			return true
		}
		idx = pos + 3
	}
	// 再看 Tm
	idx = 0
	for {
		j := strings.Index(s[idx:], " Tm")
		if j < 0 {
			break
		}
		pos := idx + j
		if isRotationMatrix(s[:pos]) {
			return true
		}
		idx = pos + 3
	}
	return false
}

func isRotationMatrix(s string) bool {
	start := len(s) - 100
	if start < 0 {
		start = 0
	}
	pre := s[start:]
	fields := strings.Fields(pre)
	if len(fields) < 6 {
		return false
	}
	var nums []float64
	for _, f := range fields {
		var v float64
		if _, err := fmt.Sscanf(f, "%f", &v); err == nil {
			nums = append(nums, v)
		}
	}
	if len(nums) < 6 {
		return false
	}
	a := nums[len(nums)-6]
	b := nums[len(nums)-5]
	c := nums[len(nums)-4]
	d := nums[len(nums)-3]
	asq := a*a + b*b
	if absF(b) > 0.1 && asq > 0.5 && asq < 1.5 {
		return true
	}
	csq := c*c + d*d
	if absF(c) > 0.1 && csq > 0.5 && csq < 1.5 {
		return true
	}
	return false
}

func hasSmallFont(s string) bool {
	idx := 0
	for {
		j := strings.Index(s[idx:], " Tf")
		if j < 0 {
			return false
		}
		pos := idx + j
		start := pos - 40
		if start < 0 {
			start = 0
		}
		pre := strings.TrimRight(s[start:pos], " \t\n\r")
		fields := strings.Fields(pre)
		if len(fields) >= 1 {
			last := fields[len(fields)-1]
			var f float64
			if _, err := fmt.Sscanf(last, "%f", &f); err == nil {
				if f > 0 && f < 20 {
					return true
				}
			}
		}
		idx = pos + 3
	}
}

func clearStream(ctx *model.Context, objNr int) error {
	entry, ok := ctx.XRefTable.Find(objNr)
	if !ok {
		return fmt.Errorf("xref %d not found", objNr)
	}
	sd, ok := entry.Object.(types.StreamDict)
	if !ok {
		return fmt.Errorf("xref %d is not a stream", objNr)
	}
	// 保留过滤器，把内容替换成一段"什么都不画"的合法内容流，
	// 然后用 Encode() 重新压缩，确保写回后流仍然合法（避免语法错误）。
	sd.Content = []byte("q\nQ\n")
	if err := sd.Encode(); err != nil {
		// 若编码失败，退回"无过滤器+原始内容"
		delete(sd.Dict, "Filter")
		delete(sd.Dict, "DecodeParms")
		sd.Content = []byte("q\nQ\n")
	}
	entry.Object = sd
	return nil
}

func validate(src, dst string) error {
	ctx, err := api.ReadContextFile(dst)
	if err != nil {
		return err
	}
	srcSigs := extractWatermarkSignatures(src)
	if len(srcSigs) == 0 {
		return nil
	}
	for objNr := range ctx.XRefTable.Table {
		entry, ok := ctx.XRefTable.Find(objNr)
		if !ok || entry == nil || entry.Object == nil {
			continue
		}
		sd, ok := entry.Object.(types.StreamDict)
		if !ok {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}
		cs := string(sd.Content)
		for _, sig := range srcSigs {
			if strings.Contains(cs, sig) {
				return fmt.Errorf("xref %d 仍含水印文本 %q", objNr, sig)
			}
		}
	}
	return nil
}

func extractWatermarkSignatures(src string) []string {
	ctx, err := api.ReadContextFile(src)
	if err != nil {
		return nil
	}
	var sigs []string
	seen := map[string]bool{}
	for objNr := range ctx.XRefTable.Table {
		entry, ok := ctx.XRefTable.Find(objNr)
		if !ok || entry == nil || entry.Object == nil {
			continue
		}
		sd, ok := entry.Object.(types.StreamDict)
		if !ok {
			continue
		}
		t := sd.Type()
		if t == nil || *t != "XObject" {
			continue
		}
		st := sd.Subtype()
		if st == nil || *st != "Form" {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}
		cs := string(sd.Content)
		for _, m := range extractStringLiterals(cs) {
			runeCount := len([]rune(m))
			if runeCount >= 4 && !seen[m] {
				seen[m] = true
				sigs = append(sigs, m)
			}
		}
	}
	return sigs
}

func extractStringLiterals(s string) []string {
	var out []string
	// (...) 形式
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(':
			if depth == 0 {
				start = i
			}
			depth++
		case ')':
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, s[start+1:i])
				start = -1
			}
		}
	}
	// <...> 形式
	i := 0
	for i < len(s) {
		if s[i] == '<' && (i+1 >= len(s) || s[i+1] != '<') {
			j := strings.IndexByte(s[i+1:], '>')
			if j < 0 {
				break
			}
			hex := s[i+1 : i+1+j]
			if decoded := decodeHex(hex); decoded != "" {
				out = append(out, decoded)
			}
			i += j + 2
		} else {
			i++
		}
	}
	return out
}

func decodeHex(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return -1
		}
		return r
	}, s)
	if len(s)%2 != 0 {
		s = s[:len(s)-1]
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi := hexNibble(s[i])
		lo := hexNibble(s[i+1])
		if hi < 0 || lo < 0 {
			return ""
		}
		b[i/2] = byte(hi<<4 | lo)
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		b = b[2:]
	}
	runes := decodeUTF16BE(b)
	return string(runes)
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func decodeUTF16BE(b []byte) []rune {
	var out []rune
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(uint16(b[i])<<8 | uint16(b[i+1]))
		if r == 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// runGUI 启动图形界面（仅 Windows）
func runGUI() error {
	return runWalkGUI()
}
