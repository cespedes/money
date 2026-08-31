package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// apiError is returned by the API as {"error": "..."} on failure.
type apiError struct {
	Error string `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s %s: %s", method, path, apiErr.Error)
		}
		return fmt.Errorf("%s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	if err := c.do(ctx, http.MethodGet, "/accounts", nil, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (c *Client) CreateAccount(ctx context.Context, a Account) (Account, error) {
	var created Account
	err := c.do(ctx, http.MethodPost, "/accounts", a, &created)
	return created, err
}

func (c *Client) DeleteAccount(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/accounts/"+strconv.FormatInt(id, 10), nil, nil)
}

func (c *Client) ListTransactions(ctx context.Context) ([]Transaction, error) {
	var transactions []Transaction
	if err := c.do(ctx, http.MethodGet, "/transactions", nil, &transactions); err != nil {
		return nil, err
	}
	return transactions, nil
}

func (c *Client) GetTransaction(ctx context.Context, id int64) (Transaction, error) {
	var t Transaction
	err := c.do(ctx, http.MethodGet, "/transactions/"+strconv.FormatInt(id, 10), nil, &t)
	return t, err
}

func (c *Client) CreateTransaction(ctx context.Context, t Transaction) (Transaction, error) {
	var created Transaction
	err := c.do(ctx, http.MethodPost, "/transactions", t, &created)
	return created, err
}

func (c *Client) DeleteTransaction(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/transactions/"+strconv.FormatInt(id, 10), nil, nil)
}
