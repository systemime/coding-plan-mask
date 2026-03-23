// Package storage 提供数据存储和统计功能
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Storage 数据存储
type Storage struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex

	// 统计缓存
	totalRequests     int64
	totalInputTokens  int64
	totalOutputTokens int64
	totalTokens       int64
}

// RequestRecord 请求记录
type RequestRecord struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Stream       bool      `json:"stream"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	ClientIP     string    `json:"client_ip"`
	RequestBody  string    `json:"request_body"`
	ResponseBody string    `json:"response_body"`
	StatusCode   int       `json:"status_code"`
	Duration     float64   `json:"duration_ms"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	Success      bool      `json:"success"`
	ErrorMsg     string    `json:"error_msg"`
}

// RequestRecordLite 轻量级请求记录（用于列表显示）
type RequestRecordLite struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Stream       bool      `json:"stream"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	ClientIP     string    `json:"client_ip"`
	StatusCode   int       `json:"status_code"`
	Duration     float64   `json:"duration_ms"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	Success      bool      `json:"success"`
}

// Stats 统计信息
type Stats struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TodayRequests     int64   `json:"today_requests"`
	TodayInputTokens  int64   `json:"today_input_tokens"`
	TodayOutputTokens int64   `json:"today_output_tokens"`
	TodayTokens       int64   `json:"today_tokens"`
	RequestsPerMin    float64 `json:"requests_per_min"`
	InputPerMin       float64 `json:"input_per_min"`
	OutputPerMin      float64 `json:"output_per_min"`
}

// New 创建存储实例
func New(dataDir string) (*Storage, error) {
	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	dbPath := filepath.Join(dataDir, "proxy.db")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	s := &Storage{
		db:   db,
		path: dbPath,
	}

	// 初始化表结构
	if err := s.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化表结构失败: %w", err)
	}

	// 加载缓存统计
	if err := s.loadStats(); err != nil {
		// 非致命错误，继续
		fmt.Printf("加载统计缓存失败: %v\n", err)
	}

	return s, nil
}

// initTables 初始化表结构
func (s *Storage) initTables() error {
	// 请求记录表
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    provider TEXT NOT NULL,
    model TEXT,
    stream INTEGER DEFAULT 0,
    method TEXT,
    path TEXT,
    client_ip TEXT,
    request_body TEXT,
    response_body TEXT,
    status_code INTEGER,
    duration_ms REAL,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    success INTEGER DEFAULT 1,
    error_msg TEXT
);

CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON requests(timestamp);
CREATE INDEX IF NOT EXISTS idx_requests_provider ON requests(provider);
CREATE INDEX IF NOT EXISTS idx_requests_model ON requests(model);
	`)
	return err
}

// loadStats 加载统计缓存
func (s *Storage) loadStats() error {
	var total struct {
		requests int64
		input    int64
		output   int64
	}

	err := s.db.QueryRow(`
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM requests
	`).Scan(&total.requests, &total.input, &total.output)

	if err != nil {
		return err
	}

	s.mu.Lock()
	s.totalRequests = total.requests
	s.totalInputTokens = total.input
	s.totalOutputTokens = total.output
	s.totalTokens = total.input + total.output
	s.mu.Unlock()

	return nil
}

// SaveRequest 保存请求记录
func (s *Storage) SaveRequest(record *RequestRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO requests (
			timestamp, provider, model, stream, method, path, client_ip,
			request_body, response_body, status_code, duration_ms,
			input_tokens, output_tokens, total_tokens, success, error_msg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.Timestamp,
		record.Provider,
		record.Model,
		record.Stream,
		record.Method,
		record.Path,
		record.ClientIP,
		record.RequestBody,
		record.ResponseBody,
		record.StatusCode,
		record.Duration,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		record.Success,
		record.ErrorMsg,
	)

	if err == nil {
		// 更新缓存
		s.totalRequests++
		s.totalInputTokens += int64(record.InputTokens)
		s.totalOutputTokens += int64(record.OutputTokens)
		s.totalTokens += int64(record.TotalTokens)
	}

	return err
}

// GetStats 获取统计信息
func (s *Storage) GetStats() (*Stats, error) {
	s.mu.RLock()
	total := Stats{
		TotalRequests:     s.totalRequests,
		TotalInputTokens:  s.totalInputTokens,
		TotalOutputTokens: s.totalOutputTokens,
		TotalTokens:       s.totalTokens,
	}
	s.mu.RUnlock()

	// 获取今日统计
	today := time.Now().Format("2006-01-02")
	err := s.db.QueryRow(`
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM requests
		WHERE date(timestamp) = ?
	`, today).Scan(&total.TodayRequests, &total.TodayInputTokens, &total.TodayOutputTokens)

	if err != nil {
		return nil, err
	}

	total.TodayTokens = total.TodayInputTokens + total.TodayOutputTokens

	// 计算每分钟速率（基于最近5分钟的数据）
	var reqCount, inputSum, outputSum int64
	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	err = s.db.QueryRow(`
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM requests
		WHERE timestamp >= ?
	`, fiveMinAgo).Scan(&reqCount, &inputSum, &outputSum)

	if err == nil {
		total.RequestsPerMin = float64(reqCount) / 5
		total.InputPerMin = float64(inputSum) / 5
		total.OutputPerMin = float64(outputSum) / 5
	} else {
		total.RequestsPerMin = 0
		total.InputPerMin = 0
		total.OutputPerMin = 0
	}

	return &total, nil
}

// GetRecentRequests 获取最近请求记录
func (s *Storage) GetRecentRequests(limit int) ([]RequestRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, timestamp, provider, model, stream, method, path, client_ip,
		       status_code, duration_ms, input_tokens, output_tokens, total_tokens, success, error_msg
		FROM requests
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RequestRecord
	for rows.Next() {
		var r RequestRecord
		var stream int
		var success int
		err := rows.Scan(
			&r.ID, &r.Timestamp, &r.Provider, &r.Model, &stream, &r.Method, &r.Path, &r.ClientIP,
			&r.StatusCode, &r.Duration, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &success, &r.ErrorMsg,
		)
		if err != nil {
			continue
		}
		r.Stream = stream == 1
		r.Success = success == 1
		records = append(records, r)
	}

	return records, nil
}

// GetAllRequestsLite 获取所有请求记录（轻量版，用于历史列表）
func (s *Storage) GetAllRequestsLite() ([]RequestRecordLite, error) {
	rows, err := s.db.Query(`
		SELECT id, timestamp, provider, model, stream, method, path, client_ip,
		       status_code, duration_ms, input_tokens, output_tokens, total_tokens, success
		FROM requests
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RequestRecordLite
	for rows.Next() {
		var r RequestRecordLite
		var stream int
		var success int
		err := rows.Scan(
			&r.ID, &r.Timestamp, &r.Provider, &r.Model, &stream, &r.Method, &r.Path, &r.ClientIP,
			&r.StatusCode, &r.Duration, &r.InputTokens, &r.OutputTokens, &r.TotalTokens, &success,
		)
		if err != nil {
			continue
		}
		r.Stream = stream == 1
		r.Success = success == 1
		records = append(records, r)
	}

	return records, nil
}

// GetRequestDetail 获取请求详情
func (s *Storage) GetRequestDetail(id int64) (*RequestRecord, error) {
	var r RequestRecord
	var stream int
	var success int

	err := s.db.QueryRow(`
		SELECT id, timestamp, provider, model, stream, method, path, client_ip,
		       request_body, response_body, status_code, duration_ms,
		       input_tokens, output_tokens, total_tokens, success, error_msg
		FROM requests
		WHERE id = ?
	`, id).Scan(
		&r.ID, &r.Timestamp, &r.Provider, &r.Model, &stream, &r.Method, &r.Path, &r.ClientIP,
		&r.RequestBody, &r.ResponseBody, &r.StatusCode, &r.Duration,
		&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &success, &r.ErrorMsg,
	)

	if err != nil {
		return nil, err
	}

	r.Stream = stream == 1
	r.Success = success == 1
	return &r, nil
}

// Close 关闭存储
func (s *Storage) Close() error {
	return s.db.Close()
}

// GetDBPath 获取数据库路径
func (s *Storage) GetDBPath() string {
	return s.path
}

// GetTotalStats 获取缓存的总统计（快速）
func (s *Storage) GetTotalStats() (requests, inputTokens, outputTokens, totalTokens int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalRequests, s.totalInputTokens, s.totalOutputTokens, s.totalTokens
}

// GetHourlyStats 获取每小时统计
func (s *Storage) GetHourlyStats(hours int) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT
			strftime('%Y-%m-%d %H:00', timestamp) as hour,
			COUNT(*) as requests,
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens
		FROM requests
		WHERE timestamp >= datetime('now', '-' || ? || ' hours')
		GROUP BY hour
		ORDER BY hour DESC
	`, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var hour string
		var requests, inputTokens, outputTokens int64
		if err := rows.Scan(&hour, &requests, &inputTokens, &outputTokens); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"hour":          hour,
			"requests":      requests,
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"total_tokens":  inputTokens + outputTokens,
		})
	}

	return result, nil
}

// ExportStatsJSON 导出统计为 JSON
func (s *Storage) ExportStatsJSON() (string, error) {
	stats, err := s.GetStats()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
