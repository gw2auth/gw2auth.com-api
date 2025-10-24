package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
)

type gw2StatusRoute struct {
	Path   string `json:"path"`
	Active bool   `json:"active"`
}

type gw2StatusResponse struct {
	Routes []gw2StatusRoute `json:"routes"`
}

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
	apiDowntimeStart := time.Unix(1761325200, 0)
	apiDowntimeEnd := time.Unix(1761843600, 0)
	relevantEndpoints := []string{
		"/v2/tokeninfo",
		"/v2/account",
		"/v2/characters",
		"/v2/createsubtoken",
		"/v2/commerce/transactions/current/buys",
	}

	return func(c echo.Context) error {
		ctx := c.Request().Context()
		g, gCtx := errgroup.WithContext(ctx)

		var disabledEndpoints []string
		var endpointsWithIssues []string

		g.Go(func() error {
			req, err := http.NewRequestWithContext(gCtx, http.MethodGet, "https://api.guildwars2.com/v2.json", nil)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}

			req.Header.Set("User-Agent", "GW2Auth")

			res, err := httpClient.Do(req)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, err)
			}

			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				return echo.NewHTTPError(http.StatusBadGateway, fmt.Errorf("unexpected status code: %d", res.StatusCode))
			}

			var gw2Status gw2StatusResponse
			if err = json.NewDecoder(res.Body).Decode(&gw2Status); err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, err)
			}

			for _, element := range gw2Status.Routes {
				if slices.Contains(relevantEndpoints, element.Path) && !element.Active {
					disabledEndpoints = append(disabledEndpoints, element.Path)
				}
			}

			return nil
		})

		g.Go(func() error {
			req, err := http.NewRequestWithContext(gCtx, http.MethodGet, "https://status.gw2efficiency.com/api", nil)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}

			req.Header.Set("User-Agent", "GW2Auth")

			res, err := httpClient.Do(req)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, err)
			}

			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				return echo.NewHTTPError(http.StatusBadGateway, fmt.Errorf("unexpected status code: %d", res.StatusCode))
			}

			var gw2EffStatus gw2EffApiStatusResponse
			if err = json.NewDecoder(res.Body).Decode(&gw2EffStatus); err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, err)
			}

			for _, element := range gw2EffStatus.Data {
				if slices.Contains(relevantEndpoints, element.Name) && (element.Status != http.StatusOK || element.CheckError() || element.Duration >= 15_000) {
					endpointsWithIssues = append(endpointsWithIssues, element.Name)
				}
			}

			return nil
		})

		if err := g.Wait(); err != nil {
			var httpErr *echo.HTTPError
			if errors.As(err, &httpErr) {
				return httpErr
			} else {
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
		}

		notifications := make([]notification, 0)

		if len(disabledEndpoints) > 0 {
			notifications = append(notifications, notification{
				Type:    notificationTypeError,
				Header:  "The Guild Wars 2 API is unavailable",
				Content: "Some of the endpoints used by GW2Auth are currently disabled. This might impact your experience with GW2Auth and Applications using GW2Auth.",
			})
		} else if len(endpointsWithIssues) > 0 {
			notifications = append(notifications, notification{
				Type:    notificationTypeWarning,
				Header:  "The Guild Wars 2 API experiences issues right now",
				Content: "Some of the endpoints used by GW2Auth appear to be in a degraded state right now. This might impact your experience with GW2Auth and Applications using GW2Auth.",
			})
		}

		now := time.Now()
		if now.After(apiDowntimeStart) && now.Before(apiDowntimeEnd) {
			notifications = append(notifications, notification{
				Type:   notificationTypeError,
				Header: "VoE Release - Guild Wars 2 API Disabled",
				Content: fmt.Sprintf(
					"The Guild Wars 2 API will be disabled until %s to prevent spoilers of Visions of Eternity. While the API is disabled, you will not be able to add or update API Tokens or verify accounts.\nSee: [Guild Wars 2 API Disabled from October 24–30](https://en-forum.guildwars2.com/topic/163562-guild-wars-2-api-disabled-from-october-24%%E2%%80%%9330/)",
					apiDowntimeEnd.Format(time.RFC3339),
				),
			})
		}

		return c.JSON(http.StatusOK, notifications)
	}
}
