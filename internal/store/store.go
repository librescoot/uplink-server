// Package store provides durable persistence (SQLite) for telemetry history,
// events, and command history/queue.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed persistence layer.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies the
// schema. WAL mode is enabled for concurrent read/write throughput.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// modernc's driver is safe for concurrent use but a single writer avoids
	// "database is locked" churn under bursty telemetry.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS telemetry_history (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	scooter_id TEXT    NOT NULL,
	ts         INTEGER NOT NULL,
	lat        REAL,
	lng        REAL,
	speed      REAL,
	state      TEXT,
	data       TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_th_scooter_ts ON telemetry_history(scooter_id, ts);

CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	scooter_id TEXT    NOT NULL,
	ts         INTEGER NOT NULL,
	event      TEXT    NOT NULL,
	data       TEXT
);
CREATE INDEX IF NOT EXISTS idx_ev_scooter_ts ON events(scooter_id, ts);

CREATE TABLE IF NOT EXISTS commands (
	request_id  TEXT    PRIMARY KEY,
	scooter_id  TEXT    NOT NULL,
	command     TEXT    NOT NULL,
	params      TEXT,
	status      TEXT    NOT NULL,
	result      TEXT,
	error       TEXT,
	enqueued_at INTEGER NOT NULL,
	sent_at     INTEGER,
	acked_at    INTEGER,
	expires_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cmd_scooter_status ON commands(scooter_id, status);
`
	_, err := s.db.Exec(schema)
	return err
}

// --- Telemetry history ---

// TelemetryRow is one stored snapshot.
type TelemetryRow struct {
	Timestamp time.Time      `json:"timestamp"`
	Lat       *float64       `json:"lat,omitempty"`
	Lng       *float64       `json:"lng,omitempty"`
	Speed     *float64       `json:"speed,omitempty"`
	State     string         `json:"state,omitempty"`
	Data      map[string]any `json:"data"`
}

// InsertTelemetry stores a full-state snapshot, extracting a few columns for
// efficient querying.
func (s *Store) InsertTelemetry(scooterID string, ts time.Time, data map[string]any) error {
	lat, lng, speed, state := extractColumns(data)
	blob, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO telemetry_history(scooter_id, ts, lat, lng, speed, state, data) VALUES(?,?,?,?,?,?,?)`,
		scooterID, ts.UnixMilli(), lat, lng, speed, state, string(blob),
	)
	return err
}

// QueryTelemetry returns snapshots for a scooter within [from, to], newest
// first, up to limit rows.
func (s *Store) QueryTelemetry(scooterID string, from, to time.Time, limit int) ([]TelemetryRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(
		`SELECT ts, lat, lng, speed, state, data FROM telemetry_history
		 WHERE scooter_id=? AND ts BETWEEN ? AND ? ORDER BY ts DESC LIMIT ?`,
		scooterID, from.UnixMilli(), to.UnixMilli(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TelemetryRow
	for rows.Next() {
		var (
			tsMillis            int64
			lat, lng, speed     sql.NullFloat64
			state               sql.NullString
			blob                string
		)
		if err := rows.Scan(&tsMillis, &lat, &lng, &speed, &state, &blob); err != nil {
			return nil, err
		}
		row := TelemetryRow{Timestamp: time.UnixMilli(tsMillis).UTC()}
		if lat.Valid {
			v := lat.Float64
			row.Lat = &v
		}
		if lng.Valid {
			v := lng.Float64
			row.Lng = &v
		}
		if speed.Valid {
			v := speed.Float64
			row.Speed = &v
		}
		if state.Valid {
			row.State = state.String
		}
		_ = json.Unmarshal([]byte(blob), &row.Data)
		out = append(out, row)
	}
	return out, rows.Err()
}

// PruneTelemetryBefore deletes telemetry rows older than the cutoff, returning
// the number removed.
func (s *Store) PruneTelemetryBefore(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM telemetry_history WHERE ts < ?`, cutoff.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Events ---

// InsertEvent stores an event.
func (s *Store) InsertEvent(scooterID string, ts time.Time, event string, data map[string]any) error {
	var blob any
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		blob = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO events(scooter_id, ts, event, data) VALUES(?,?,?,?)`,
		scooterID, ts.UnixMilli(), event, blob,
	)
	return err
}

// EventRow is one stored event.
type EventRow struct {
	Timestamp time.Time      `json:"timestamp"`
	Event     string         `json:"event"`
	Data      map[string]any `json:"data,omitempty"`
}

// QueryEvents returns events for a scooter within [from, to], newest first.
func (s *Store) QueryEvents(scooterID string, from, to time.Time, limit int) ([]EventRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(
		`SELECT ts, event, data FROM events WHERE scooter_id=? AND ts BETWEEN ? AND ?
		 ORDER BY ts DESC LIMIT ?`,
		scooterID, from.UnixMilli(), to.UnixMilli(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRow
	for rows.Next() {
		var (
			tsMillis int64
			event    string
			blob     sql.NullString
		)
		if err := rows.Scan(&tsMillis, &event, &blob); err != nil {
			return nil, err
		}
		row := EventRow{Timestamp: time.UnixMilli(tsMillis).UTC(), Event: event}
		if blob.Valid {
			_ = json.Unmarshal([]byte(blob.String), &row.Data)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func extractColumns(data map[string]any) (lat, lng, speed sql.NullFloat64, state sql.NullString) {
	if gps, ok := data["gps"].(map[string]any); ok {
		lat = nullFloatField(gps, "latitude")
		lng = nullFloatField(gps, "longitude")
	}
	if eng, ok := data["engine-ecu"].(map[string]any); ok {
		speed = nullFloatField(eng, "speed")
	}
	if v, ok := data["vehicle"].(map[string]any); ok {
		if s, ok := v["state"].(string); ok && s != "" {
			state = sql.NullString{String: s, Valid: true}
		}
	}
	return
}

func nullFloatField(m map[string]any, field string) sql.NullFloat64 {
	s, ok := m[field].(string)
	if !ok {
		return sql.NullFloat64{}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}
