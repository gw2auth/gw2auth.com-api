package gw2

import "github.com/gofrs/uuid/v5"

type Permission string

const (
	PermissionAccount     Permission = "account"
	PermissionBuilds      Permission = "builds"
	PermissionCharacters  Permission = "characters"
	PermissionGuilds      Permission = "guilds"
	PermissionInventories Permission = "inventories"
	PermissionProgression Permission = "progression"
	PermissionPvp         Permission = "pvp"
	PermissionTradingpost Permission = "tradingpost"
	PermissionUnlocks     Permission = "unlocks"
	PermissionWallet      Permission = "wallet"
)

type Account struct {
	Id   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type TokenInfo struct {
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
}
