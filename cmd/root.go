package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/youcd/toolkit/log"
)

var (
	// 全局标志
	apiBase      string
	providerName string
	dryRun       bool
	forceUpdate  bool
)

// ANSI 颜色码
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

var rootCmd = &cobra.Command{
	Use:   "github.com/YouCD/music_metadata",
	Short: "🎵 音乐元数据补全工具",
	Long: fmt.Sprintf("%s🎵 音乐元数据补全工具%s\n\n"+
		"自动抓取并补全音乐文件的歌词、封面等元数据。\n"+
		"支持 MP3、FLAC、M4A、OGG、WAV、APE、AIFF 等格式。\n\n"+
		"%s支持的提供者:%s netease, qqmusic, migu, baidu, kugou\n"+
		"%s提示:%s 使用 --provider 指定数据源，--api 指定自定义 API 地址\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
		ColorYellow, ColorReset,
	),
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute 执行根命令
func Execute() {
	// 初始化日志
	log.Init(nil)

	if err := rootCmd.Execute(); err != nil {
		log.WithCtx(rootCmd.Context()).Error(err)
	}
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVarP(&providerName, "provider", "p", "netease", "元数据提供者 (netease)")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api", "", "API 地址（留空使用提供者默认地址）")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "仅显示信息，不修改文件")
	rootCmd.PersistentFlags().BoolVarP(&forceUpdate, "force", "f", false, "强制更新已有元数据")
}
