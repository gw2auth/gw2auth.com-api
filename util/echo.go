package util

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
)

func EchoAllParams[T any](c echo.Context, fn func(string) (T, error), params ...string) ([]T, error) {
	r := make([]T, 0, len(params))
	for _, p := range params {
		if v, err := fn(c.Param(p)); err != nil {
			return nil, err
		} else {
			r = append(r, v)
		}
	}

	return r, nil
}

func NewEchoPgxHTTPError(err error) *echo.HTTPError {
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, err)
	}

	return echo.NewHTTPError(http.StatusInternalServerError, err)
}
