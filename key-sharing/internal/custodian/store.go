// Package custodian implementa o Custodiante de Chaves: o serviço que mantém
// um compartilhamento da chave de cada paciente e executa decifrações parciais
// sob demanda do KMS.
package custodian

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // driver SQLite sem dependência de CGO
)

// Store persiste os compartilhamentos sob custódia e o registro local de
// auditoria. Conforme a proposta, os dados armazenados são apenas
// identificadores, metadados e compartilhamentos criptográficos, o que dispensa
// mecanismos complexos de consulta.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS shares (
    key_id      TEXT    NOT NULL PRIMARY KEY,
    share_index INTEGER NOT NULL,
    share_value BLOB    NOT NULL,
    created_at  TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS access_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id         TEXT    NOT NULL,
    requester_id   TEXT    NOT NULL,
    institution_id TEXT    NOT NULL,
    emergency      INTEGER NOT NULL,
    reason         TEXT,
    occurred_at    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// accessLogIndex: a coluna ledger_access_log guarda a chave do registro de auditoria do livro-razão que
// autorizou a operação. O índice único impede que a mesma autorização seja
// usada duas vezes no mesmo custodiante. A coluna é acrescentada por
// migração, para preservar as bases criadas antes da verificação no
// livro-razão.
const accessLogIndex = `
CREATE UNIQUE INDEX IF NOT EXISTS access_log_ledger
    ON access_log (ledger_access_log) WHERE ledger_access_log IS NOT NULL;
`

// OpenStore abre (ou cria) a base local do custodiante.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("abrindo base %s: %w", path, err)
	}
	// A base é acessada por um único processo; uma conexão evita
	// contenção de escrita ("database is locked") no SQLite.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("criando esquema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate acrescenta a coluna ledger_access_log às bases antigas.
func migrate(db *sql.DB) error {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('access_log') WHERE name = 'ledger_access_log'`,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("inspecionando o esquema: %w", err)
	}
	if n == 0 {
		if _, err := db.Exec(`ALTER TABLE access_log ADD COLUMN ledger_access_log TEXT`); err != nil {
			return fmt.Errorf("migrando o esquema: %w", err)
		}
	}
	if _, err := db.Exec(accessLogIndex); err != nil {
		return fmt.Errorf("criando o índice de autorizações: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// PutShare armazena o compartilhamento de uma chave. Reenvios do mesmo key_id
// sobrescrevem o registro, tornando a operação idempotente.
func (s *Store) PutShare(keyID string, index int, value []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO shares (key_id, share_index, share_value) VALUES (?, ?, ?)
		 ON CONFLICT(key_id) DO UPDATE SET share_index = excluded.share_index,
		                                   share_value = excluded.share_value`,
		keyID, index, value,
	)
	if err != nil {
		return fmt.Errorf("gravando share de %s: %w", keyID, err)
	}
	return nil
}

// ErrShareNotFound indica que o custodiante não possui share para a chave.
var ErrShareNotFound = fmt.Errorf("compartilhamento não encontrado")

// GetShare recupera o compartilhamento de uma chave.
func (s *Store) GetShare(keyID string) (index int, value []byte, err error) {
	row := s.db.QueryRow(`SELECT share_index, share_value FROM shares WHERE key_id = ?`, keyID)
	switch err := row.Scan(&index, &value); {
	case err == sql.ErrNoRows:
		return 0, nil, ErrShareNotFound
	case err != nil:
		return 0, nil, fmt.Errorf("lendo share de %s: %w", keyID, err)
	}
	return index, value, nil
}

// CountShares informa quantos compartilhamentos o custodiante mantém.
func (s *Store) CountShares() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM shares`).Scan(&n); err != nil {
		return 0, fmt.Errorf("contando shares: %w", err)
	}
	return n, nil
}

// ErrAuthorizationUsed indica que o registro de auditoria do livro-razão já
// autorizou uma decifração parcial neste custodiante.
var ErrAuthorizationUsed = errors.New("autorização já utilizada")

// LogAccess registra localmente uma operação de decifração parcial. Quando a
// entrada traz a chave do registro do livro-razão, o registro também consome
// a autorização: uma segunda operação com a mesma chave devolve
// ErrAuthorizationUsed e não é gravada.
func (s *Store) LogAccess(e AccessEntry) error {
	var ledgerLog any
	if e.AccessLog != "" {
		ledgerLog = e.AccessLog
	}
	res, err := s.db.Exec(
		`INSERT INTO access_log (key_id, requester_id, institution_id, emergency, reason, ledger_access_log)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (ledger_access_log) WHERE ledger_access_log IS NOT NULL DO NOTHING`,
		e.KeyID, e.RequesterID, e.InstitutionID, e.Emergency, e.Reason, ledgerLog,
	)
	if err != nil {
		return fmt.Errorf("registrando acesso a %s: %w", e.KeyID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("registrando acesso a %s: %w", e.KeyID, err)
	} else if n == 0 {
		return ErrAuthorizationUsed
	}
	return nil
}

// AccessEntry é uma linha do registro local de auditoria.
type AccessEntry struct {
	KeyID         string
	RequesterID   string
	InstitutionID string
	Emergency     bool
	Reason        string
	AccessLog     string // chave do registro de auditoria no livro-razão
	OccurredAt    string
}

// AccessLog devolve o histórico de acessos em ordem cronológica.
func (s *Store) AccessLog() ([]AccessEntry, error) {
	rows, err := s.db.Query(
		`SELECT key_id, requester_id, institution_id, emergency, reason, ledger_access_log, occurred_at
		 FROM access_log ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("consultando auditoria: %w", err)
	}
	defer rows.Close()

	var entries []AccessEntry
	for rows.Next() {
		var e AccessEntry
		var reason, ledgerLog sql.NullString
		if err := rows.Scan(&e.KeyID, &e.RequesterID, &e.InstitutionID,
			&e.Emergency, &reason, &ledgerLog, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("lendo auditoria: %w", err)
		}
		e.Reason = reason.String
		e.AccessLog = ledgerLog.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
