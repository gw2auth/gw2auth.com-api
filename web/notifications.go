package web

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"net/http"
	"slices"
	"time"
)

type gw2EffApiStatusResponse struct {
	Data []struct {
		Name        string `json:"name"`
		Url         string `json:"url"`
		Status      int    `json:"status"`
		Duration    int    `json:"duration"`
		Error       bool   `json:"error"`
		SchemaValid bool   `json:"schemaValid"`
	} `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
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
	relevantEndpoints := []string{
		"/v2/tokeninfo",
		"/v2/account",
		"/v2/characters",
		"/v2/createsubtoken",
		"/v2/commerce/transactions/current/buys",
	}

	return func(c echo.Context) error {
		ctx := c.Request().Context()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://status.gw2efficiency.com/api", nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		req.Header.Set("User-Agent", "GW2Auth")

		res, err := httpClient.Do(req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, err)
		}

		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return echo.NewHTTPError(http.StatusBadGateway, res.Status)
		}

		var gw2EffStatus gw2EffApiStatusResponse
		if err = json.NewDecoder(res.Body).Decode(&gw2EffStatus); err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, err)
		}

		var endpointsWithIssues []string
		for _, element := range gw2EffStatus.Data {
			if slices.Contains(relevantEndpoints, element.Name) && (element.Status != http.StatusOK || element.Error || element.Duration >= 3000) {
				endpointsWithIssues = append(endpointsWithIssues, element.Name)
			}
		}

		var notifications []notification
		if len(endpointsWithIssues) > 0 {
			notifications = []notification{
				{
					Type:    notificationTypeWarning,
					Header:  "The Guild Wars 2 API experiences issues right now",
					Content: "Some of the endpoints used by GW2Auth appear to be in a degraded state right now. This might cause unexpected results using GW2Auth.",
				},
			}
		} else {
			notifications = make([]notification, 0)
		}

		return c.JSON(http.StatusOK, notifications)
	}
}
