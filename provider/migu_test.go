package provider

import (
	"context"
	"fmt"
	"testing"
)

func TestMiGuProvider_Search(t *testing.T) {
	provider := NewMiGuProvider()

	ctx := context.Background()
	songs, err := provider.Search(ctx, "周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	fmt.Printf("找到 %d 首歌曲\n", len(songs))
	for i, song := range songs {
		fmt.Printf("[%d] %s - %s (%s)\n", i+1, song.Title, song.Artist, song.Album)
		if i >= 5 { // 只显示前5个结果
			break
		}
	}
}

func TestMiGuProvider_GetLyrics(t *testing.T) {
	provider := NewMiGuProvider()

	ctx := context.Background()
	// 先搜索一首歌
	songs, err := provider.Search(ctx, "晴天 周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		t.Fatal("未找到歌曲")
	}

	// 获取第一首歌的歌词
	lyrics, err := provider.GetLyrics(ctx, songs[0])
	if err != nil {
		t.Fatalf("获取歌词失败: %v", err)
	}

	fmt.Printf("歌曲: %s - %s\n", songs[0].Title, songs[0].Artist)
	fmt.Printf("歌词长度: %d 字符\n", len(lyrics))
	if len(lyrics) > 200 {
		fmt.Printf("歌词预览: %s...\n", lyrics[:200])
	} else {
		fmt.Printf("歌词: %s\n", lyrics)
	}
}

func TestMiGuProvider_GetCover(t *testing.T) {
	provider := NewMiGuProvider()

	ctx := context.Background()
	// 先搜索一首歌
	songs, err := provider.Search(ctx, "晴天 周杰伦")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		t.Fatal("未找到歌曲")
	}

	// 获取第一首歌的封面
	data, contentType, err := provider.GetCover(ctx, songs[0])
	if err != nil {
		t.Fatalf("获取封面失败: %v", err)
	}

	fmt.Printf("歌曲: %s - %s\n", songs[0].Title, songs[0].Artist)
	fmt.Printf("封面大小: %d 字节\n", len(data))
	fmt.Printf("内容类型: %s\n", contentType)
}

func TestMiGuProvider_Name(t *testing.T) {
	provider := NewMiGuProvider()

	name := provider.Name()
	expected := "migu"

	if name != expected {
		t.Errorf("期望提供者名称为 %s, 实际为 %s", expected, name)
	}

	fmt.Printf("提供者名称: %s\n", name)
}
