package auth

func HasPermission(identity *Identity, permission string) bool {
	if identity == nil || permission == "" {
		return false
	}
	for _, item := range identity.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}
