package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// 全局标志
	apiBase     string
	server      string
	secretKey   string
	dryRun      bool
	forceUpdate bool
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
	Use:   "music_metadata",
	Short: "🎵 音乐元数据补全工具",
	Long: fmt.Sprintf("%s🎵 音乐元数据补全工具%s\n\n"+
		"通过 Meting API 自动抓取并补全音乐文件的歌词、封面等元数据。\n"+
		"支持 MP3、FLAC、M4A、OGG、WAV、APE、AIFF 等格式。\n\n"+
		"%s支持的音乐平台:%s netease / tencent / kugou / baidu / kuwo\n"+
		"%sAPI 地址:%s https://api.i-meto.com/meting\n",
		ColorBold, ColorReset,
		ColorCyan, ColorReset,
		ColorCyan, ColorReset,
	),
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute 执行根命令
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误: %v%s\n", ColorRed, err, ColorReset)
		os.Exit(1)
	}
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVar(&apiBase, "api", "https://api.i-meto.com/meting/api", "Meting API 地址")
	rootCmd.PersistentFlags().StringVarP(&server, "server", "s", "netease", "音乐平台 (netease/tencent/kugou/baidu/kuwo)")
	rootCmd.PersistentFlags().StringVar(&secretKey, "token", "token", "HMAC 签名密钥")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "仅显示信息，不修改文件")
	rootCmd.PersistentFlags().BoolVarP(&forceUpdate, "force", "f", false, "强制更新已有元数据")
}
