package provider

import (
	"context"
	"fmt"
	"testing"
)

func TestKugouProvider_Search(t *testing.T) {
	provider := NewKugouProvider()
	ctx := context.Background()

	songs, err := provider.Search(ctx, "周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		t.Fatal("未找到任何歌曲")
	}

	fmt.Printf("找到 %d 首歌曲\n", len(songs))
	for i, song := range songs {
		fmt.Printf("[%d] %s - %s (%s)\n", i+1, song.Title, song.Artist, song.Album)
		if i >= 5 {
			break
		}
	}
}

func TestKugouProvider_GetLyrics(t *testing.T) {
	provider := NewKugouProvider()
	ctx := context.Background()

	// 先搜索一首歌
	songs, err := provider.Search(ctx, "晴天 周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		t.Fatal("未找到歌曲")
	}

	song := songs[0]
	fmt.Printf("歌曲: %s - %s\n", song.Title, song.Artist)
	fmt.Printf("SongID (Hash): %s\n", song.SongID)

	// 获取歌词
	lyrics, err := provider.GetLyrics(ctx, song)
	if err != nil {
		t.Fatalf("获取歌词失败: %v", err)
	}

	fmt.Printf("歌词长度: %d 字符\n", len(lyrics))
	if len(lyrics) > 200 {
		fmt.Printf("歌词预览: %s...\n", lyrics[:200])
	} else {
		fmt.Printf("歌词预览: %s\n", lyrics)
	}
}

func TestKugouProvider_GetCover(t *testing.T) {
	provider := NewKugouProvider()
	ctx := context.Background()

	// 先搜索一首歌
	songs, err := provider.Search(ctx, "晴天 周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		t.Fatal("未找到歌曲")
	}

	song := songs[0]
	fmt.Printf("歌曲: %s - %s\n", song.Title, song.Artist)

	// 获取封面
	coverData, contentType, err := provider.GetCover(ctx, song)
	if err != nil {
		t.Fatalf("获取封面失败: %v", err)
	}

	fmt.Printf("封面大小: %d 字节\n", len(coverData))
	fmt.Printf("内容类型: %s\n", contentType)
}

func TestKugouProvider_Name(t *testing.T) {
	provider := NewKugouProvider()
	name := provider.Name()

	if name != "kugou" {
		t.Errorf("期望提供者名称为 'kugou', 实际为 '%s'", name)
	}

	fmt.Printf("提供者名称: %s\n", name)
}
