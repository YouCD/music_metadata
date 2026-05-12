package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	qqMusicSearchURL = "https://u.y.qq.com/cgi-bin/musicu.fcg"
	qqMusicLyricURL  = "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg"
	qqMusicCoverURL  = "http://y.qq.com/music/photo_new/T002R300x300M000%s.jpg"
)

// QQMusicProvider 基于QQ音乐 API 的音乐元数据提供者
type QQMusicProvider struct {
	HTTPClient *http.Client
}

// qqMusicSearchRequest QQ音乐搜索请求结构
type qqMusicSearchRequest struct {
	Comm struct {
		Wid                      string `json:"wid"`
		TmeAppID                 string `json:"tmeAppID"`
		Authst                   string `json:"authst"`
		Uid                      string `json:"uid"`
		Gray                     string `json:"gray"`
		OpenUDID                 string `json:"OpenUDID"`
		Ct                       string `json:"ct"`
		Patch                    string `json:"patch"`
		PsrfQqOpenID             string `json:"psrf_qqopenid"`
		Sid                      string `json:"sid"`
		PsrfAccessTokenExpiresAt string `json:"psrf_access_token_expiresAt"`
		Cv                       string `json:"cv"`
		Gzip                     string `json:"gzip"`
		Qq                       string `json:"qq"`
		NetType                  string `json:"nettype"`
		PsrfQqUnionID            string `json:"psrf_qqunionid"`
		PsrfQqAccessToken        string `json:"psrf_qqaccess_token"`
		TmeLoginType             string `json:"tmeLoginType"`
	} `json:"comm"`
	DoSearchForQQMusicDesktop struct {
		Module string `json:"module"`
		Method string `json:"method"`
		Param  struct {
			NumPerPage  int    `json:"num_per_page"`
			PageNum     int    `json:"page_num"`
			RemotePlace string `json:"remoteplace"`
			SearchType  int    `json:"search_type"`
			Query       string `json:"query"`
			Grp         int    `json:"grp"`
			SearchID    string `json:"searchid"`
			NqcFlag     int    `json:"nqc_flag"`
		} `json:"param"`
	} `json:"music.search.SearchCgiService.DoSearchForQQMusicDesktop"`
}

// qqMusicSearchResponse QQ音乐搜索响应结构
type qqMusicSearchResponse struct {
	MusicSearchCgiService struct {
		Data struct {
			Body struct {
				Song struct {
					List []qqMusicSong `json:"list"`
				} `json:"song"`
			} `json:"body"`
			Meta struct {
				Sum      int `json:"sum"`
				NextPage int `json:"nextpage"`
				CurPage  int `json:"curpage"`
			} `json:"meta"`
		} `json:"data"`
	} `json:"music.search.SearchCgiService.DoSearchForQQMusicDesktop"`
}

