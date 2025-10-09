package web

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
	"net/http"
	"slices"
	"time"
)

type gw2EffApiStatusElement struct {
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Duration int    `json:"duration"`
	Error    any    `json:"error"`
}

func (e gw2EffApiStatusElement) CheckError() bool {
	switch v := e.Error.(type) {
	case bool:
		return v

	case string:
		return v != ""

	default:
		return false
	}
}

type gw2EffApiStatusResponse struct {
	Data      []gw2EffApiStatusElement `json:"data"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type notificationType string

const (
	notificationTypeSuccess    = notificationType("success")
	notificationTypeInfo       = notificationType("info")
	notificationTypeWarning    = notificationType("warning")
	notificationTypeError      = notificationType("error")
	notificationTypeInProgress = notificationType("in-progress")
)

type notification struct {
	Type    notificationType `json:"type"`
	Header  string           `json:"header,omitempty"`
	Content string           `json:"content,omitempty"`
}

func NotificationsEndpoint(httpClient *http.Client) echo.HandlerFunc {
	apiDowntimeEnd := time.Unix(1731366000, 0)
	relevantEndpoints := []string{
		"/v2/tokeninfo",
		"/v2/account",
		"/v2/characters",
		"/v2/createsubtoken",
		"/v2/commerce/transactions/current/buys",
	}

	return func(c echo.Context) error {
		ctx := c.Request().Context()
		g, ctx := errgroup.WithContext(ctx)

		var firstResult, secondResult string

		g.Go(func() error {
			// Simulate first parallel request
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/first", nil)
			if err != nil {
				return err
			}
			res, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if err := json.NewDecoder(res.Body).Decode(&firstResult); err != nil {
				return err
			}
			return nil
		})

		g.Go(func() error {
			// Simulate second parallel request
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/second", nil)
			if err != nil {
				return err
			}
			res, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if err := json.NewDecoder(res.Body).Decode(&secondResult); err != nil {
				return err
			}
			return nil
		})

		if err := g.Wait(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		return c.JSON(http.StatusOK, map[string]string{
			"first":  firstResult,
			"second": secondResult,
		})
	}
}