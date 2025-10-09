package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
)

type apiResult[T any] struct {
	Value T
	Error error
}

type gw2StatusRoute struct {
	Path   string `json:"path"`
	Lang   string `json:"lang"`
	Auth   string `json:"auth"`
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

		gw2Chan := make(chan apiResult[[]string], 1)
		gw2EffChan := make(chan apiResult[[]string], 1)

		g.Go(func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.guildwars2.com/v2.json", nil)
			if err != nil {
				gw2Chan <- apiResult[[]string]{Error: err}
				return err
			}

			req.Header.Set("User-Agent", "GW2Auth")

			res, err := httpClient.Do(req)
			if err != nil || res.StatusCode != http.StatusOK {
				gw2Chan <- apiResult[[]string]{Error: err}
				return nil
			}

			var gw2Status gw2StatusResponse
			if err = json.NewDecoder(res.Body).Decode(&gw2Status); err != nil {
				gw2Chan <- apiResult[[]string]{Error: err}
				return nil
			}

			var disabledEndpoints []string
			for _, element := range gw2Status.Routes {
				if slices.Contains(relevantEndpoints, element.Path) && !element.Active {
					disabledEndpoints = append(disabledEndpoints, element.Path)
				}
			}

			gw2Chan <- apiResult[[]string]{Value: disabledEndpoints}

			defer res.Body.Close()
			return nil
		})

		g.Go(func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://status.gw2efficiency.com/api", nil)
			if err != nil {
				return err
			}

			req.Header.Set("User-Agent", "GW2Auth")

			res, err := httpClient.Do(req)
			if err != nil {
				gw2Chan <- apiResult[[]string]{Error: err}
				return nil
			}

			if res.StatusCode != http.StatusOK {
				gw2Chan <- apiResult[[]string]{Error: errors.New(res.Status)}
				return nil
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

			defer res.Body.Close()
			return nil
		})

		var notifications []notification

		gw2Result := <-gw2Chan

		if gw2Result.Error == nil {
			disabledEndpoints := gw2Result.Value

			if len(disabledEndpoints) > 0 {
				notifications = []notification{
					{
						Type:    notificationTypeError,
						Header:  "The Guild Wars 2 API is unavailable",
						Content: "Some of the endpoints used by GW2Auth are currently disabled. This might impact your experience with GW2Auth and Applications using GW2Auth.",
					},
				}
			} else {
				notifications = make([]notification, 0)
			}
		} else {
			gw2EffResult := <-gw2EffChan

			if gw2EffResult.Error == nil {
				return echo.NewHTTPError(http.StatusBadGateway, gw2EffResult.Error)
			}

			endpointsWithIssues := gw2EffResult.Value
			if len(endpointsWithIssues) > 0 {
				notifications = []notification{
					{
						Type:    notificationTypeWarning,
						Header:  "The Guild Wars 2 API experiences issues right now",
						Content: "Some of the endpoints used by GW2Auth appear to be in a degraded state right now. This might impact your experience with GW2Auth and Applications using GW2Auth.",
					},
				}
			} else {
				notifications = make([]notification, 0)
			}
		}

		now := time.Now()
		if now.Before(apiDowntimeEnd) {
			notifications = append(notifications, notification{
				Type:    notificationTypeWarning,
				Header:  "Issues with the Guild Wars 2 API",
				Content: "Please be aware of issues with the official Guild Wars 2 API. Applications using GW2Auth might not work properly. GW2Auth itself is not affected.\nSee: [Issues with the Guild Wars 2 API](https://en-forum.guildwars2.com/topic/153373-issues-with-the-guild-wars-2-api/)",
			})
		}

		return c.JSON(http.StatusOK, notifications)
	}
}
