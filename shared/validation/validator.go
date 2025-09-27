// shared/validation/validator.go - SIMPLIFIED VERSION
package validation

import (
    "fmt"
    "regexp"
    "strings"
)

type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
    Code    string `json:"code"`
}

type Validator struct {
    errors []ValidationError
}

func New() *Validator {
    return &Validator{}
}

func (v *Validator) AddError(field, message string) {
    v.errors = append(v.errors, ValidationError{
        Field:   field,
        Message: message,
        Code:    "VALIDATION_ERROR",
    })
}

func (v *Validator) Required(field, value string) {
    if strings.TrimSpace(value) == "" {
        v.AddError(field, fmt.Sprintf("%s is required", field))
    }
}

func (v *Validator) Email(field, value string) {
    if value == "" {
        return
    }
    emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
    if !emailRegex.MatchString(value) {
        v.AddError(field, "Invalid email format")
    }
}

func (v *Validator) MinLength(field, value string, min int) {
    if len(value) < min {
        v.AddError(field, fmt.Sprintf("%s must be at least %d characters", field, min))
    }
}

func (v *Validator) MaxLength(field, value string, max int) {
    if len(value) > max {
        v.AddError(field, fmt.Sprintf("%s must be at most %d characters", field, max))
    }
}

func (v *Validator) AccountCode(field, value string) {
    if value == "" {
        return
    }
    codeRegex := regexp.MustCompile(`^\d{4}$`)
    if !codeRegex.MatchString(value) {
        v.AddError(field, "Account code must be 4 digits")
    }
}

func (v *Validator) IndonesianTaxID(field, value string) {
    if value == "" {
        return
    }
    taxRegex := regexp.MustCompile(`^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$`)
    if !taxRegex.MatchString(value) {
        v.AddError(field, "Invalid Indonesian Tax ID format")
    }
}

func (v *Validator) OneOf(field, value string, validOptions []string) {
    if value == "" {
        return
    }
    for _, option := range validOptions {
        if value == option {
            return
        }
    }
    v.AddError(field, fmt.Sprintf("%s must be one of: %s", field, strings.Join(validOptions, ", ")))
}

func (v *Validator) PositiveNumber(field string, value float64) {
    if value <= 0 {
        v.AddError(field, fmt.Sprintf("%s must be positive", field))
    }
}

func (v *Validator) IsValid() bool {
    return len(v.errors) == 0
}

func (v *Validator) Errors() []ValidationError {
    return v.errors
}