// qqMusicSong QQ音乐歌曲结构
type qqMusicSong struct {
	Album struct {
		ID    int    `json:"id"`
		Mid   string `json:"mid"`
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"album"`
	DocID      string          `json:"docid"`
	ID         int             `json:"id"`
	Mid        string          `json:"mid"`
	Title      string          `json:"title"`
	Singer     []qqMusicArtist `json:"singer"`
	TimePublic string          `json:"time_public"`
	File       struct {
		MediaMid   string `json:"media_mid"`
		SizeHires  int64  `json:"size_hires"`
		SizeFlac   int64  `json:"size_flac"`
		Size320mp3 int64  `json:"size_320mp3"`
		Size192ogg int64  `json:"size_192ogg"`
		Size128mp3 int64  `json:"size_128mp3"`
		Size96aac  int64  `json:"size_96aac"`
	} `json:"file"`
}

// qqMusicArtist QQ音乐歌手结构
type qqMusicArtist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Mid  string `json:"mid"`
}

// qqMusicLyricResponse QQ音乐歌词响应结构
type qqMusicLyricResponse struct {
	Code  int    `json:"code"`
	Lyric string `json:"lyric"`
	Trans string `json:"trans"`
}

// NewQQMusicProvider 创建QQ音乐API提供者
func NewQQMusicProvider() *QQMusicProvider {
	return &QQMusicProvider{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Search 搜索歌曲
func (p *QQMusicProvider) Search(ctx context.Context, keyword string) ([]SongInfo, error) {
	searchReq := p.buildSearchRequest(keyword, 1, 15)

	resp, err := p.doSearchRequest(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}

	result := make([]SongInfo, 0, len(resp.MusicSearchCgiService.Data.Body.Song.List))
	for _, song := range resp.MusicSearchCgiService.Data.Body.Song.List {
		songInfo := p.convertToSongInfo(song)
		result = append(result, songInfo)
	}

	return result, nil
}

// GetLyrics 获取歌词内容
func (p *QQMusicProvider) GetLyrics(ctx context.Context, song SongInfo) (string, error) {
	if song.SongID == "" {
		return "", nil
	}

	apiURL := fmt.Sprintf("%s?g_tk=5381&format=json&inCharset=utf-8&outCharset=utf-8&notice=0&platform=h5&needNewCode=1&ct=121&cv=0&songmid=%s",
		qqMusicLyricURL, song.SongID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建歌词请求失败: %w", err)
	}

	req.Header.Set("Referer", "https://y.qq.com")
	req.Header.Set("User-Agent", "QQ音乐/73222 CFNetwork/1406.0.2 Darwin/22.4.0")

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

	var lyricResp qqMusicLyricResponse
	if err := json.Unmarshal(body, &lyricResp); err != nil {
		return "", fmt.Errorf("解析歌词失败: %w", err)
	}

	if lyricResp.Code != 0 {
		return "", fmt.Errorf("获取歌词返回错误码: %d", lyricResp.Code)
	}

	// 合并原文歌词和翻译歌词
	lyrics := lyricResp.Lyric
	if lyrics != "" && lyricResp.Trans != "" {
		lyrics = mergeLyrics(lyricResp.Lyric, lyricResp.Trans)
	}

	return lyrics, nil
}

// GetCover 获取封面图片数据
func (p *QQMusicProvider) GetCover(ctx context.Context, song SongInfo) ([]byte, string, error) {
	if song.PicURL == "" {
		return nil, "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, song.PicURL, nil)
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

// Name 返回提供者名称
func (p *QQMusicProvider) Name() string {
	return "qqmusic"
}

// buildSearchRequest 构建搜索请求
func (p *QQMusicProvider) buildSearchRequest(keyword string, page, size int) qqMusicSearchRequest {
	var req qqMusicSearchRequest

	req.Comm.Wid = ""
	req.Comm.TmeAppID = "qqmusic"
	req.Comm.Authst = ""
	req.Comm.Uid = ""
	req.Comm.Gray = "0"
	req.Comm.OpenUDID = "2d484d3157d4ed482e406e6c5fdcf8c3d3275deb"
	req.Comm.Ct = "6"
	req.Comm.Patch = "2"
	req.Comm.PsrfQqOpenID = ""
	req.Comm.Sid = ""
	req.Comm.PsrfAccessTokenExpiresAt = ""
	req.Comm.Cv = "80600"
	req.Comm.Gzip = "0"
	req.Comm.Qq = ""
	req.Comm.NetType = "2"
	req.Comm.PsrfQqUnionID = ""
	req.Comm.PsrfQqAccessToken = ""
	req.Comm.TmeLoginType = "2"

	req.DoSearchForQQMusicDesktop.Module = "music.search.SearchCgiService"
	req.DoSearchForQQMusicDesktop.Method = "DoSearchForQQMusicDesktop"
	req.DoSearchForQQMusicDesktop.Param.NumPerPage = size
	req.DoSearchForQQMusicDesktop.Param.PageNum = page
	req.DoSearchForQQMusicDesktop.Param.RemotePlace = "txt.mac.search"
	req.DoSearchForQQMusicDesktop.Param.SearchType = 0
	req.DoSearchForQQMusicDesktop.Param.Query = keyword
	req.DoSearchForQQMusicDesktop.Param.Grp = 1
	req.DoSearchForQQMusicDesktop.Param.SearchID = uuid.New().String()
	req.DoSearchForQQMusicDesktop.Param.NqcFlag = 0

	return req
}

// doSearchRequest 执行搜索请求
func (p *QQMusicProvider) doSearchRequest(ctx context.Context, req qqMusicSearchRequest) (*qqMusicSearchResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, qqMusicSearchURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	httpReq.Header.Set("Referer", "https://y.qq.com/portal/profile.html")
	httpReq.Header.Set("User-Agent", "QQ%E9%9F%B3%E4%B9%90/73222 CFNetwork/1406.0.3 Darwin/22.4.0")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("执行HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var searchResp qqMusicSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	return &searchResp, nil
}

// convertToSongInfo 将QQ音乐歌曲转换为统一的SongInfo结构
func (p *QQMusicProvider) convertToSongInfo(song qqMusicSong) SongInfo {
	// 拼接歌手名
	singerNames := make([]string, 0, len(song.Singer))
	for _, s := range song.Singer {
		singerNames = append(singerNames, s.Name)
	}
	artist := strings.Join(singerNames, ",")

	// 处理专辑名
	albumName := strings.TrimSpace(song.Album.Title)
	if albumName == "" {
		albumName = "未分类专辑"
	}

	// 生成封面URL
	coverURL := fmt.Sprintf(qqMusicCoverURL, song.Album.Mid)

	return SongInfo{
		Title:  song.Title,
		Artist: artist,
		Album:  albumName,
		Date:   song.TimePublic,
		SongID: song.Mid,
		PicURL: coverURL,
	}
}
