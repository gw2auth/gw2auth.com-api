package gw2

var nthBitConversion = [10]Permission{
	PermissionAccount,
	PermissionBuilds,
	PermissionCharacters,
	PermissionGuilds,
	PermissionInventories,
	PermissionProgression,
	PermissionPvp,
	PermissionTradingpost,
	PermissionUnlocks,
	PermissionWallet,
}

func PermissionsFromBitSet(bitSet int32) []Permission {
	res := make([]Permission, 0)
	for i, v := range nthBitConversion {
		if bitSet&(1<<i) != 0 {
			res = append(res, v)
		}
	}

	return res
}

func PermissionsToBitSet(perms []Permission) int32 {
	var res int32
	for _, perm := range perms {
		for i, v := range nthBitConversion {
			if perm == v {
				res |= 1 << i
				break
			}
		}
	}

	return res
}
