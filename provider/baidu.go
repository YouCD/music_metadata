package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// 千千音乐（原百度音乐）新API
	qianqianSearchURL    = "https://music.91q.com/v1/search"
	qianqianTrackLinkURL = "https://music.91q.com/v1/song/tracklink"
	qianqianAppID        = "16073360"
	qianqianSecret       = "0b50b02fd0d73a9c4c8c3a781c30845f"
)

// BaiduProvider 基于千千音乐 API 的音乐元数据提供者
// 注意: 使用新的千千音乐API (music.91q.com)，需要签名验证
type BaiduProvider struct {
	HTTPClient *http.Client
}

// NewBaiduProvider 创建千千音乐API提供者
// 使用新的API端点: https://music.91q.com
func NewBaiduProvider() *BaiduProvider {
	return &BaiduProvider{
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// qianqianSearchResponse 千千音乐搜索响应结构
type qianqianSearchResponse struct {
	Data struct {
		TypeTrack []qianqianSong `json:"typeTrack"`
	} `json:"data"`
}

// qianqianSong 千千音乐歌曲结构
type qianqianSong struct {
	TSID       string           `json:"TSID"`
	Title      string           `json:"title"`
	Artist     []qianqianArtist `json:"artist"`
	AlbumTitle string           `json:"albumTitle"`
	Pic        string           `json:"pic"`
	Lyric      string           `json:"lyric"`
}

// qianqianArtist 千千音乐歌手结构
type qianqianArtist struct {
	Name string `json:"name"`
}

// qianqianTrackLinkResponse 千千音乐歌曲链接响应
type qianqianTrackLinkResponse struct {
	Data struct {
		Path     string `json:"path"`
		Duration int    `json:"duration"`
	} `json:"data"`
}

// Search 搜索歌曲
// 使用千千音乐API: https://music.91q.com/v1/search
func (p *BaiduProvider) Search(ctx context.Context, keyword string) ([]SongInfo, error) {
	// 构建搜索参数
	params := map[string]string{
		"word":     keyword,
		"type":     "1",
		"pageNo":   "1",
		"pageSize": "10",
		"appid":    qianqianAppID,
	}

	// 添加签名和时间戳
	p.addSignAndTimestamp(params)

	// 构建URL查询参数（需要按key排序，与签名一致）
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	queryParts := make([]string, 0, len(params))
	for _, k := range keys {
		// URL编码参数值
		encodedValue := url.QueryEscape(params[k])
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, encodedValue))
	}
	apiURL := qianqianSearchURL + "?" + strings.Join(queryParts, "&")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Referer", "https://music.91q.com/player")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	req.Header.Set("From", "web")

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

	var searchResp qianqianSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	result := make([]SongInfo, 0, len(searchResp.Data.TypeTrack))
	for _, song := range searchResp.Data.TypeTrack {
		// 拼接歌手名
		artistNames := make([]string, 0, len(song.Artist))
		for _, artist := range song.Artist {
			artistNames = append(artistNames, artist.Name)
		}
		artist := strings.Join(artistNames, ", ")

		songInfo := SongInfo{
			Title:  song.Title,
			Artist: artist,
			Album:  song.AlbumTitle,
			Date:   "",
			SongID: song.TSID,
			PicURL: song.Pic,
			LrcURL: song.Lyric, // 歌词URL
		}

		result = append(result, songInfo)
	}

	return result, nil
}

// GetLyrics 获取歌词内容
func (p *BaiduProvider) GetLyrics(ctx context.Context, song SongInfo) (string, error) {
	if song.LrcURL == "" {
		return "", fmt.Errorf("歌曲未提供歌词URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, song.LrcURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌词请求失败: %w", err)
	}

	req.Header.Set("Referer", "https://music.91q.com/player")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")

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

	return string(body), nil
}

// GetCover 获取封面图片数据
func (p *BaiduProvider) GetCover(ctx context.Context, song SongInfo) ([]byte, string, error) {
	if song.PicURL == "" {
		return nil, "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, song.PicURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建封面下载请求失败: %w", err)
	}

	req.Header.Set("Referer", "https://music.91q.com/player")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")

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

// addSignAndTimestamp 添加签名和时间戳参数
func (p *BaiduProvider) addSignAndTimestamp(params map[string]string) {
	// 添加时间戳
	params["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)

	// 按key排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建签名字符串
	var signStr strings.Builder
	for i, k := range keys {
		if i > 0 {
			signStr.WriteString("&")
		}
		signStr.WriteString(k)
		signStr.WriteString("=")
		signStr.WriteString(params[k])
	}
	signStr.WriteString(qianqianSecret)

	// 计算MD5签名
	hash := md5.Sum([]byte(signStr.String()))
	params["sign"] = hex.EncodeToString(hash[:])
}

// Name 返回提供者名称
func (p *BaiduProvider) Name() string {
	return "baidu"
}
