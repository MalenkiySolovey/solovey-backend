package validation

import "github.com/MalenkiySolovey/solovey-backend/security/ssrf"

func ValidateProxyURLValue(value string, storedSecretMarker string) error {
	if value == "" || value == storedSecretMarker {
		return nil
	}
	return ssrf.ValidateProxyURL(value)
}
