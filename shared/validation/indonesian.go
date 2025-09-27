// shared/validation/indonesian.go - ENHANCED VERSION
package validation

import (
    "fmt"
    "regexp"
    "strconv"
    "strings"
    "time"
    "unicode"
)

// Indonesian-specific validation rules
type IndonesianValidator struct {
    *Validator
}

func NewIndonesianValidator() *IndonesianValidator {
    return &IndonesianValidator{
        Validator: New(),
    }
}

// Enhanced NPWP validation with full checksum algorithm
func (iv *IndonesianValidator) ValidateNPWP(field, value string) {
    if value == "" {
        return
    }
    
    // Remove all non-digit characters
    cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(value, "")
    
    if len(cleaned) != 15 {
        iv.AddErrorWithCode(field, "NPWP must be 15 digits", "INVALID_NPWP_LENGTH")
        return
    }
    
    // Validate tax office code (first 2 digits)
    taxOfficeCode := cleaned[0:2]
    if !iv.isValidTaxOfficeCode(taxOfficeCode) {
        iv.AddErrorWithCode(field, "Invalid NPWP tax office code", "INVALID_NPWP_OFFICE")
        return
    }
    
    // Validate sequence number (next 6 digits) - must not be all zeros
    sequenceNumber := cleaned[2:8]
    if sequenceNumber == "000000" {
        iv.AddErrorWithCode(field, "Invalid NPWP sequence number", "INVALID_NPWP_SEQUENCE")
        return
    }
    
    // Validate checksum using proper algorithm
    if !iv.validateNPWPChecksum(cleaned) {
        iv.AddErrorWithCode(field, "Invalid NPWP checksum", "INVALID_NPWP_CHECKSUM")
    }
}

func (iv *IndonesianValidator) isValidTaxOfficeCode(code string) bool {
    // Complete list of valid Indonesian tax office codes
    validCodes := map[string]bool{
        "01": true, "02": true, "03": true, "04": true, "05": true,
        "06": true, "07": true, "08": true, "09": true, "10": true,
        "11": true, "12": true, "13": true, "14": true, "15": true,
        "16": true, "17": true, "18": true, "19": true, "20": true,
        "21": true, "22": true, "23": true, "24": true, "25": true,
        "26": true, "27": true, "28": true, "29": true, "30": true,
        "31": true, "32": true, "33": true, "34": true, "35": true,
        "71": true, // Special codes for certain entities
    }
    return validCodes[code]
}

func (iv *IndonesianValidator) validateNPWPChecksum(npwp string) bool {
    // Proper NPWP checksum algorithm
    weights := []int{1, 2, 1, 2, 1, 2, 1, 2}
    sum := 0
    
    for i := 0; i < 8; i++ {
        digit, _ := strconv.Atoi(string(npwp[i]))
        product := digit * weights[i]
        if product > 9 {
            product = (product / 10) + (product % 10)
        }
        sum += product
    }
    
    checkDigit := (10 - (sum % 10)) % 10
    expectedCheck, _ := strconv.Atoi(string(npwp[8]))
    
    return checkDigit == expectedCheck
}

// Enhanced NIK validation with comprehensive checks
func (iv *IndonesianValidator) ValidateNIK(field, value string) {
    if value == "" {
        return
    }
    
    cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(value, "")
    
    if len(cleaned) != 16 {
        iv.AddErrorWithCode(field, "NIK must be 16 digits", "INVALID_NIK_LENGTH")
        return
    }
    
    // Validate province code
    provinceCode := cleaned[0:2]
    if !iv.isValidProvinceCode(provinceCode) {
        iv.AddErrorWithCode(field, "Invalid province code in NIK", "INVALID_NIK_PROVINCE")
        return
    }
    
    // Validate city/regency code
    cityCode := cleaned[2:4]
    if cityCode == "00" {
        iv.AddErrorWithCode(field, "Invalid city/regency code in NIK", "INVALID_NIK_CITY")
        return
    }
    
    // Validate district code
    districtCode := cleaned[4:6]
    if districtCode == "00" {
        iv.AddErrorWithCode(field, "Invalid district code in NIK", "INVALID_NIK_DISTRICT")
        return
    }
    
    // Validate birth date
    if !iv.validateNIKBirthDate(cleaned[6:12]) {
        iv.AddErrorWithCode(field, "Invalid birth date in NIK", "INVALID_NIK_BIRTHDATE")
        return
    }
    
    // Validate sequence number (last 4 digits)
    sequenceNumber := cleaned[12:16]
    if sequenceNumber == "0000" {
        iv.AddErrorWithCode(field, "Invalid sequence number in NIK", "INVALID_NIK_SEQUENCE")
    }
}

