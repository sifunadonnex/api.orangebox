package handlers

import "database/sql"

// Statements belong to one transaction and are closed on every exit path.
// Preparing lazily avoids work for inserts that a detection run never needs.
type preparedWrites struct {
	tx         *sql.Tx
	statements map[string]*sql.Stmt
}

func (w *preparedWrites) Exec(query string, args ...any) (sql.Result, error) {
	statement := w.statements[query]
	if statement == nil {
		var err error
		statement, err = w.tx.Prepare(query)
		if err != nil {
			return nil, err
		}
		w.statements[query] = statement
	}
	return statement.Exec(args...)
}

func (w *preparedWrites) Close() {
	for _, statement := range w.statements {
		_ = statement.Close()
	}
}
