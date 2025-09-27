// currency-service/main.go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "sync"
    "time"
    
    "github.com/gorilla/mux"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
    "github.com/massehanto/accounting-system-go/shared/service"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type CurrencyService struct {
    *service.BaseService
    rates       map[string]Currency
    mutex       sync.RWMutex
    lastUpdated time.Time
    apiKey      string
}

type Currency struct {
    Code        string    `json:"code"`
    Name        string    `json:"name"`
    Rate        float64   `json:"rate"`
    LastUpdated time.Time `json:"last_updated"`
}

type ConversionRequest struct {
    Amount float64 `json:"amount"`
    From   string  `json:"from"`
    To     string  `json:"to"`
}

type ConversionResponse struct {
    OriginalAmount  float64   `json:"original_amount"`
    ConvertedAmount float64   `json:"converted_amount"`
    FromCurrency    string    `json:"from_currency"`
    ToCurrency      string    `json:"to_currency"`
    ExchangeRate    float64   `json:"exchange_rate"`
    ConvertedAt     time.Time `json:"converted_at"`
}

type ExchangeAPIResponse struct {
    Success bool               `json:"success"`
    Base    string             `json:"base"`
    Date    string             `json:"date"`
    Rates   map[string]float64 `json:"rates"`
}

func main() {
    cfg := config.Load()
    
    currencyService := &CurrencyService{
        BaseService: &service.BaseService{DB: nil},
        rates: map[string]Currency{
            "IDR": {Code: "IDR", Name: "Indonesian Rupiah", Rate: 1.0, LastUpdated: time.Now()},
            "USD": {Code: "USD", Name: "US Dollar", Rate: 15000.0, LastUpdated: time.Now()},
            "EUR": {Code: "EUR", Name: "Euro", Rate: 16500.0, LastUpdated: time.Now()},
            "SGD": {Code: "SGD", Name: "Singapore Dollar", Rate: 11000.0, LastUpdated: time.Now()},
            "MYR": {Code: "MYR", Name: "Malaysian Ringgit", Rate: 3500.0, LastUpdated: time.Now()},
        },
        lastUpdated: time.Now(),
        apiKey:      getEnv("EXCHANGE_API_KEY", ""),
    }
    
    if currencyService.apiKey != "" {
        go currencyService.startRateUpdates()
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(nil, "currency-service")).Methods("GET")
    
    r.Handle("/convert", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.RateLimit(100),
        middleware.LoggingMiddleware,
    )(currencyService.convertCurrencyHandler)).Methods("POST")
    
    r.Handle("/rates", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.RateLimit(200),
        middleware.LoggingMiddleware,
    )(currencyService.getRatesHandler)).Methods("GET")
    
    r.Handle("/rates/{code}", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.LoggingMiddleware,
    )(currencyService.getRateHandler)).Methods("GET")
    
    r.Handle("/rates/update", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.RateLimit(10),
        middleware.LoggingMiddleware,
    )(currencyService.updateRatesHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (cs *CurrencyService) startRateUpdates() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        if err := cs.fetchExchangeRates(); err != nil {
            fmt.Printf("Failed to update exchange rates: %v\n", err)
        }
    }
}

func (cs *CurrencyService) fetchExchangeRates() error {
    url := fmt.Sprintf("https://api.exchangeratesapi.io/v1/latest?access_key=%s&base=IDR&symbols=USD,EUR,SGD,MYR", cs.apiKey)
    
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }
    
    var apiResp ExchangeAPIResponse
    if err := json.Unmarshal(body, &apiResp); err != nil {
        return err
    }
    
    if !apiResp.Success {
        return fmt.Errorf("API request failed")
    }
    
    cs.mutex.Lock()
    defer cs.mutex.Unlock()
    
    now := time.Now()
    
    for code, rate := range apiResp.Rates {
        if currency, exists := cs.rates[code]; exists {
            currency.Rate = rate
            currency.LastUpdated = now
            cs.rates[code] = currency
        }
    }
    
    cs.lastUpdated = now
    return nil
}

func (cs *CurrencyService) convertCurrencyHandler(w http.ResponseWriter, r *http.Request) {
    var req ConversionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        cs.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    if req.Amount <= 0 {
        validator.AddError("amount", "Amount must be positive")
    }
    validator.Required("from", req.From)
    validator.Required("to", req.To)
    
    if !validator.IsValid() {
        cs.RespondValidationError(w, validator.Errors())
        return
    }

    converted, exchangeRate, ok := cs.convertAmount(req.Amount, req.From, req.To)
    if !ok {
        cs.RespondWithError(w, http.StatusBadRequest, "INVALID_CURRENCY", "Invalid currency codes")
        return
    }
    
    response := ConversionResponse{
        OriginalAmount:  req.Amount,
        ConvertedAmount: converted,
        FromCurrency:    req.From,
        ToCurrency:      req.To,
        ExchangeRate:    exchangeRate,
        ConvertedAt:     time.Now(),
    }
    
    cs.RespondWithJSON(w, http.StatusOK, response)
}

func (cs *CurrencyService) convertAmount(amount float64, from, to string) (float64, float64, bool) {
    if from == to {
        return amount, 1.0, true
    }
    
    cs.mutex.RLock()
    defer cs.mutex.RUnlock()
    
    fromCurrency, okFrom := cs.rates[from]
    toCurrency, okTo := cs.rates[to]
    
    if !okFrom || !okTo {
        return 0, 0, false
    }
    
    baseAmount := amount / fromCurrency.Rate
    convertedAmount := baseAmount * toCurrency.Rate
    exchangeRate := toCurrency.Rate / fromCurrency.Rate
    
    return convertedAmount, exchangeRate, true
}

func (cs *CurrencyService) getRatesHandler(w http.ResponseWriter, r *http.Request) {
    cs.mutex.RLock()
    defer cs.mutex.RUnlock()
    
    currencies := make([]Currency, 0, len(cs.rates))
    for _, currency := range cs.rates {
        currencies = append(currencies, currency)
    }
    
    response := map[string]interface{}{
        "rates":        currencies,
        "last_updated": cs.lastUpdated,
        "base":         "IDR",
    }
    
    cs.RespondWithJSON(w, http.StatusOK, response)
}

func (cs *CurrencyService) getRateHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    code := vars["code"]
    
    cs.mutex.RLock()
    defer cs.mutex.RUnlock()
    
    currency, exists := cs.rates[code]
    if !exists {
        cs.RespondWithError(w, http.StatusNotFound, "CURRENCY_NOT_FOUND", "Currency not found")
        return
    }
    
    cs.RespondWithJSON(w, http.StatusOK, currency)
}

func (cs *CurrencyService) updateRatesHandler(w http.ResponseWriter, r *http.Request) {
    if cs.apiKey == "" {
        cs.RespondWithError(w, http.StatusServiceUnavailable, "NO_API_KEY", "Exchange rate API not configured")
        return
    }
    
    if err := cs.fetchExchangeRates(); err != nil {
        cs.RespondWithError(w, http.StatusServiceUnavailable, "UPDATE_FAILED", "Failed to update rates")
        return
    }
    
    cs.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
        "status":      "updated",
        "updated_at":  cs.lastUpdated,
        "message":     "Exchange rates updated successfully",
    })
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}