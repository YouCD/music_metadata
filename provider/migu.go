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
	miguBaseURL    = "https://m.music.migu.cn/"
	miguSearchURL  = "http://pd.musicapp.migu.cn/MIGUM2.0/v1.0/content/search_all.do"
	miguLyricURL   = "https://music.migu.cn/v3/api/music/audioPlayer/getLyric"
	miguCoverURL   = "https:%s" // 咪咕封面URL需要补全协议
	miguAndroidUA  = "Android_migu"
	miguAppVersion = "5.0.1"
)

// MiGuProvider 基于咪咕音乐 API 的音乐元数据提供者
type MiGuProvider struct {
	HTTPClient *http.Client
}

// miguSearchRequest 咪咕搜索请求参数
type miguSearchRequest struct {
	Ua           string `json:"ua"`
	Version      string `json:"version"`
	Text         string `json:"text"`
	PageNo       int    `json:"pageNo"`
	PageSize     int    `json:"pageSize"`
	SearchSwitch string `json:"searchSwitch"`
}

// miguSearchResponse 咪咕搜索响应结构
type miguSearchResponse struct {
	SongResultData struct {
		Result []miguSong `json:"result"`
	} `json:"songResultData"`
}

// miguSong 咪咕歌曲结构
type miguSong struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Singers     []miguSinger     `json:"singers"`
	Albums      []miguAlbum      `json:"albums"`
	ImgItems    []miguImage      `json:"imgItems"`
	LyricUrl    string           `json:"lyricUrl"`
	TrcUrl      string           `json:"trcUrl"`
	ContentId   string           `json:"contentId"`
	RateFormats []miguRateFormat `json:"rateFormats"`
}

// miguSinger 咪咕歌手结构
type miguSinger struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// miguAlbum 咪咕专辑结构
type miguAlbum struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// miguImage 咪咕图片结构
type miguImage struct {
	Img string `json:"img"`
}

// miguRateFormat 咪咕音质格式结构
type miguRateFormat struct {
	FormatType   string `json:"formatType"`
	ResourceType string `json:"resourceType"`
	Size         string `json:"size"`
	FileType     string `json:"fileType"`
}

// miguLyricResponse 咪咕歌词响应结构
type miguLyricResponse struct {
	Code  string `json:"code"`
	Lyric string `json:"lyric"`
}

// NewMiGuProvider 创建咪咕音乐API提供者
// 注意: 咪咕音乐使用HTTP协议 (pd.musicapp.migu.cn)，某些网络环境可能无法访问
func NewMiGuProvider() *MiGuProvider {
	return &MiGuProvider{
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Search 搜索歌曲
// 使用咪咕音乐APP API: http://pd.musicapp.migu.cn/MIGUM2.0/v1.0/content/search_all.do
func (p *MiGuProvider) Search(ctx context.Context, keyword string) ([]SongInfo, error) {
	// 构建URL查询参数
	searchSwitch := `{"song":1,"album":0,"singer":0,"tagSong":0,"mvSong":0,"songlist":0,"bestShow":1}`
	apiURL := fmt.Sprintf("%s?ua=%s&version=%s&text=%s&pageNo=1&pageSize=10&searchSwitch=%s",
		miguSearchURL,
		miguAndroidUA,
		miguAppVersion,
		url.QueryEscape(keyword),
		url.QueryEscape(searchSwitch))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

	// 设置请求头 - 使用Android咪咕APP的User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; MI 9 Build/QKQ1.190825.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/83.0.4103.106 Mobile Safari/537.36")
	req.Header.Set("Referer", "http://music.migu.cn/")
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

	var searchResp miguSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	result := make([]SongInfo, 0, len(searchResp.SongResultData.Result))
	for _, song := range searchResp.SongResultData.Result {
		songInfo := p.convertToSongInfo(song)
		result = append(result, songInfo)
	}

	return result, nil
}

// GetLyrics 获取歌词内容
func (p *MiGuProvider) GetLyrics(ctx context.Context, song SongInfo) (string, error) {
	// 咪咕歌词URL可能在搜索结果中已经提供
	if song.LrcURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, song.LrcURL, nil)
		if err != nil {
			return "", fmt.Errorf("创建歌词请求失败: %w", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; MI 9 Build/QKQ1.190825.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/83.0.4103.106 Mobile Safari/537.36")
		req.Header.Set("Referer", "http://music.migu.cn/")

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

	return "", fmt.Errorf("歌曲未提供歌词URL")
}

// GetCover 获取封面图片数据
func (p *MiGuProvider) GetCover(ctx context.Context, song SongInfo) ([]byte, string, error) {
	if song.PicURL == "" {
		return nil, "", nil
	}

	// 咪咕封面URL可能需要补全协议
	coverURL := song.PicURL
	if strings.HasPrefix(coverURL, "//") {
		coverURL = "https:" + coverURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建封面下载请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; MI 9 Build/QKQ1.190825.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/83.0.4103.106 Mobile Safari/537.36")
	req.Header.Set("Referer", "http://music.migu.cn/")

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
func (p *MiGuProvider) Name() string {
	return "migu"
}

// convertToSongInfo 将咪咕歌曲转换为统一的SongInfo结构
func (p *MiGuProvider) convertToSongInfo(song miguSong) SongInfo {
	// 拼接歌手名
	singerNames := make([]string, 0, len(song.Singers))
	for _, s := range song.Singers {
		singerNames = append(singerNames, s.Name)
	}
	artist := strings.Join(singerNames, "、")

	// 获取专辑名
	albumName := ""
	if len(song.Albums) > 0 {
		albumName = song.Albums[0].Name
	}

	// 获取封面URL
	coverURL := ""
	if len(song.ImgItems) > 0 {
		coverURL = song.ImgItems[0].Img
	}

	// 获取歌词URL
	lyricURL := song.LyricUrl
	if lyricURL == "" {
		lyricURL = song.TrcUrl
	}

	return SongInfo{
		Title:  song.Name,
		Artist: artist,
		Album:  albumName,
		Date:   "", // 咪咕API不直接提供发布日期
		SongID: song.ContentId,
		PicURL: coverURL,
		LrcURL: lyricURL,
	}
}
