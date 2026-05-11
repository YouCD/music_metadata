package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultNetEaseBaseURL = "https://api-music.imsyy.com"
)

// NetEaseProvider 基于网易云音乐 API 的音乐元数据提供者
// API 文档参考: https://github.com/Binaryify/NeteaseCloudMusicApi
type NetEaseProvider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// netEaseSearchResponse 网易云搜索 API 响应结构
type netEaseSearchResponse struct {
	Result struct {
		Songs []netEaseSong `json:"songs"`
	} `json:"result"`
	Code int `json:"code"`
}

// netEaseSong 网易云歌曲结构
type netEaseSong struct {
	ID       int             `json:"id"`
	Name     string          `json:"name"`
	Artists  []netEaseArtist `json:"artists"`
	Album    netEaseAlbum    `json:"album"`
	Duration int             `json:"duration"`
}

// netEaseArtist 网易云歌手结构
type netEaseArtist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// netEaseAlbum 网易云专辑结构
type netEaseAlbum struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PicID       int64  `json:"picId"`
	PublishTime int64  `json:"publishTime"`
}

// netEaseLyricResponse 网易云歌词 API 响应结构
type netEaseLyricResponse struct {
	Lrc struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
	Tlyric struct {
		Lyric string `json:"lyric"`
	} `json:"tlyric"`
	Code int `json:"code"`
}

// netEaseSongDetailResponse 网易云歌曲详情 API 响应结构
type netEaseSongDetailResponse struct {
	Songs []struct {
		Al struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			PicURL string `json:"picUrl"`
		} `json:"al"`
	} `json:"songs"`
	Code int `json:"code"`
}

// NewNetEaseProvider 创建网易云音乐 API 提供者
func NewNetEaseProvider(baseURL string) *NetEaseProvider {
	if baseURL == "" {
		baseURL = defaultNetEaseBaseURL
	}
	return &NetEaseProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Search 搜索歌曲
func (p *NetEaseProvider) Search(ctx context.Context, keyword string) ([]SongInfo, error) {
	apiURL := fmt.Sprintf("%s/search?keywords=%s&limit=30", p.BaseURL, url.QueryEscape(keyword))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

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

	var searchResp netEaseSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	if searchResp.Code != 200 {
		return nil, fmt.Errorf("搜索返回错误码: %d", searchResp.Code)
	}

	result := make([]SongInfo, 0, len(searchResp.Result.Songs))
	for _, s := range searchResp.Result.Songs {
		// 拼接歌手名
		artistNames := make([]string, 0, len(s.Artists))
		for _, a := range s.Artists {
			artistNames = append(artistNames, a.Name)
		}
		artist := strings.Join(artistNames, " / ")

		song := SongInfo{
			Title:    s.Name,
			Artist:   artist,
			Album:    s.Album.Name,
			Date:     formatPublishTime(s.Album.PublishTime),
			SongID:   fmt.Sprintf("%d", s.ID),
			Duration: s.Duration,
		}

		result = append(result, song)
	}

	return result, nil
}

// GetLyrics 获取歌词内容
func (p *NetEaseProvider) GetLyrics(ctx context.Context, song SongInfo) (string, error) {
	if song.SongID == "" {
		return "", nil
	}

	apiURL := fmt.Sprintf("%s/lyric?id=%s", p.BaseURL, song.SongID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌词请求失败: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取歌词请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取歌词返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌词响应失败: %w", err)
	}

	var lyricResp netEaseLyricResponse
	if err := json.Unmarshal(body, &lyricResp); err != nil {
		return "", fmt.Errorf("解析歌词失败: %w", err)
	}

	if lyricResp.Code != 200 {
		return "", fmt.Errorf("获取歌词返回错误码: %d", lyricResp.Code)
	}

	lyrics := lyricResp.Lrc.Lyric
	if lyrics == "" {
		return "", nil
	}

	// 如果有翻译歌词，合并到原文后面
	if lyricResp.Tlyric.Lyric != "" {
		lyrics = mergeLyrics(lyricResp.Lrc.Lyric, lyricResp.Tlyric.Lyric)
	}

	return lyrics, nil
}

// GetCover 获取封面图片数据
func (p *NetEaseProvider) GetCover(ctx context.Context, song SongInfo) ([]byte, string, error) {
	if song.SongID == "" {
		return nil, "", nil
	}

	// 先通过歌曲详情接口获取封面 URL
	coverURL, err := p.getCoverURL(ctx, song.SongID)
	if err != nil {
		return nil, "", fmt.Errorf("获取封面 URL 失败: %w", err)
	}

	if coverURL == "" {
		return nil, "", nil
	}

	// 下载封面图片
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建封面下载请求失败: %w", err)
	}

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

// getCoverURL 通过歌曲详情接口获取封面图片 URL
func (p *NetEaseProvider) getCoverURL(ctx context.Context, songID string) (string, error) {
	apiURL := fmt.Sprintf("%s/song/detail?ids=%s", p.BaseURL, songID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌曲详情请求失败: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取歌曲详情请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取歌曲详情返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取歌曲详情响应失败: %w", err)
	}

	var detailResp netEaseSongDetailResponse
	if err := json.Unmarshal(body, &detailResp); err != nil {
		return "", fmt.Errorf("解析歌曲详情失败: %w", err)
	}

	if len(detailResp.Songs) == 0 {
		return "", nil
	}

	return detailResp.Songs[0].Al.PicURL, nil
}

// mergeLyrics 合并原文歌词和翻译歌词
// 将翻译歌词按时间标签合并到原文歌词的下一行
func mergeLyrics(original, translation string) string {
	// 解析翻译歌词为时间标签 -> 歌词的映射
	transMap := parseLrcByTime(translation)

	lines := strings.Split(original, "\n")
	var result strings.Builder

	for _, line := range lines {
		result.WriteString(line)
		result.WriteString("\n")

		// 提取当前行的时间标签
		timestamp := extractTimestamp(line)
		if timestamp != "" {
			if transLine, ok := transMap[timestamp]; ok && transLine != "" {
				result.WriteString(transLine)
				result.WriteString("\n")
			}
		}
	}

	return strings.TrimRight(result.String(), "\n")
}

// parseLrcByTime 将 LRC 歌词解析为 时间标签 -> 歌词 的映射
func parseLrcByTime(lrc string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(lrc, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 提取时间标签 [mm:ss.xx]
		timestamp := extractTimestamp(line)
		if timestamp == "" {
			continue
		}

		// 提取歌词文本（去掉时间标签部分）
		text := removeTimestamps(line)
		if text != "" {
			result[timestamp] = text
		}
	}

	return result
}

// extractTimestamp 从 LRC 行中提取第一个时间标签
func extractTimestamp(line string) string {
	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start == -1 || end == -1 || end <= start {
		return ""
	}

	timestamp := line[start+1 : end]
	// 验证是否为时间格式 mm:ss.xx
	if len(timestamp) >= 5 && timestamp[2] == ':' {
		return timestamp
	}
	return ""
}

// removeTimestamps 移除 LRC 行中的所有时间标签
func removeTimestamps(line string) string {
	var result strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '[' {
			end := strings.Index(line[i:], "]")
			if end != -1 {
				i += end + 1
				continue
			}
		}
		result.WriteByte(line[i])
		i++
	}
	return strings.TrimSpace(result.String())
}

// formatPublishTime 将毫秒时间戳格式化为日期字符串
func formatPublishTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	t := time.Unix(ms/1000, 0)
	return t.Format("2006-01-02")
}

// Name 返回提供者名称
func (p *NetEaseProvider) Name() string {
	return "netease"
}
