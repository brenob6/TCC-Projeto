// Package ipfs acessa a API HTTP do Kubo nos nós da rede IPFS privada.
package ipfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// Client fala com a API (porta 5001) de um nó IPFS.
type Client struct {
	base string
	http *http.Client
}

// New cria um cliente para a API em base (ex.: http://192.168.56.31:5001).
func New(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) post(ctx context.Context, path string, query url.Values, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v0/"+path+"?"+query.Encode(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IPFS %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IPFS %s: %s: %s", path, resp.Status, bytes.TrimSpace(data))
	}
	return data, nil
}

// Add armazena e fixa (pin) o conteúdo, devolvendo o CID (versão 1).
func (c *Client) Add(ctx context.Context, name string, data []byte) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	out, err := c.post(ctx, "add", url.Values{"pin": {"true"}, "cid-version": {"1"}}, &body, w.FormDataContentType())
	if err != nil {
		return "", err
	}
	var res struct{ Hash string }
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("resposta inválida do IPFS: %w", err)
	}
	return res.Hash, nil
}

// Cat recupera o conteúdo de um CID, buscando-o nos demais nós se preciso.
func (c *Client) Cat(ctx context.Context, cid string) ([]byte, error) {
	return c.post(ctx, "cat", url.Values{"arg": {cid}}, nil, "")
}

// Unpin remove a fixação do CID neste nó. O coletor de lixo do Kubo apaga o
// bloco posteriormente.
func (c *Client) Unpin(ctx context.Context, cid string) error {
	_, err := c.post(ctx, "pin/rm", url.Values{"arg": {cid}}, nil, "")
	return err
}

// ID consulta a identidade do nó, usada na verificação de disponibilidade.
func (c *Client) ID(ctx context.Context) (string, error) {
	out, err := c.post(ctx, "id", nil, nil, "")
	if err != nil {
		return "", err
	}
	var res struct{ ID string }
	if err := json.Unmarshal(out, &res); err != nil {
		return "", err
	}
	return res.ID, nil
}
