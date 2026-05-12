package main

import (
	"context"
	"fmt"
	"log"

	"github.com/YouCD/music_metadata/provider"
)

func main() {
	// 创建咪咕音乐provider
	miguProvider := provider.NewMiGuProvider()

	ctx := context.Background()

	// 搜索歌曲
	fmt.Println("=== 搜索歌曲 ===")
	songs, err := miguProvider.Search(ctx, "周杰伦 晴天")
	if err != nil {
		log.Fatalf("搜索失败: %v", err)
	}

	fmt.Printf("找到 %d 首歌曲\n\n", len(songs))

	if len(songs) == 0 {
		fmt.Println("未找到歌曲")
		return
	}

	// 显示搜索结果
	for i, song := range songs {
		fmt.Printf("[%d] %s - %s\n", i+1, song.Title, song.Artist)
		fmt.Printf("    专辑: %s\n", song.Album)
		fmt.Printf("    ID: %s\n", song.SongID)
		fmt.Printf("    封面URL: %s\n\n", song.PicURL)

		if i >= 2 { // 只显示前3个结果
			break
		}
	}

	// 获取第一首歌的歌词
	if len(songs) > 0 {
		fmt.Println("=== 获取歌词 ===")
		firstSong := songs[0]
		lyrics, err := miguProvider.GetLyrics(ctx, firstSong)
		if err != nil {
			log.Printf("获取歌词失败: %v", err)
		} else {
			fmt.Printf("歌曲: %s - %s\n", firstSong.Title, firstSong.Artist)
			fmt.Printf("歌词长度: %d 字符\n", len(lyrics))
			if len(lyrics) > 300 {
				fmt.Printf("歌词预览:\n%s...\n", lyrics[:300])
			} else {
				fmt.Printf("歌词:\n%s\n", lyrics)
			}
		}
		fmt.Println()

		// 获取封面
		fmt.Println("=== 获取封面 ===")
		data, contentType, err := miguProvider.GetCover(ctx, firstSong)
		if err != nil {
			log.Printf("获取封面失败: %v", err)
		} else {
			fmt.Printf("歌曲: %s - %s\n", firstSong.Title, firstSong.Artist)
			fmt.Printf("封面大小: %d 字节\n", len(data))
			fmt.Printf("内容类型: %s\n", contentType)
		}
		fmt.Println()
	}

	// 显示provider名称
	fmt.Printf("Provider名称: %s\n", miguProvider.Name())
}
