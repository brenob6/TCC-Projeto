// Package crypto implementa o esquema de cifração de limiar utilizado para
// proteger os prontuários médicos.
//
// Esquema:
//   - Shamir Secret Sharing (k-de-n) divide a chave privada em shares
//   - Cada custodiante decifra parcialmente com seu share: D_i = C1^{x_i}
//   - Combinação via interpolação de Lagrange no expoente: D = C1^x = Y^r
//   - A chave privada completa jamais é reconstituída em memória
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/random"
)

// Suite define a curva elíptica utilizada em todo o sistema.
var Suite = suites.MustFind("Ed25519")

// Share representa um compartilhamento da chave privada mantido por um
// custodiante.
type Share struct {
	Index int
	Value []byte
}

// EncryptedRecord é o envelope de um prontuário cifrado. Apenas este envelope
// é armazenado na rede IPFS.
type EncryptedRecord struct {
	C1         []byte `json:"c1"`          // ponto efêmero g^r
	WrappedKey []byte `json:"wrapped_key"` // K XOR SHA256(Y^r)
	Nonce      []byte `json:"nonce"`       // nonce AES-GCM
	Ciphertext []byte `json:"ciphertext"`  // prontuário cifrado com AES-256-GCM
}

// ParseEnvelope decodifica o envelope serializado em JSON, formato em que ele
// é armazenado na rede IPFS.
func ParseEnvelope(data []byte) (*EncryptedRecord, error) {
	var rec EncryptedRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("envelope inválido: %w", err)
	}
	if len(rec.C1) == 0 {
		return nil, fmt.Errorf("envelope inválido: C1 ausente")
	}
	return &rec, nil
}

// GenerateKey cria uma chave de limiar (k,n): retorna a chave pública e os n
// compartilhamentos. O segredo nunca é retornado nem persistido.
func GenerateKey(threshold, total int) (pubKey []byte, shares []Share, err error) {
	if threshold < 2 || threshold > total {
		return nil, nil, fmt.Errorf("limiar inválido: k=%d, n=%d", threshold, total)
	}

	secret := Suite.Scalar().Pick(random.New())
	priPoly := share.NewPriPoly(Suite, threshold, secret, random.New())

	// Chave pública Y = g^x derivada dos commitments, sem expor x.
	pubPoly := priPoly.Commit(Suite.Point().Base())
	pkBytes, err := pubPoly.Commit().MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("serializando chave pública: %w", err)
	}

	for _, s := range priPoly.Shares(total) {
		val, err := s.V.MarshalBinary()
		if err != nil {
			return nil, nil, fmt.Errorf("serializando share %d: %w", s.I, err)
		}
		shares = append(shares, Share{Index: s.I, Value: val})
	}

	return pkBytes, shares, nil
}

// Encrypt cifra o prontuário com a chave pública, usando ElGamal híbrido:
//
//	C1        = g^r
//	shared    = Y^r
//	K         = chave AES-256 aleatória
//	wrapped   = K XOR SHA256(shared)
//	ciphertext = AES-256-GCM(K, plaintext)
func Encrypt(pubKey, plaintext []byte) (*EncryptedRecord, error) {
	Y := Suite.Point()
	if err := Y.UnmarshalBinary(pubKey); err != nil {
		return nil, fmt.Errorf("chave pública inválida: %w", err)
	}

	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, err
	}

	r := Suite.Scalar().Pick(random.New())
	c1 := Suite.Point().Mul(r, Suite.Point().Base())
	shared := Suite.Point().Mul(r, Y)

	wrapped, err := wrapKey(aesKey, shared)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(aesKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	c1Bytes, err := c1.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &EncryptedRecord{
		C1:         c1Bytes,
		WrappedKey: wrapped,
		Nonce:      nonce,
		Ciphertext: gcm.Seal(nil, nonce, plaintext, nil),
	}, nil
}

// PartialDecrypt executa a decifração parcial D_i = C1^{x_i}. É a única
// operação criptográfica realizada por um custodiante, que utiliza apenas o
// share sob sua custódia.
func PartialDecrypt(shareValue, c1Bytes []byte) ([]byte, error) {
	x := Suite.Scalar()
	if err := x.UnmarshalBinary(shareValue); err != nil {
		return nil, fmt.Errorf("share inválido: %w", err)
	}

	c1 := Suite.Point()
	if err := c1.UnmarshalBinary(c1Bytes); err != nil {
		return nil, fmt.Errorf("ponto C1 inválido: %w", err)
	}

	partial, err := Suite.Point().Mul(x, c1).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("serializando decifração parcial: %w", err)
	}
	return partial, nil
}

// Partial associa uma decifração parcial ao índice do share que a produziu,
// informação necessária à interpolação de Lagrange.
type Partial struct {
	Index int
	Value []byte
}

// CombineAndDecrypt combina as decifrações parciais via Lagrange no expoente,
// recuperando D = C1^x = Y^r, e decifra o prontuário. A chave privada não é
// reconstituída em nenhum momento.
func CombineAndDecrypt(rec *EncryptedRecord, partials []Partial, threshold, total int) ([]byte, error) {
	if len(partials) < threshold {
		return nil, fmt.Errorf("necessário ao menos %d decifrações parciais, obtidas %d", threshold, len(partials))
	}

	pubShares := make([]*share.PubShare, 0, len(partials))
	for _, p := range partials {
		point := Suite.Point()
		if err := point.UnmarshalBinary(p.Value); err != nil {
			return nil, fmt.Errorf("decifração parcial inválida do índice %d: %w", p.Index, err)
		}
		pubShares = append(pubShares, &share.PubShare{I: p.Index, V: point})
	}

	shared, err := share.RecoverCommit(Suite, pubShares, threshold, total)
	if err != nil {
		return nil, fmt.Errorf("combinando decifrações parciais: %w", err)
	}

	aesKey, err := wrapKey(rec.WrappedKey, shared)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(aesKey)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, rec.Nonce, rec.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("falha na decifragem AES-GCM (shares incorretos?): %w", err)
	}
	return plaintext, nil
}

// wrapKey aplica XOR entre a chave e a máscara derivada do segredo
// compartilhado. A operação é involutiva: serve para embrulhar e desembrulhar.
func wrapKey(key []byte, shared interface {
	MarshalBinary() ([]byte, error)
}) ([]byte, error) {
	sharedBytes, err := shared.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("serializando segredo compartilhado: %w", err)
	}
	mask := sha256.Sum256(sharedBytes)
	if len(key) > len(mask) {
		return nil, fmt.Errorf("chave maior que a máscara: %d bytes", len(key))
	}

	out := make([]byte, len(key))
	for i := range key {
		out[i] = key[i] ^ mask[i]
	}
	return out, nil
}

func newGCM(aesKey []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
