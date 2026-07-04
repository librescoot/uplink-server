package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Command statuses.
const (
	StatusQueued  = "queued"
	StatusSent    = "sent"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

// CommandRecord is the persisted view of a command.
type CommandRecord struct {
	RequestID  string         `json:"request_id"`
	ScooterID  string         `json:"scooter_id"`
	Command    string         `json:"command"`
	Params     map[string]any `json:"params,omitempty"`
	Status     string         `json:"status"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	EnqueuedAt time.Time      `json:"enqueued_at"`
	SentAt     *time.Time     `json:"sent_at,omitempty"`
	AckedAt    *time.Time     `json:"acked_at,omitempty"`
}

// QueuedCommand is a command awaiting delivery to a reconnecting scooter.
type QueuedCommand struct {
	RequestID string
	Command   string
	Params    map[string]any
}

// RecordSent inserts a command that was delivered immediately to an online
// scooter, so its metadata (name, params) is known before any ack arrives.
func (s *Store) RecordSent(requestID, scooterID, command string, params map[string]any) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO commands(request_id, scooter_id, command, params, status, enqueued_at, sent_at, expires_at)
		 VALUES(?,?,?,?,?,?,?,0)`,
		requestID, scooterID, command, marshalMap(params), StatusSent, now, now,
	)
	return err
}

// Enqueue stores a command for later delivery when the scooter reconnects. A
// ttl of zero means no expiry.
func (s *Store) Enqueue(requestID, scooterID, command string, params map[string]any, ttl time.Duration) error {
	now := time.Now()
	var expires int64
	if ttl > 0 {
		expires = now.Add(ttl).UnixMilli()
	}
	_, err := s.db.Exec(
		`INSERT INTO commands(request_id, scooter_id, command, params, status, enqueued_at, expires_at)
		 VALUES(?,?,?,?,?,?,?)`,
		requestID, scooterID, command, marshalMap(params), StatusQueued, now.UnixMilli(), expires,
	)
	return err
}

// UpdateResult records the outcome of a command from its ack.
func (s *Store) UpdateResult(requestID, status string, result map[string]any, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE commands SET status=?, result=?, error=?, acked_at=? WHERE request_id=?`,
		status, marshalMap(result), nullString(errMsg), time.Now().UnixMilli(), requestID,
	)
	return err
}

// DequeueQueued returns all non-expired queued commands for a scooter and marks
// them sent. Intended to be called on reconnect.
func (s *Store) DequeueQueued(scooterID string) ([]QueuedCommand, error) {
	now := time.Now().UnixMilli()
	rows, err := s.db.Query(
		`SELECT request_id, command, params FROM commands
		 WHERE scooter_id=? AND status=? AND (expires_at=0 OR expires_at>?)
		 ORDER BY enqueued_at ASC`,
		scooterID, StatusQueued, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueuedCommand
	for rows.Next() {
		var (
			id, command string
			params      sql.NullString
		)
		if err := rows.Scan(&id, &command, &params); err != nil {
			return nil, err
		}
		qc := QueuedCommand{RequestID: id, Command: command}
		if params.Valid {
			_ = json.Unmarshal([]byte(params.String), &qc.Params)
		}
		out = append(out, qc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(out) > 0 {
		if _, err := s.db.Exec(
			`UPDATE commands SET status=?, sent_at=? WHERE scooter_id=? AND status=? AND (expires_at=0 OR expires_at>?)`,
			StatusSent, now, scooterID, StatusQueued, now,
		); err != nil {
			return out, err
		}
	}
	return out, nil
}

// ExpireStale marks queued commands past their expiry as expired, returning the
// count.
func (s *Store) ExpireStale() (int64, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(
		`UPDATE commands SET status=? WHERE status=? AND expires_at!=0 AND expires_at<=?`,
		StatusExpired, StatusQueued, now,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetCommand returns a single command record.
func (s *Store) GetCommand(requestID string) (*CommandRecord, bool, error) {
	row := s.db.QueryRow(
		`SELECT request_id, scooter_id, command, params, status, result, error, enqueued_at, sent_at, acked_at
		 FROM commands WHERE request_id=?`, requestID,
	)
	rec, ok, err := scanCommand(row)
	return rec, ok, err
}

// QueryCommands returns recent command records for a scooter, newest first.
func (s *Store) QueryCommands(scooterID string, limit int) ([]CommandRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT request_id, scooter_id, command, params, status, result, error, enqueued_at, sent_at, acked_at
		 FROM commands WHERE scooter_id=? ORDER BY enqueued_at DESC LIMIT ?`,
		scooterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandRecord
	for rows.Next() {
		rec, _, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// rowScanner unifies *sql.Row and *sql.Rows for scanCommand.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCommand(sc rowScanner) (*CommandRecord, bool, error) {
	var (
		rec                       CommandRecord
		params, result, errMsg    sql.NullString
		enqueued                  int64
		sentAt, ackedAt           sql.NullInt64
	)
	err := sc.Scan(&rec.RequestID, &rec.ScooterID, &rec.Command, &params, &rec.Status,
		&result, &errMsg, &enqueued, &sentAt, &ackedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if params.Valid {
		_ = json.Unmarshal([]byte(params.String), &rec.Params)
	}
	if result.Valid {
		_ = json.Unmarshal([]byte(result.String), &rec.Result)
	}
	if errMsg.Valid {
		rec.Error = errMsg.String
	}
	rec.EnqueuedAt = time.UnixMilli(enqueued).UTC()
	if sentAt.Valid {
		t := time.UnixMilli(sentAt.Int64).UTC()
		rec.SentAt = &t
	}
	if ackedAt.Valid {
		t := time.UnixMilli(ackedAt.Int64).UTC()
		rec.AckedAt = &t
	}
	return &rec, true, nil
}

func marshalMap(m map[string]any) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
