// frontend/src/services/api.js - UPDATED FOR SERVICE SEPARATION
import axios from 'axios';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:8000/api';

class ApiService {
    constructor() {
        this.api = axios.create({
            baseURL: API_BASE_URL,
            timeout: 30000,
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
                'X-Client-Version': '1.0.0',
                'X-Client-Type': 'web',
            }
        });

        this.setupInterceptors();
        this.retryConfig = {
            retries: 3,
            retryDelay: 1000,
            retryCondition: this.isRetryableError.bind(this),
        };
    }

    setupInterceptors() {
        // Request interceptor
        this.api.interceptors.request.use(
            (config) => {
                const token = localStorage.getItem('token');
                if (token) {
                    config.headers.Authorization = `Bearer ${token}`;
                }
                
                // Indonesian compliance headers
                config.headers['X-Timezone'] = 'Asia/Jakarta';
                config.headers['X-Currency'] = 'IDR';
                config.headers['X-Locale'] = 'id-ID';
                config.headers['X-Request-Time'] = new Date().toISOString();
                config.headers['X-Request-ID'] = this.generateRequestId();
                
                return config;
            },
            (error) => Promise.reject(this.formatError(error))
        );

        // Response interceptor with retry logic
        this.api.interceptors.response.use(
            (response) => {
                // Format Indonesian currency in responses
                if (response.data?.data) {
                    response.data.data = this.formatIndonesianData(response.data.data);
                }
                
                // Cache successful responses for offline use
                this.cacheResponse(response);
                
                return response;
            },
            async (error) => {
                const originalRequest = error.config;
                
                // Handle authentication errors
                if (error.response?.status === 401 && !originalRequest._retry) {
                    this.handleAuthError();
                    return Promise.reject(this.formatError(error));
                }
                
                // Retry logic
                if (this.shouldRetry(error, originalRequest)) {
                    return this.retryRequest(originalRequest);
                }
                
                // Handle offline scenarios
                if (!navigator.onLine) {
                    const cachedResponse = this.getCachedResponse(originalRequest);
                    if (cachedResponse) {
                        return Promise.resolve(cachedResponse);
                    }
                }
                
                return Promise.reject(this.formatError(error));
            }
        );
    }

    generateRequestId() {
        return 'req_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
    }

    formatError(error) {
        if (!error.response) {
            return {
                message: navigator.onLine 
                    ? 'Network error - please check your connection'
                    : 'You are offline. Some features may not be available.',
                code: navigator.onLine ? 'NETWORK_ERROR' : 'OFFLINE',
                status: 0,
                isNetworkError: true,
            };
        }

        const { data, status } = error.response;
        return {
            message: data?.error || data?.message || 'An error occurred',
            code: data?.code || `HTTP_${status}`,
            status,
            details: data?.details,
            timestamp: data?.timestamp,
            requestId: data?.request_id,
        };
    }

    formatIndonesianData(data) {
        if (!data) return data;
        
        const formatCurrency = (obj) => {
            if (Array.isArray(obj)) {
                return obj.map(formatCurrency);
            } else if (obj && typeof obj === 'object') {
                const formatted = {};
                for (const [key, value] of Object.entries(obj)) {
                    if (this.isCurrencyField(key)) {
                        formatted[key] = this.formatIndonesianCurrency(value);
                    } else if (this.isDateField(key)) {
                        formatted[key] = this.formatIndonesianDate(value);
                    } else {
                        formatted[key] = formatCurrency(value);
                    }
                }
                return formatted;
            }
            return obj;
        };
        
        return formatCurrency(data);
    }

    isCurrencyField(key) {
        const currencyFields = [
            'amount', 'price', 'total', 'balance', 'subtotal', 'tax_amount',
            'total_amount', 'debit_amount', 'credit_amount', 'unit_price',
            'cost_price', 'line_total'
        ];
        return currencyFields.some(field => 
            key.toLowerCase().includes(field) || 
            key.toLowerCase().endsWith('_amount') ||
            key.toLowerCase().endsWith('_price')
        );
    }

    isDateField(key) {
        const dateFields = ['date', 'created_at', 'updated_at', 'posted_at', 'last_login'];
        return dateFields.some(field => key.toLowerCase().includes(field));
    }

    formatIndonesianCurrency(amount) {
        if (amount === null || amount === undefined) return amount;
        const numAmount = parseFloat(amount);
        if (isNaN(numAmount)) return amount;
        
        // Indonesian Rupiah formatting - no decimals
        return Math.round(numAmount);
    }

    formatIndonesianDate(dateString) {
        if (!dateString) return dateString;
        
        try {
            const date = new Date(dateString);
            return date.toLocaleDateString('id-ID', {
                timeZone: 'Asia/Jakarta',
                year: 'numeric',
                month: '2-digit',
                day: '2-digit'
            });
        } catch (error) {
            return dateString;
        }
    }

    shouldRetry(error, originalRequest) {
        if (originalRequest._retryCount >= this.retryConfig.retries) {
            return false;
        }
        
        return this.isRetryableError(error);
    }

    isRetryableError(error) {
        if (!error.response) return true; // Network errors
        
        const retryableStatuses = [408, 429, 500, 502, 503, 504];
        return retryableStatuses.includes(error.response.status);
    }

    async retryRequest(originalRequest) {
        originalRequest._retryCount = (originalRequest._retryCount || 0) + 1;
        
        const delay = this.retryConfig.retryDelay * Math.pow(2, originalRequest._retryCount - 1);
        await this.sleep(delay);
        
        return this.api(originalRequest);
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    cacheResponse(response) {
        if (!('caches' in window)) return;
        
        try {
            const cacheKey = this.getCacheKey(response.config);
            const cacheData = {
                data: response.data,
                timestamp: Date.now(),
                url: response.config.url,
                method: response.config.method,
            };
            
            localStorage.setItem(cacheKey, JSON.stringify(cacheData));
        } catch (error) {
            console.warn('Failed to cache response:', error);
        }
    }

    getCachedResponse(config) {
        try {
            const cacheKey = this.getCacheKey(config);
            const cached = localStorage.getItem(cacheKey);
            
            if (cached) {
                const cacheData = JSON.parse(cached);
                const maxAge = 5 * 60 * 1000; // 5 minutes
                
                if (Date.now() - cacheData.timestamp < maxAge) {
                    return {
                        data: cacheData.data,
                        status: 200,
                        statusText: 'OK (Cached)',
                        headers: { 'x-from-cache': 'true' },
                        config: config,
                    };
                }
            }
        } catch (error) {
            console.warn('Failed to get cached response:', error);
        }
        
        return null;
    }

    getCacheKey(config) {
        return `api_cache_${config.method}_${config.url}`;
    }

    handleAuthError() {
        this.clearAuth();
        if (window.location.pathname !== '/login') {
            window.location.href = '/login';
        }
    }

    // UPDATED: Enhanced login method to handle new service separation
    async login(credentials) {
        try {
            const response = await this.api.post('/auth/login', credentials);
            const { data } = response.data;
            
            localStorage.setItem('token', data.token);
            localStorage.setItem('user', JSON.stringify(data.user));
            localStorage.setItem('company', JSON.stringify(data.company));
            
            return { success: true, data: response.data };
        } catch (error) {
            return { success: false, error: error.message };
        }
    }

    // UPDATED: User-related API calls (authentication service)
    async getUsers() {
        return this.get('/users');
    }

    async updateProfile(profileData) {
        return this.put('/profile', profileData);
    }

    async getProfile() {
        return this.get('/profile');
    }

    async refreshToken() {
        return this.post('/auth/refresh');
    }

    // UPDATED: Company-related API calls (company service)
    async getCompanies() {
        return this.get('/companies');
    }

    async createCompany(company) {
        return this.post('/companies', company);
    }

    async updateCompany(id, company) {
        return this.put(`/companies/${id}`, company);
    }

    async getCompany(id) {
        return this.get(`/companies/${id}`);
    }

    async getCompanySettings(companyId) {
        return this.get(`/companies/${companyId}/settings`);
    }

    async updateCompanySettings(companyId, settings) {
        return this.put(`/companies/${companyId}/settings`, settings);
    }

    // Account-related API calls
    async getAccounts(companyId) {
        return this.get('/accounts', { company_id: companyId, include_balance: true });
    }

    async createAccount(account) {
        // Validate Indonesian account code format
        if (account.account_code && !/^\d{4}$/.test(account.account_code)) {
            throw new Error('Account code must be 4 digits for Indonesian accounting standards');
        }
        
        return this.post('/accounts', account);
    }

    async updateAccount(id, account) {
        return this.put(`/accounts/${id}`, account);
    }

    async getAccountBalance(id, asOf = null) {
        const params = asOf ? { as_of: asOf } : {};
        return this.get(`/accounts/${id}/balance`, params);
    }

    // Transaction-related API calls
    async getTransactions(companyId, filters = {}) {
        return this.get('/transactions', { company_id: companyId, ...filters });
    }

    async createTransaction(transaction) {
        // Client-side validation for Indonesian compliance
        if (transaction.lines) {
            const debits = transaction.lines.reduce((sum, line) => sum + (line.debit_amount || 0), 0);
            const credits = transaction.lines.reduce((sum, line) => sum + (line.credit_amount || 0), 0);
            
            if (Math.abs(debits - credits) > 0.01) {
                throw new Error('Transaction must be balanced (debits must equal credits)');
            }
            
            // Validate Indonesian Rupiah (no decimals)
            for (const line of transaction.lines) {
                if (line.debit_amount && line.debit_amount % 1 !== 0) {
                    throw new Error('Indonesian Rupiah amounts should not have decimal places');
                }
                if (line.credit_amount && line.credit_amount % 1 !== 0) {
                    throw new Error('Indonesian Rupiah amounts should not have decimal places');
                }
            }
        }
        
        return this.post('/transactions', transaction);
    }

    async getTransaction(id) {
        return this.get(`/transactions/${id}`);
    }

    async postTransaction(id) {
        return this.post(`/transactions/${id}/post`);
    }

    async reverseTransaction(id, reason) {
        return this.post(`/transactions/${id}/reverse`, { reason });
    }

    // Invoice-related API calls
    async getInvoices(companyId) {
        return this.get('/invoices', { company_id: companyId });
    }

    async createInvoice(invoice) {
        return this.post('/invoices', invoice);
    }

    async sendInvoice(id) {
        return this.post(`/invoices/${id}/send`);
    }

    async getCustomers(companyId) {
        return this.get('/customers', { company_id: companyId });
    }

    async createCustomer(customer) {
        return this.post('/customers', customer);
    }

    // Exchange rate operations
    async getExchangeRates() {
        try {
            const response = await this.get('/rates');
            
            // Ensure IDR is the base currency
            if (response.data && response.data.base !== 'IDR') {
                console.warn('Warning: Base currency is not IDR for Indonesian business');
            }
            
            return response;
        } catch (error) {
            // Fallback to cached rates if available
            const cachedRates = localStorage.getItem('exchange_rates_cache');
            if (cachedRates) {
                const parsed = JSON.parse(cachedRates);
                if (Date.now() - parsed.timestamp < 24 * 60 * 60 * 1000) { // 24 hours
                    return { data: parsed.rates };
                }
            }
            throw error;
        }
    }

    async convertCurrency(amount, from, to) {
        return this.post('/convert', { amount, from, to });
    }

    // Generic CRUD operations with enhanced error handling
    async get(endpoint, params = {}) {
        const indonesianParams = { 
            ...params, 
            timezone: 'Asia/Jakarta', 
            currency: 'IDR',
            locale: 'id-ID'
        };
        return this.api.get(endpoint, { params: indonesianParams });
    }

    async post(endpoint, data) {
        return this.api.post(endpoint, this.formatRequestData(data));
    }

    async put(endpoint, data) {
        return this.api.put(endpoint, this.formatRequestData(data));
    }

    async delete(endpoint) {
        return this.api.delete(endpoint);
    }

    formatRequestData(data) {
        if (!data) return data;
        
        // Ensure currency amounts are properly formatted for Indonesian Rupiah
        const formatForServer = (obj) => {
            if (Array.isArray(obj)) {
                return obj.map(formatForServer);
            } else if (obj && typeof obj === 'object') {
                const formatted = {};
                for (const [key, value] of Object.entries(obj)) {
                    if (this.isCurrencyField(key) && typeof value === 'number') {
                        formatted[key] = Math.round(value); // Remove decimals for IDR
                    } else {
                        formatted[key] = formatForServer(value);
                    }
                }
                return formatted;
            }
            return obj;
        };
        
        return formatForServer(data);
    }

    clearAuth() {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        localStorage.removeItem('company');
    }

    getCurrentUser() {
        try {
            const userStr = localStorage.getItem('user');
            return userStr ? JSON.parse(userStr) : null;
        } catch (error) {
            this.clearAuth();
            return null;
        }
    }

    getCurrentCompany() {
        try {
            const companyStr = localStorage.getItem('company');
            return companyStr ? JSON.parse(companyStr) : null;
        } catch (error) {
            return null;
        }
    }

    // Indonesian tax calculation helper
    calculatePPN(amount, rate = 11.0) {
        const baseAmount = Math.round(parseFloat(amount) || 0);
        const taxAmount = Math.round(baseAmount * (rate / 100));
        
        return {
            base_amount: baseAmount,
            tax_rate: rate,
            tax_amount: taxAmount,
            total_amount: baseAmount + taxAmount
        };
    }

    // Indonesian number formatting
    formatRupiah(amount) {
        if (amount === null || amount === undefined) return 'Rp 0';
        
        const numAmount = Math.round(parseFloat(amount) || 0);
        return `Rp ${numAmount.toLocaleString('id-ID')}`;
    }

    // Connection status monitoring
    setupConnectionMonitoring() {
        window.addEventListener('online', () => {
            console.log('Connection restored');
            // Retry failed requests
            this.retryFailedRequests();
        });
        
        window.addEventListener('offline', () => {
            console.log('Connection lost - switching to offline mode');
        });
    }

    retryFailedRequests() {
        // Implementation would retry any queued requests
        const failedRequests = JSON.parse(localStorage.getItem('failed_requests') || '[]');
        
        failedRequests.forEach(async (request) => {
            try {
                await this.api(request);
                // Remove from failed requests on success
            } catch (error) {
                console.warn('Retry failed:', error);
            }
        });
        
        localStorage.removeItem('failed_requests');
    }
}

export default new ApiService();