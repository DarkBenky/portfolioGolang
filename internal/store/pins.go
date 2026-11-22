package store

type Pin struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	CreatedAt int64  `json:"created_at"`
}

func (d *DB) EnsurePinsTable() error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS pinned_symbols (
		user_id TEXT NOT NULL, symbol TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
		sort_order INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
		PRIMARY KEY (user_id, symbol))`); err != nil {
		return err
	}
	_, err := d.Exec(`CREATE INDEX IF NOT EXISTS idx_pinned_symbols_user ON pinned_symbols(user_id)`)
	return err
}

func (d *DB) ListPins(userID string) ([]Pin, error) {
	rows, err := d.Query(`SELECT symbol, COALESCE(name,''), COALESCE(sort_order,0), created_at
		FROM pinned_symbols WHERE user_id = ? ORDER BY sort_order ASC, created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pins := make([]Pin, 0)
	for rows.Next() {
		var pin Pin
		if err := rows.Scan(&pin.Symbol, &pin.Name, &pin.SortOrder, &pin.CreatedAt); err == nil {
			pins = append(pins, pin)
		}
	}
	return pins, rows.Err()
}

func (d *DB) AddPin(userID string, symbol string, name string) error {
	_, err := d.Exec(`INSERT INTO pinned_symbols (user_id, symbol, name, sort_order, created_at)
		VALUES (?, ?, ?, 0, ?) ON CONFLICT (user_id, symbol) DO UPDATE SET name = excluded.name`,
		userID, symbol, name, Now())
	return err
}

func (d *DB) RemovePin(userID string, symbol string) error {
	_, err := d.Exec(`DELETE FROM pinned_symbols WHERE user_id = ? AND symbol = ?`, userID, symbol)
	return err
}
