package auth

import (
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/util"
	"time"
)

type Permission string

const (
	PermissionRead         Permission = "read"
	PermissionClientCreate Permission = "client:create"
	PermissionClientModify Permission = "client:modify"
)

type ApiKey struct {
	Id            uuid.UUID
	ApplicationId uuid.UUID
	Permissions   []Permission
	NotBefore     time.Time
	ExpiresAt     time.Time
	AccountId     uuid.UUID
}

func FilterPermissions(perms []Permission) []Permission {
	s := util.NewSet(PermissionRead, PermissionClientCreate, PermissionClientModify)
	r := make([]Permission, 0, len(perms))

	for _, v := range perms {
		if s.Remove(v) {
			r = append(r, v)
		}

		if len(s) < 1 {
			break
		}
	}

	return r
}
