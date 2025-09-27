// shared/validation/validator.go - OPTIMIZED VERSION
package validation

import (
    "fmt"
    "regexp"
    "strconv"
    "strings"
    "time"
    "unicode"
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
    v.AddErrorWithCode(field, message, "VALIDATION_ERROR")
}

func (v *Validator) AddErrorWithCode(field, message, code string) {
    v.errors = append(v.errors, ValidationError{
        Field:   field,
        Message: message,
        Code:    code,
    })
}

// Core validations
func (v *Validator) Required(field, value string) {
    if strings.TrimSpace(value) == "" {
        v.AddErrorWithCode(field, fmt.Sprintf("%s is required", field), "REQUIRED")
    }
}

func (v *Validator) Email(field, value string) {
    if value == "" {
        return
    }
    emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
    if !emailRegex.MatchString(value) {
        v.AddErrorWithCode(field, "Invalid email format", "INVALID_EMAIL")
    }
}

func (v *Validator) MinLength(field, value string, min int) {
    if len(value) < min {
        v.AddErrorWithCode(field, fmt.Sprintf("%s must be at least %d characters", field, min), "MIN_LENGTH")
    }
}

func (v *Validator) MaxLength(field, value string, max int) {
    if len(value) > max {
        v.AddErrorWithCode(field, fmt.Sprintf("%s must be at most %d characters", field, max), "MAX_LENGTH")
    }
}

// Indonesian specific validations
func (v *Validator) IndonesianTaxID(field, value string) {
    if value == "" {
        return
    }
    taxRegex := regexp.MustCompile(`^\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}$`)
    if !taxRegex.MatchString(value) {
        v.AddErrorWithCode(field, "Invalid Indonesian Tax ID format (XX.XXX.XXX.X-XXX.XXX)", "INVALID_TAX_ID")
    }
}

func (v *Validator) AccountCode(field, value string) {
    if value == "" {
        return
    }
    codeRegex := regexp.MustCompile(`^\d{4}$`)
    if !codeRegex.MatchString(value) {
        v.AddErrorWithCode(field, "Account code must be 4 digits", "INVALID_ACCOUNT_CODE")
    }
}

func (v *Validator) IndonesianPhone(field, value string) {
    if value == "" {
        return
    }
    phoneRegex := regexp.MustCompile(`^(\+62|62|0)[0-9]{8,12}$`)
    if !phoneRegex.MatchString(value) {
        v.AddErrorWithCode(field, "Invalid Indonesian phone number format", "INVALID_PHONE")
    }
}

func (v *Validator) StrongPassword(field, value string) {
    if len(value) < 8 {
        v.AddErrorWithCode(field, "Password must be at least 8 characters", "WEAK_PASSWORD")
        return
    }
    
    var hasUpper, hasLower, hasNumber, hasSpecial bool
    for _, char := range value {
        switch {
        case unicode.IsUpper(char):
            hasUpper = true
        case unicode.IsLower(char):
            hasLower = true
        case unicode.IsDigit(char):
            hasNumber = true
        case unicode.IsPunct(char) || unicode.IsSymbol(char):
            hasSpecial = true
        }
    }
    
    var missing []string
    if !hasUpper { missing = append(missing, "uppercase letter") }
    if !hasLower { missing = append(missing, "lowercase letter") }
    if !hasNumber { missing = append(missing, "number") }
    if !hasSpecial { missing = append(missing, "special character") }
    
    if len(missing) > 0 {
        v.AddErrorWithCode(field, fmt.Sprintf("Password must contain: %s", strings.Join(missing, ", ")), "WEAK_PASSWORD")
    }
}

// Business validations
func (v *Validator) OneOf(field, value string, validOptions []string) {
    if value == "" {
        return
    }
    for _, option := range validOptions {
        if value == option {
            return
        }
    }
    v.AddErrorWithCode(field, fmt.Sprintf("%s must be one of: %s", field, strings.Join(validOptions, ", ")), "INVALID_OPTION")
}

func (v *Validator) PositiveNumber(field string, value float64) {
    if value <= 0 {
        v.AddErrorWithCode(field, fmt.Sprintf("%s must be positive", field), "INVALID_POSITIVE")
    }
}

func (v *Validator) CurrencyAmount(field string, amount float64) {
    if amount < 0 {
        v.AddErrorWithCode(field, "Amount cannot be negative", "INVALID_AMOUNT")
    }
    
    // Indonesian Rupiah validation (no decimals)
    if amount != float64(int64(amount)) {
        v.AddErrorWithCode(field, "Indonesian Rupiah amounts should not have decimal places", "INVALID_CURRENCY_PRECISION")
    }
}

func (v *Validator) ValidAccountType(field, accountType string) {
    validTypes := []string{"Asset", "Liability", "Equity", "Revenue", "Expense"}
    v.OneOf(field, accountType, validTypes)
}

// Result methods
func (v *Validator) IsValid() bool {
    return len(v.errors) == 0
}

func (v *Validator) Errors() []ValidationError {
    return v.errors
}

func (v *Validator) ErrorCount() int {
    return len(v.errors)
}

func (v *Validator) ClearErrors() {
    v.errors = []ValidationError{}
}