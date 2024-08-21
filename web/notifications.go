package web

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
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
	apiDowntimeStart := time.Unix(1723827600, 0)
	apiDowntimeEnd := apiDowntimeStart.Add(time.Hour * 24 * 7)
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
			if slices.Contains(relevantEndpoints, element.Name) && (element.Status != http.StatusOK || element.CheckError() || element.Duration >= 15_000) {
				endpointsWithIssues = append(endpointsWithIssues, element.Name)
			}
		}

		var notifications []notification
		if len(endpointsWithIssues) > 0 {
			notifications = []notification{
				{
					Type:    notificationTypeWarning,
					Header:  "The Guild Wars 2 API experiences issues right now",
					Content: "Some of the endpoints used by GW2Auth appear to be in a degraded state right now. This might impact your experience with GW2Auth and Applications using GW2Auth. ",
				},
			}
		} else {
			notifications = make([]notification, 0)
		}

		now := time.Now()
		if now.Before(apiDowntimeEnd) {
			if now.After(apiDowntimeStart) {
				notifications = append(notifications, notification{
					Type:    notificationTypeWarning,
					Header:  "The Guild Wars 2 API is temporarily disabled",
					Content: "The official Guild Wars 2 API is temporarily disabled until 2024-08-23. During this time, it will not be possible to add new API Tokens or to verify your accounts. Applications using GW2Auth might not work properly.\nSee: [Guild Wars 2 API Disabled from August 16-23](https://en-forum.guildwars2.com/topic/149447-guild-wars-2-api-disabled-from-august-16-23)",
				})
			} else if timeUntilDowntime := apiDowntimeStart.Sub(now); timeUntilDowntime <= (time.Hour * 24 * 3) {
				notifications = append(notifications, notification{
					Type:    notificationTypeInfo,
					Header:  "The Guild Wars 2 API will be temporarily disabled shortly",
					Content: "The official Guild Wars 2 API will be temporarily disabled in preparation for the release of Janthir Wilds. It will be kept disabled until 2024-08-23. During this time, it will not be possible to add new API Tokens or verify your accounts.\nSee: [Guild Wars 2 API Disabled from August 16-23](https://en-forum.guildwars2.com/topic/149447-guild-wars-2-api-disabled-from-august-16-23)",
				})
			}
		}

		return c.JSON(http.StatusOK, notifications)
	}
}