func (iv *IndonesianValidator) validateNIKBirthDate(birthDateStr string) bool {
    day, _ := strconv.Atoi(birthDateStr[0:2])
    month, _ := strconv.Atoi(birthDateStr[2:4])
    year, _ := strconv.Atoi(birthDateStr[4:6])
    
    // For females, day is increased by 40
    originalDay := day
    if day > 40 {
        day -= 40
    }
    
    // Validate day and month ranges
    if day < 1 || day > 31 || month < 1 || month > 12 {
        return false
    }
    
    // Convert 2-digit year to 4-digit year
    currentYear := time.Now().Year()
    century := currentYear / 100 * 100
    if year > (currentYear % 100) {
        year += century - 100
    } else {
        year += century
    }
    
    // Validate the actual date
    birthDate := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
    _, err := time.Parse("2006-01-02", birthDate)
    if err != nil {
        return false
    }
    
    // Check if birth date is not in the future
    parsedDate, _ := time.Parse("2006-01-02", birthDate)
    if parsedDate.After(time.Now()) {
        return false
    }
    
    return true
}

func (iv *IndonesianValidator) isValidProvinceCode(code string) bool {
    // Complete list of Indonesian province codes
    validCodes := map[string]string{
        "11": "Aceh",
        "12": "Sumatera Utara",
        "13": "Sumatera Barat", 
        "14": "Riau",
        "15": "Jambi",
        "16": "Sumatera Selatan",
        "17": "Bengkulu",
        "18": "Lampung",
        "19": "Kepulauan Bangka Belitung",
        "21": "Kepulauan Riau",
        "31": "DKI Jakarta",
        "32": "Jawa Barat",
        "33": "Jawa Tengah",
        "34": "DI Yogyakarta",
        "35": "Jawa Timur",
        "36": "Banten",
        "51": "Bali",
        "52": "Nusa Tenggara Barat",
        "53": "Nusa Tenggara Timur",
        "61": "Kalimantan Barat",
        "62": "Kalimantan Tengah",
        "63": "Kalimantan Selatan",
        "64": "Kalimantan Timur",
        "65": "Kalimantan Utara",
        "71": "Sulawesi Utara",
        "72": "Sulawesi Tengah",
        "73": "Sulawesi Selatan",
        "74": "Sulawesi Tenggara",
        "75": "Gorontalo",
        "76": "Sulawesi Barat",
        "81": "Maluku",
        "82": "Maluku Utara",
        "91": "Papua Barat",
        "94": "Papua",
        "95": "Papua Tengah",
        "96": "Papua Pegunungan",
        "97": "Papua Selatan",
        "98": "Papua Barat Daya",
    }
    
    _, exists := validCodes[code]
    return exists
}

// Enhanced Indonesian phone number validation
func (iv *IndonesianValidator) ValidateIndonesianPhone(field, value string) {
    if value == "" {
        return
    }
    
    // Clean and normalize
    cleaned := regexp.MustCompile(`[^\d+]`).ReplaceAllString(value, "")
    
    // Normalize different formats
    if strings.HasPrefix(cleaned, "0") {
        cleaned = "62" + cleaned[1:]
    } else if strings.HasPrefix(cleaned, "+62") {
        cleaned = cleaned[1:]
    } else if !strings.HasPrefix(cleaned, "62") {
        iv.AddErrorWithCode(field, "Phone number must be Indonesian (+62)", "INVALID_PHONE_COUNTRY")
        return
    }
    
    number := cleaned[2:] // Remove country code
    
    // Validate length
    if len(number) < 8 || len(number) > 13 {
        iv.AddErrorWithCode(field, "Invalid Indonesian phone number length", "INVALID_PHONE_LENGTH")
        return
    }
    
    // Validate mobile prefixes (more comprehensive)
    if number[0] == '8' {
        // Mobile numbers
        validMobilePrefixes := []string{
            "811", "812", "813", "821", "822", "823", "851", "852", "853", // Telkomsel
            "814", "815", "816", "855", "856", "857", "858", // Indosat
            "817", "818", "819", "859", "877", "878", // XL
            "838", "831", "832", "833", // Axis
            "896", "897", "898", "899", // Three
            "881", "882", "883", "884", "885", "886", "887", "888", // Smartfren
        }
        
        isValidMobile := false
        for _, prefix := range validMobilePrefixes {
            if strings.HasPrefix(number, prefix) {
                isValidMobile = true
                break
            }
        }
        
        if !isValidMobile {
            iv.AddErrorWithCode(field, "Invalid Indonesian mobile number prefix", "INVALID_MOBILE_PREFIX")
        }
        return
    }
    
    // Landline validation by area code
    validLandlinePrefixes := map[string]string{
        "21":  "Jakarta",
        "22":  "Bandung", 
        "24":  "Semarang",
        "31":  "Surabaya",
        "61":  "Medan",
        "274": "Yogyakarta",
        "341": "Malang",
        "271": "Solo",
    }
    
    isValidLandline := false
    for prefix := range validLandlinePrefixes {
        if strings.HasPrefix(number, prefix) {
            isValidLandline = true
            break
        }
    }
    
    if !isValidLandline {
        iv.AddErrorWithCode(field, "Invalid Indonesian phone number prefix", "INVALID_PHONE_PREFIX")
    }
}

