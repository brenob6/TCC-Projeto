package datatypes

import "testing"

func TestCPF(t *testing.T) {
	valid := []string{"529.982.247-25", "52998224725", "111.444.777-35"}
	invalid := []string{"529.982.247-24", "111.111.111-11", "1234567890", "abc.def.ghi-jk", ""}

	for _, v := range valid {
		if _, parsed, err := cpf.Parse(v); err != nil || len(parsed.(string)) != 11 {
			t.Errorf("CPF %q deveria ser válido: %v", v, err)
		}
	}
	for _, v := range invalid {
		if _, _, err := cpf.Parse(v); err == nil {
			t.Errorf("CPF %q deveria ser inválido", v)
		}
	}
}

func TestCNPJ(t *testing.T) {
	valid := []string{"11.222.333/0001-81", "45997418000153"}
	invalid := []string{"11.222.333/0001-80", "00.000.000/0000-00", "123"}

	for _, v := range valid {
		if _, _, err := cnpj.Parse(v); err != nil {
			t.Errorf("CNPJ %q deveria ser válido: %v", v, err)
		}
	}
	for _, v := range invalid {
		if _, _, err := cnpj.Parse(v); err == nil {
			t.Errorf("CNPJ %q deveria ser inválido", v)
		}
	}
}

func TestEnumAndKeys(t *testing.T) {
	if _, _, err := accessLevel.Parse(AccessReadWrite); err != nil {
		t.Error(err)
	}
	if _, _, err := accessLevel.Parse("admin"); err == nil {
		t.Error("nível de acesso desconhecido aceito")
	}
	if _, _, err := sha256.Parse("9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08"); err != nil {
		t.Error(err)
	}
	if _, _, err := sha256.Parse("abc"); err == nil {
		t.Error("hash curto aceito")
	}
	if _, _, err := ed25519PubKey.Parse("11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="); err != nil {
		t.Error(err)
	}
	if _, _, err := ed25519PubKey.Parse("eA=="); err == nil {
		t.Error("chave curta aceita")
	}
}
