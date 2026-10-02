package models

import (
	"fmt"
	"regexp"
)

// The AH5 naming rules enforced by the ServiceRegistry discovery endpoints
// (CONTRACT.md, "AH5 names"). A name that breaks them is refused with 400.
var (
	systemNameRe        = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,62}$`)
	serviceDefinitionRe = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
)

// ValidateSystemName reports whether name is a valid AH5 system name:
// PascalCase, for example "TemperatureProvider".
func ValidateSystemName(name string) error {
	if !systemNameRe.MatchString(name) {
		return fmt.Errorf("system name %q is not PascalCase (^[A-Z][A-Za-z0-9]{0,62}$)", name)
	}
	return nil
}

// ValidateServiceDefinitionName reports whether name is a valid AH5 service
// definition name: camelCase, for example "temperatureService".
func ValidateServiceDefinitionName(name string) error {
	if !serviceDefinitionRe.MatchString(name) {
		return fmt.Errorf("service definition name %q is not camelCase (^[a-z][A-Za-z0-9]{0,62}$)", name)
	}
	return nil
}