// Indonesian currency amount validation
func (iv *IndonesianValidator) ValidateRupiahAmount(field string, amount float64) {
    if amount < 0 {
        iv.AddErrorWithCode(field, "Amount cannot be negative", "NEGATIVE_AMOUNT")
        return
    }
    
    // Check for decimal places (IDR typically doesn't use decimals)
    if amount != float64(int64(amount)) {
        iv.AddErrorWithCode(field, "Indonesian Rupiah should not have decimal places", "INVALID_RUPIAH_DECIMALS")
    }
    
    // Validate reasonable limits for Indonesian context
    if amount > 999999999999999 { // 999.9 trillion IDR
        iv.AddErrorWithCode(field, "Amount exceeds maximum allowed value", "AMOUNT_TOO_LARGE")
    }
    
    // Minimum transaction amount (avoid micro-transactions)
    if amount > 0 && amount < 100 {
        iv.AddErrorWithCode(field, "Amount too small for Indonesian Rupiah transactions", "AMOUNT_TOO_SMALL")
    }
}

// Indonesian business license validation  
func (iv *IndonesianValidator) ValidateBusinessLicense(field, licenseType, value string) {
    if value == "" {
        return
    }
    
    switch strings.ToUpper(licenseType) {
    case "NIB":
        // Nomor Induk Berusaha (new unified business license)
        cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(value, "")
        if len(cleaned) != 13 {
            iv.AddErrorWithCode(field, "NIB must be 13 digits", "INVALID_NIB_LENGTH")
        }
    case "SIUP":
        // Surat Izin Usaha Perdagangan (legacy)
        pattern := regexp.MustCompile(`^\d{3,4}/[\d\-\.A-Z]+/[A-Z\-]+/\d{4}$`)
        if !pattern.MatchString(value) {
            iv.AddErrorWithCode(field, "Invalid SIUP format", "INVALID_SIUP_FORMAT")
        }
    case "TDP":
        // Tanda Daftar Perusahaan
        cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(value, "")
        if len(cleaned) < 8 || len(cleaned) > 10 {
            iv.AddErrorWithCode(field, "TDP must be 8-10 digits", "INVALID_TDP_FORMAT")
        }
    default:
        iv.AddErrorWithCode(field, "Unknown license type. Valid types: NIB, SIUP, TDP", "INVALID_LICENSE_TYPE")
    }
}

// Indonesian bank account validation
func (iv *IndonesianValidator) ValidateBankAccount(field, bankCode, accountNumber string) {
    if accountNumber == "" {
        return
    }
    
    cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(accountNumber, "")
    
    // Indonesian bank account validation by bank code
    bankValidation := map[string]struct {
        minLength int
        maxLength int
        name      string
    }{
        "002": {10, 13, "BRI"},
        "008": {10, 13, "Mandiri"},
        "009": {10, 13, "BNI"},
        "011": {10, 13, "Danamon"},
        "013": {10, 13, "Permata"},
        "014": {10, 15, "BCA"},
        "016": {10, 13, "Maybank"},
        "022": {10, 13, "CIMB Niaga"},
        "023": {10, 13, "UOB Indonesia"},
        "213": {10, 16, "BTN"},
        "451": {10, 16, "BSI"},
    }
    
    if validation, exists := bankValidation[bankCode]; exists {
        if len(cleaned) < validation.minLength || len(cleaned) > validation.maxLength {
            iv.AddErrorWithCode(field, 
                fmt.Sprintf("Invalid %s account number length (must be %d-%d digits)", 
                    validation.name, validation.minLength, validation.maxLength),
                "INVALID_BANK_ACCOUNT_LENGTH")
        }
    } else {
        // Generic validation for unknown banks
        if len(cleaned) < 8 || len(cleaned) > 20 {
            iv.AddErrorWithCode(field, "Bank account number must be 8-20 digits", "INVALID_BANK_ACCOUNT_LENGTH")
        }
    }
}

// Indonesian postal code validation  
func (iv *IndonesianValidator) ValidateIndonesianPostalCode(field, value string) {
    if value == "" {
        return
    }
    
    cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(value, "")
    
    if len(cleaned) != 5 {
        iv.AddErrorWithCode(field, "Indonesian postal code must be 5 digits", "INVALID_POSTAL_CODE_LENGTH")
        return
    }
    
    // Validate first digit (region)
    firstDigit := cleaned[0]
    validRegions := map[byte]string{
        '1': "Jakarta, Banten, West Java",
        '2': "West Java", 
        '3': "Central Java",
        '4': "East Java, Bali",
        '5': "East Java, NTT, NTB",
        '6': "Kalimantan",
        '7': "Sulawesi",
        '8': "Bali, NTB, NTT, Maluku",
        '9': "Maluku, Papua",
    }
    
    if _, valid := validRegions[firstDigit]; !valid {
        iv.AddErrorWithCode(field, "Invalid Indonesian postal code region", "INVALID_POSTAL_CODE_REGION")
    }
}