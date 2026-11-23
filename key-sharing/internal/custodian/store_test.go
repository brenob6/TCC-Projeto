package custodian

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// Bases criadas antes da verificação no livro-razão recebem a nova coluna sem
// perder os compartilhamentos nem a auditoria existentes.
func TestOpenStoreMigraBaseAntiga(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shares.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO shares (key_id, share_index, share_value) VALUES ('paciente-1', 1, x'01');
		INSERT INTO access_log (key_id, requester_id, institution_id, emergency) VALUES ('paciente-1', 'medico', 'hospital', 0)`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("abrindo base antiga: %v", err)
	}
	defer store.Close()

	if _, _, err := store.GetShare("paciente-1"); err != nil {
		t.Fatalf("compartilhamento perdido na migração: %v", err)
	}
	entries, err := store.AccessLog()
	if err != nil || len(entries) != 1 || entries[0].AccessLog != "" {
		t.Fatalf("auditoria após a migração: %+v, %v", entries, err)
	}

	// Reabrir a base já migrada não falha.
	store.Close()
	if store, err = OpenStore(path); err != nil {
		t.Fatalf("reabrindo base migrada: %v", err)
	}
}

// A mesma autorização do livro-razão é aceita uma única vez; entradas sem
// autorização (modo isolado) não são restringidas.
func TestLogAccessConsomeAutorizacao(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "shares.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	e := AccessEntry{KeyID: "paciente-1", RequesterID: "medico", InstitutionID: "hospital", AccessLog: "accessLog:1"}
	if err := store.LogAccess(e); err != nil {
		t.Fatalf("primeiro uso: %v", err)
	}
	if err := store.LogAccess(e); !errors.Is(err, ErrAuthorizationUsed) {
		t.Fatalf("segundo uso: %v, esperado ErrAuthorizationUsed", err)
	}

	isolated := AccessEntry{KeyID: "paciente-1", RequesterID: "medico", InstitutionID: "hospital"}
	for i := 0; i < 2; i++ {
		if err := store.LogAccess(isolated); err != nil {
			t.Fatalf("registro sem autorização %d: %v", i+1, err)
		}
	}
	entries, _ := store.AccessLog()
	if len(entries) != 3 {
		t.Fatalf("%d registros, esperado 3", len(entries))
	}
}
