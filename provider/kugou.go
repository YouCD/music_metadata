package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// 酷狗音乐搜索API (无需签名)
	kugouSearchURL = "http://mobilecdn.kugou.com/api/v3/search/song"
	// 歌词API
	kugouLyricSearchURL   = "http://lyrics.kugou.com/search"
	kugouLyricDownloadURL = "http://lyrics.kugou.com/download"
)

// KugouProvider 基于酷狗音乐 API 的音乐元数据提供者
type KugouProvider struct {
	HTTPClient *http.Client
}

// NewKugouProvider 创建酷狗音乐API提供者
func NewKugouProvider() *KugouProvider {
	return &KugouProvider{
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// kugouSearchResponse 酷狗搜索响应结构
type kugouSearchResponse struct {
	Status int         `json:"status"`
	Error  interface{} `json:"error"` // 可能是int或string
	Data   struct {
		Info []kugouSong `json:"info"`
	} `json:"data"`
}

// kugouSong 酷狗歌曲结构
type kugouSong struct {
	Hash       string `json:"hash"`
	SongName   string `json:"songname"`
	SingerName string `json:"singername"`
	AlbumName  string `json:"album_name"`
	Timelen    int    `json:"timelen"` // 时长(毫秒)
	AlbumInfo  struct {
		Name string `json:"name"`
	} `json:"albuminfo"`
	TransParam struct {
		UnionCover string `json:"union_cover"`
	} `json:"trans_param"`
}

// kugouLyricSearchResponse 歌词搜索响应
type kugouLyricSearchResponse struct {
	Candidates []struct {
		ID        interface{} `json:"id"` // 可能是int或string
		AccessKey string      `json:"accesskey"`
	} `json:"candidates"`
}

// kugouLyricDownloadResponse 歌词下载响应
type kugouLyricDownloadResponse struct {
	Content string `json:"content"` // Base64编码的歌词
}

// Search 搜索歌曲
// 使用酷狗音乐搜索API: http://mobilecdn.kugou.com/api/v3/search/song
func (p *KugouProvider) Search(ctx context.Context, keyword string) ([]SongInfo, error) {
	// 构建搜索参数
	apiURL := fmt.Sprintf("%s?format=json&keyword=%s&showtype=1&page=1&pagesize=10",
		kugouSearchURL, keyword)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("搜索返回状态码: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	var searchResp kugouSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	if searchResp.Status != 1 {
		return nil, fmt.Errorf("搜索API返回错误: status=%d", searchResp.Status)
	}

	result := make([]SongInfo, 0, len(searchResp.Data.Info))
	for _, song := range searchResp.Data.Info {
		// 处理封面URL
		coverURL := song.TransParam.UnionCover
		if coverURL != "" && strings.Contains(coverURL, "{size}") {
			coverURL = strings.ReplaceAll(coverURL, "{size}", "300")
		}

		songInfo := SongInfo{
			Title:  song.SongName,
			Artist: song.SingerName,
			Album:  song.AlbumName,
			Date:   "",
			SongID: song.Hash, // 酷狗使用hash作为ID
			PicURL: coverURL,
			LrcURL: "", // 需要通过歌词API获取
		}

		result = append(result, songInfo)
	}

	return result, nil
}

// GetLyrics 获取歌词内容
func (p *KugouProvider) GetLyrics(ctx context.Context, song SongInfo) (string, error) {
	if song.SongID == "" {
		return "", fmt.Errorf("歌曲ID(hash)为空")
	}

	// 1. 搜索歌词
	searchURL := fmt.Sprintf("%s?keyword=&duration=99999&hash=%s",
		kugouLyricSearchURL, song.SongID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌词搜索请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("歌词搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌词搜索响应失败: %w", err)
	}

	var lyricSearchResp kugouLyricSearchResponse
	if err := json.Unmarshal(body, &lyricSearchResp); err != nil {
		return "", fmt.Errorf("解析歌词搜索结果失败: %w", err)
	}

	if len(lyricSearchResp.Candidates) == 0 {
		return "", fmt.Errorf("未找到歌词")
	}

	// 2. 下载歌词
	candidate := lyricSearchResp.Candidates[0]

	// 转换ID为字符串
	var idStr string
	switch v := candidate.ID.(type) {
	case float64:
		idStr = fmt.Sprintf("%.0f", v)
	case string:
		idStr = v
	default:
		idStr = fmt.Sprintf("%v", v)
	}

	downloadURL := fmt.Sprintf("%s?ver=1&client=pc&id=%s&accesskey=%s&fmt=lrc&charset=utf8",
		kugouLyricDownloadURL, idStr, candidate.AccessKey)

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌词下载请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err = p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("歌词下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌词下载响应失败: %w", err)
	}

	var lyricDownloadResp kugouLyricDownloadResponse
	if err := json.Unmarshal(body, &lyricDownloadResp); err != nil {
		return "", fmt.Errorf("解析歌词下载响应失败: %w", err)
	}

	// 3. 解码Base64歌词
	if lyricDownloadResp.Content == "" {
		return "", fmt.Errorf("歌词内容为空")
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(lyricDownloadResp.Content)
	if err != nil {
		return "", fmt.Errorf("解码歌词失败: %w", err)
	}

	return string(decodedBytes), nil
}

// GetCover 获取封面图片数据
func (p *KugouProvider) GetCover(ctx context.Context, song SongInfo) ([]byte, string, error) {
	if song.PicURL == "" {
		return nil, "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, song.PicURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建封面下载请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("下载封面图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("下载封面图片返回状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取封面图片数据失败: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return data, contentType, nil
}

// Name 返回提供者名称
func (p *KugouProvider) Name() string {
	return "kugou"
}
