package gw2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const apiVersion = "2023-11-19"

var (
	ErrInvalidApiToken = errors.New("invalid api token")
)

type ApiError struct {
	StatusCode  int
	RawResponse string
}

func (e ApiError) Error() string {
	return fmt.Sprintf("status=%d response=[%s]", e.StatusCode, e.RawResponse)
}

func IsApiError(err error) bool {
	var x ApiError
	return errors.Is(err, &x)
}

type ApiClient struct {
	httpClient *http.Client
	url        string
}

func NewApiClient(httpClient *http.Client, url string) *ApiClient {
	return &ApiClient{
		httpClient: httpClient,
		url:        url,
	}
}

func (c *ApiClient) Account(ctx context.Context, token string) (Account, error) {
	var acc Account
	return acc, c.do(ctx, "/v2/account", token, &acc)
}

func (c *ApiClient) TokenInfo(ctx context.Context, token string) (TokenInfo, error) {
	var tokenInfo TokenInfo
	return tokenInfo, c.do(ctx, "/v2/tokeninfo", token, &tokenInfo)
}

func (c *ApiClient) do(ctx context.Context, endpoint string, token string, out any) error {
	req, err := c.newRequest(ctx, endpoint, token)
	if err != nil {
		return fmt.Errorf("failed to construct request: %w", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gw2api request failed: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		apiErr := ApiError{
			StatusCode:  res.StatusCode,
			RawResponse: "",
		}

		b, err := io.ReadAll(res.Body)
		if err == nil {
			apiErr.RawResponse = string(b)
			err = apiErr
		} else {
			err = errors.Join(apiErr, err)
		}

		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return errors.Join(ErrInvalidApiToken, err)
		} else {
			return err
		}
	}

	return json.NewDecoder(res.Body).Decode(out)
}

func (c *ApiClient) newRequest(ctx context.Context, endpoint string, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+endpoint, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("v", apiVersion)
	req.URL.RawQuery = q.Encode()

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return req, nil
}
